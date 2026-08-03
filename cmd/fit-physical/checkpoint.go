package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

var errCheckpointMismatch = errors.New("checkpoint does not match this run")

// Checkpoint is what survives an interrupted fit.
//
// A true continuation is not possible: mayfly runs its whole loop inside
// Optimize, exposes no per-iteration hook and cannot be given a starting
// population, so a stopped swarm's velocities and personal bests are gone. What
// can be saved is everything that stopping would otherwise waste —
//
//   - Restarts holds each restart's result. Multi-start is the outer loop, and
//     a finished restart is finished: a resumed run skips it and keeps its
//     answer. Complete is false for a restart the interrupt cut short, which is
//     re-run rather than trusted, since it saw fewer iterations than the run
//     asked for.
//   - Best is the best position any restart reached, finished or not, recorded
//     from inside the objective. This is the field that matters when the run is
//     stopped early: all restarts run concurrently, so an interrupt part-way
//     through a fit typically finds none of them complete, and without this the
//     whole elapsed search would be lost.
//
// Because a re-run of a given seed is deterministic, resuming re-derives what it
// re-runs rather than approximating it. The saving is wall clock, not accuracy.
type Checkpoint struct {
	Fingerprint Fingerprint     `json:"fingerprint"`
	Restarts    []RestartRecord `json:"restarts"`
	Best        *Snapshot       `json:"best,omitempty"`
}

// RestartRecord is one multi-start run's outcome.
type RestartRecord struct {
	Run         int       `json:"run"`
	Seed        int64     `json:"seed"`
	Complete    bool      `json:"complete"`
	Cost        float64   `json:"cost"`
	Position    []float64 `json:"position"`
	Evaluations int       `json:"evaluations"`
	Convergence []float64 `json:"convergence,omitempty"`
}

// Snapshot is the cheapest thing worth saving: one point and what it scored.
type Snapshot struct {
	Cost        float64   `json:"cost"`
	Position    []float64 `json:"position"`
	Evaluations int       `json:"evaluations"`
}

// Fingerprint is everything that changes what a stored position means or what a
// seed will produce.
//
// BaselineCost is the important entry and the reason this is not just a copy of
// the flags. It is the shipped bank's distance from the reference, measured end
// to end through the same synthesis and the same feature extraction the search
// uses, and it is computed on every run anyway. Any edit that moves a rendered
// sample or a measured feature moves it too — which is exactly the case a
// resume must refuse, because a best-of across measurements taken from two
// different models is not a fit, and nothing downstream would reveal the mix.
// A performance change that is genuinely bit-exact leaves it alone and resumes
// cleanly, so the guard also doubles as the test of that claim.
type Fingerprint struct {
	// Reference is every take the run was given, newline separated and in the
	// order they were given. Order is part of it: the velocities occupy the tail
	// of every stored position, one per take, so resuming a re-ordered list
	// would hand each take another take's velocity and never say so.
	Reference   string  `json:"reference"`
	Channel     string  `json:"channel"`
	Contact     string  `json:"contact,omitempty"`
	MalletGrams float64 `json:"malletGrams,omitempty"`
	LossScale   float64 `json:"lossScale,omitempty"`
	// SearchBlind widens the search space itself, so a checkpoint taken with it
	// set has positions of a different length. Resuming across it would not
	// merely mix two searches, it would misread every stored vector.
	SearchBlind bool `json:"searchBlind,omitempty"`
	// ModeCorrections changes the instrument rather than the search space, so
	// unlike SearchBlind a resume across it would read every stored vector
	// correctly and score it against a different drum — which is worse, because
	// nothing about the result would look wrong.
	ModeCorrections string             `json:"modeCorrections,omitempty"`
	SeededRestarts  int                `json:"seededRestarts,omitempty"`
	SeedWidth       float64            `json:"seedWidth,omitempty"`
	Quality         string             `json:"quality"`
	Variant         string             `json:"variant"`
	DurationSeconds float64            `json:"durationSeconds"`
	Iterations      int                `json:"iterations"`
	Population      int                `json:"population"`
	Restarts        int                `json:"restarts"`
	Seed            int64              `json:"seed"`
	Fixed           map[string]float64 `json:"fixed,omitempty"`
	BaselineCost    float64            `json:"baselineCost"`
}

// disagreement names the first field that differs, or "" when the two describe
// the same run. Naming it matters: "checkpoint does not match" sends someone
// hunting through a JSON file, while "quality" tells them what they changed.
func (f Fingerprint) disagreement(other Fingerprint) string {
	fields := []struct {
		name  string
		equal bool
	}{
		{"reference", f.Reference == other.Reference},
		{"channel", f.Channel == other.Channel},
		{"contact", f.Contact == other.Contact},
		{"mallet mass", f.MalletGrams == other.MalletGrams},
		{"loss scale", f.LossScale == other.LossScale},
		{
			"mode corrections, so the drum being fitted is a different one",
			f.ModeCorrections == other.ModeCorrections,
		},
		{
			"search-blind, so the search space is a different width",
			f.SearchBlind == other.SearchBlind,
		},
		{"seeded restarts", f.SeededRestarts == other.SeededRestarts},
		{"seed width", f.SeedWidth == other.SeedWidth},
		{"quality", f.Quality == other.Quality},
		{"variant", f.Variant == other.Variant},
		{"duration", f.DurationSeconds == other.DurationSeconds},
		{"iterations", f.Iterations == other.Iterations},
		{"population", f.Population == other.Population},
		{"restarts", f.Restarts == other.Restarts},
		{"seed", f.Seed == other.Seed},
		{"fixed parameters", sameAssignments(f.Fixed, other.Fixed)},
		{
			"baseline cost, so the model or the measurement changed",
			f.BaselineCost == other.BaselineCost,
		},
	}

	for _, field := range fields {
		if !field.equal {
			return field.name
		}
	}

	return ""
}

func sameAssignments(left, right map[string]float64) bool {
	if len(left) != len(right) {
		return false
	}

	for name, value := range left {
		if other, ok := right[name]; !ok || other != value {
			return false
		}
	}

	return true
}

// store owns a checkpoint file. Every restart writes to it concurrently, so it
// carries the lock; a nil store is the no-checkpoint case and every method is a
// no-op, which keeps the callers free of conditionals.
type store struct {
	path string

	mu    sync.Mutex
	state Checkpoint
}

// loadStore opens or creates the checkpoint at path, refusing one that was
// written for a different run.
func loadStore(path string, fingerprint Fingerprint) (*store, error) {
	if path == "" {
		return nil, nil //nolint:nilnil // no path means no checkpointing, not an error.
	}

	subject := &store{path: path, state: Checkpoint{Fingerprint: fingerprint}}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return subject, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read checkpoint: %w", err)
	}

	var existing Checkpoint
	if err := json.Unmarshal(raw, &existing); err != nil {
		return nil, fmt.Errorf("read checkpoint %s: %w", path, err)
	}

	if field := existing.Fingerprint.disagreement(fingerprint); field != "" {
		return nil, fmt.Errorf("%w: %s differs from %s; delete it or pass a new -checkpoint",
			errCheckpointMismatch, field, path)
	}

	subject.state = existing

	return subject, nil
}

// completed reports the runs a resume may skip.
func (s *store) completed() map[int]RestartRecord {
	done := map[int]RestartRecord{}

	if s == nil {
		return done
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, record := range s.state.Restarts {
		if record.Complete {
			done[record.Run] = record
		}
	}

	return done
}

// best returns the best point recorded so far, or nil.
func (s *store) best() *Snapshot {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.Best == nil {
		return nil
	}

	snapshot := *s.state.Best
	snapshot.Position = slices.Clone(s.state.Best.Position)

	return &snapshot
}

// recordRestart stores one restart's outcome, replacing any earlier record for
// the same run.
func (s *store) recordRestart(record RestartRecord) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	index := slices.IndexFunc(s.state.Restarts, func(existing RestartRecord) bool {
		return existing.Run == record.Run
	})
	if index < 0 {
		s.state.Restarts = append(s.state.Restarts, record)
	} else {
		s.state.Restarts[index] = record
	}

	slices.SortFunc(s.state.Restarts, func(a, b RestartRecord) int { return a.Run - b.Run })

	return s.writeLocked()
}

// recordBest keeps the cheapest position seen, and reports whether it improved.
// It is called from the objective, so the common path is one comparison under a
// lock already held for the evaluation counter's sake.
func (s *store) recordBest(cost float64, position []float64, evaluations int) bool {
	if s == nil {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.Best != nil && s.state.Best.Cost <= cost {
		return false
	}

	s.state.Best = &Snapshot{
		Cost:        cost,
		Position:    slices.Clone(position),
		Evaluations: evaluations,
	}

	return true
}

// flush writes the current state out.
func (s *store) flush() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.writeLocked()
}

// writeLocked replaces the file atomically, so an interrupt during the write
// cannot leave a truncated checkpoint behind — the one file whose job is to
// survive an interrupt.
func (s *store) writeLocked() error {
	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode checkpoint: %w", err)
	}

	directory := filepath.Dir(s.path)

	if err := ensureDir(s.path); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}

	temporary, err := os.CreateTemp(directory, ".fit-checkpoint-*")
	if err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}

	name := temporary.Name()

	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		_ = os.Remove(name)

		return fmt.Errorf("write checkpoint: %w", err)
	}

	if err := temporary.Close(); err != nil {
		_ = os.Remove(name)

		return fmt.Errorf("write checkpoint: %w", err)
	}

	if err := os.Rename(name, s.path); err != nil {
		_ = os.Remove(name)

		return fmt.Errorf("write checkpoint: %w", err)
	}

	return nil
}

// inspectOptions is what -inspect needs beyond the checkpoint itself.
//
// storedBaseline is carried separately from the report's own because the two are
// the point when rebased is set: the checkpoint was written against one and this
// build measures the other.
type inspectOptions struct {
	checkpointPath string
	rebased        bool
	storedBaseline float64
	wavPath        string
	wavDuration    time.Duration
	wavTake        int
	outputPath     string
}

// inspectCheckpoint describes a checkpoint's best point and stops.
//
// The point is to see a running fit's best candidate broken down by term without
// interrupting it: checkpoints are written atomically, so a reader gets one whole
// version or another, and the search never learns it was read. The fingerprint
// guard has already run in the caller, which is what stops this describing one
// run's point against another run's reference — with the one documented exception
// of a baseline that drifted inside baselineDriftTolerance, disclosed here.
//
// It lives beside the checkpoint rather than in main for the reason -hessian
// lives in identify.go: what it does is read a stored position, and everything it
// needs to do that honestly is here.
func inspectCheckpoint(
	stdout, stderr io.Writer,
	base *evaluator,
	report Report,
	checkpoint *store,
	options inspectOptions,
) error {
	snapshot := checkpoint.best()
	if snapshot == nil {
		return fmt.Errorf("%w: %s holds no best point yet",
			errInvalidFitOption, options.checkpointPath)
	}

	if options.rebased {
		report.BaselineDrift = noteBaselineDrift(stderr,
			options.storedBaseline, report.Baseline.Terms.Total, options.checkpointPath)
	}

	candidate, err := base.describe(snapshot.Position)
	if err != nil {
		return err
	}

	report.Best = &candidate
	report.Search.Evaluations = snapshot.Evaluations
	report.Search.Interrupted = true

	_, _ = fmt.Fprintf(stderr, "inspected: %s after %d evaluations\n",
		summarize(candidate.Terms), snapshot.Evaluations)

	// Same tail as a finished run, -wav included. A checkpoint holds the bank, so
	// the point it describes can be listened to as well as read — which matters
	// here more than the convenience suggests: twice now a defect in this metric
	// has been found by hearing something the distance called good, and a run one
	// has to finish before it can be auditioned is one nobody auditions.
	return finish(stdout, stderr, report,
		options.wavPath, options.wavDuration, options.wavTake, options.outputPath)
}
