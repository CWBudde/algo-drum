// Command fit-physical searches the physical Tom's parameter bank for the
// settings that come closest to a recorded hit.
//
// It renders candidates at the reference's own sample rate, reduces both to
// the same perceptual features, and minimizes their weighted distance with the
// Mayfly Optimization Algorithm. The search space is exactly the bank the
// product exposes, so anything it finds can be typed into the app or shared as
// a link — a fit that needed a hidden parameter would not be a preset.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cwbudde/algo-drum/internal/drum"
	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/algo-drum/internal/physical/match"
	"github.com/cwbudde/mayfly"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "fit-physical: %v\n", err)
		os.Exit(1)
	}
}

// assignmentFlag collects repeatable ID=value pairs.
type assignmentFlag map[string]float64

func (a assignmentFlag) String() string {
	parts := make([]string, 0, len(a))
	for name, value := range a {
		parts = append(parts, fmt.Sprintf("%s=%g", name, value))
	}

	return strings.Join(parts, ",")
}

func (a assignmentFlag) Set(text string) error {
	name, raw, found := strings.Cut(text, "=")
	if !found {
		return fmt.Errorf("%w: expected ID=value, got %q", errInvalidFitOption, text)
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", errInvalidFitOption, name, err)
	}

	a[strings.TrimSpace(name)] = value

	return nil
}

// Report is what the run writes out.
type Report struct {
	Reference ReferenceInfo  `json:"reference"`
	Options   match.Options  `json:"options"`
	Weights   match.Weights  `json:"weights"`
	Search    SearchInfo     `json:"search"`
	Baseline  Candidate      `json:"baseline"`
	Best      *Candidate     `json:"best,omitempty"`
	Target    match.Features `json:"target"`
}

// ReferenceInfo records what was measured, so a report can be read a year
// later without the file beside it.
type ReferenceInfo struct {
	Path         string  `json:"path"`
	Channel      string  `json:"channel"`
	SampleRateHz float64 `json:"sampleRateHz"`
	Channels     int     `json:"channels"`
	BitDepth     int     `json:"bitDepth"`
	Frames       int     `json:"frames"`
}

// SearchInfo records enough to repeat the run.
type SearchInfo struct {
	Variant         string  `json:"variant"`
	Iterations      int     `json:"iterations"`
	Population      int     `json:"population"`
	Restarts        int     `json:"restarts"`
	Seed            int64   `json:"seed"`
	Evaluations     int     `json:"evaluations"`
	DurationSeconds float64 `json:"durationSeconds"`
	Quality         string  `json:"quality"`
	Contact         string  `json:"contact,omitempty"`
	MalletGrams     float64 `json:"malletGrams,omitempty"`
	// LossScale is 1 for every run that stays inside the shipped ranges, and
	// anything else marks a report whose bank the product cannot express.
	LossScale float64 `json:"lossScale,omitempty"`
	// SeedErrorCents records what the analytic pre-solve achieved for each
	// seeded restart, best first. It is the honest caption for a seeded run: a
	// low number here says the starting point matched the reference's partial
	// frequencies, and nothing whatever about the fit that followed.
	SeedErrorCents []float64 `json:"seedErrorCents,omitempty"`
	SeedWidth      float64   `json:"seedWidth,omitempty"`
	// Interrupted marks a report the search did not finish producing. It also
	// qualifies Evaluations, which counts mayfly's calls rather than rendered
	// candidates: after an interrupt the objective refuses work without
	// measuring it, so the tail of that count is the search winding up.
	Interrupted bool               `json:"interrupted,omitempty"`
	FixedParams map[string]float64 `json:"fixedParams,omitempty"`
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("fit-physical", flag.ContinueOnError)
	flags.SetOutput(stderr)

	referencePath := flags.String("reference", "", "reference WAV file (required)")
	channel := flags.String("channel", string(match.ChannelMono), "channel reduction: mono, left or right")
	outputPath := flags.String("o", "-", "JSON report path, or - for stdout")
	duration := flags.Float64("duration", 1.2, "candidate render duration in seconds")
	quality := flags.String("quality", string(physical.QualityDraft),
		"mode budget during the search: draft, standard or high")
	contact := flags.String("contact", "",
		"excitation model: prescribed, hertzian, or empty for the configured default")
	malletGrams := flags.Float64("mallet-g", 0,
		"stick mass in grams, or 0 for the configured default")
	lossScale := flags.Float64("loss-scale", 1,
		"extra multiplier on every head loss rate, to search past DAMP's own range")
	seededRestarts := flags.Int("seeded-restarts", 0,
		"start this many restarts from an analytic solve for the reference's partial frequencies")
	seedWidth := flags.Float64("seed-width", 0.25,
		"half-width of the box a seeded restart searches around its seed")
	variant := flags.String("variant", "ma", "mayfly variant: ma, desma, olce, eobbma, gsasma, mpma or aoblmoa")
	iterations := flags.Int("iterations", 150, "mayfly iterations per restart")
	population := flags.Int("pop", 20, "mayfly males, and as many females")
	restarts := flags.Int("restarts", 0, "independent seeded runs, best of; 0 picks one per available core")
	seed := flags.Int64("seed", 1, "base RNG seed; restart n uses seed+n")
	reportOnly := flags.Bool("report-only", false, "measure the shipped defaults against the reference and stop")
	progressEvery := flags.Int("progress", 500,
		"print a progress line every N objective evaluations; 0 silences it")
	checkpointPath := flags.String("checkpoint", "",
		"file to save finished restarts and the best point to, and to resume from")
	inspect := flags.Bool("inspect", false,
		"describe the -checkpoint file's best point and stop, without searching")
	wavPath := flags.String("wav", "",
		"also render the fitted bank to this mono WAV file, for listening to")
	wavDuration := flags.Duration("wav-duration", 3*time.Second,
		"render duration for -wav; the search's own -duration is far too short to judge a tail by")

	fixed := assignmentFlag{}
	flags.Var(fixed, "fix", "freeze one parameter at a normalized position, as ID=value (repeatable)")

	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() > 0 {
		return fmt.Errorf("%w: unexpected argument %q", errInvalidFitOption, flags.Arg(0))
	}

	if *referencePath == "" {
		return fmt.Errorf("%w: -reference is required", errInvalidFitOption)
	}

	if *duration <= 0 {
		return fmt.Errorf("%w: duration %v", errInvalidFitOption, *duration)
	}

	if *iterations <= 0 || *population < 4 {
		return fmt.Errorf("%w: iterations %d, population %d (population must be at least 4)",
			errInvalidFitOption, *iterations, *population)
	}

	if *restarts < 0 {
		return fmt.Errorf("%w: restarts %d", errInvalidFitOption, *restarts)
	}

	if *progressEvery < 0 {
		return fmt.Errorf("%w: progress %d", errInvalidFitOption, *progressEvery)
	}

	switch physical.ContactModel(*contact) {
	case "", physical.ContactPrescribed, physical.ContactHertzian:
	default:
		return fmt.Errorf("%w: contact %q is neither %q nor %q",
			errInvalidFitOption, *contact, physical.ContactPrescribed, physical.ContactHertzian)
	}

	// Bounded by physical.Validate's own range for the mass, so an out-of-range
	// value is rejected here rather than turning every candidate into +Inf.
	if *malletGrams < 0 || *malletGrams > 1000 {
		return fmt.Errorf("%w: mallet mass %v g", errInvalidFitOption, *malletGrams)
	}

	// Bounded well inside what the loss law can carry: at zero the heads never
	// stop and every decay measure degenerates, and past a hundredfold the
	// modes are gone before the first analysis window closes.
	if *lossScale <= 0.01 || *lossScale > 100 {
		return fmt.Errorf("%w: loss scale %v", errInvalidFitOption, *lossScale)
	}

	if *seededRestarts < 0 {
		return fmt.Errorf("%w: seeded restarts %d", errInvalidFitOption, *seededRestarts)
	}

	// Half a cube is the whole cube, so anything at or past it is not a box and
	// is rejected rather than silently ignored.
	if *seededRestarts > 0 && (*seedWidth <= 0 || *seedWidth >= 0.5) {
		return fmt.Errorf("%w: seed width %v is not inside (0, 0.5)",
			errInvalidFitOption, *seedWidth)
	}

	if *restarts == 0 {
		*restarts = max(1, runtime.NumCPU()-1)
	}

	// QUAL is pinned rather than fitted. It buys mode count with CPU, so a
	// search left free would always spend its way to High and then ship a tom
	// that costs twice what the app budgeted for — a decision about the
	// product, not about the sound of this drum.
	if err := pinQuality(fixed, *quality); err != nil {
		return err
	}

	bank, free, err := resolveFixed(fixed, !*reportOnly)
	if err != nil {
		return err
	}

	reference, err := match.LoadReference(*referencePath, match.Channel(*channel))
	if err != nil {
		return err
	}

	options := match.DefaultOptions()

	target, err := match.Extract(reference.Samples, reference.SampleRateHz, options)
	if err != nil {
		return fmt.Errorf("measure reference: %w", err)
	}

	base := &evaluator{
		reference:       target,
		options:         options,
		weights:         match.DefaultWeights(),
		bank:            bank,
		free:            free,
		sampleRateHz:    reference.SampleRateHz,
		durationSeconds: *duration,
		contact:         physical.ContactModel(*contact),
		malletMassKg:    *malletGrams / 1000,
		lossScale:       *lossScale,
		// Rendered at the reference's own rate, so no resampler ever enters
		// the measurement path on either side.
		buffer: make([]float64, int(*duration*reference.SampleRateHz)),
	}

	report := Report{
		Reference: ReferenceInfo{
			Path:         *referencePath,
			Channel:      *channel,
			SampleRateHz: reference.SampleRateHz,
			Channels:     reference.Channels,
			BitDepth:     reference.BitDepth,
			Frames:       len(reference.Samples),
		},
		Options: options,
		Weights: base.weights,
		Target:  target,
		Search: SearchInfo{
			Variant:         *variant,
			Iterations:      *iterations,
			Population:      *population,
			Restarts:        *restarts,
			Seed:            *seed,
			DurationSeconds: *duration,
			Quality:         *quality,
			Contact:         *contact,
			MalletGrams:     *malletGrams,
			LossScale:       *lossScale,
			FixedParams:     fixed,
		},
	}

	baseline := newEvaluator(base)

	// The shipped defaults, at the velocity the product's own audition uses.
	report.Baseline, err = baseline.describe(baseline.position(defaultVelocity))
	if err != nil {
		return fmt.Errorf("measure the shipped defaults: %w", err)
	}

	_, _ = fmt.Fprintf(stderr, "reference: %d partials, fundamental %.2f Hz, glide %.1f cents\n",
		len(target.Partials), fundamentalHz(target), target.GlideCents)
	_, _ = fmt.Fprintf(stderr, "baseline:  %s\n", summarize(report.Baseline.Terms))

	// The pre-solve is analytic and takes a second or two, so it runs before the
	// checkpoint is opened and its outcome goes into the fingerprint: a resume
	// that reproduced different seeds would be resuming a different search.
	var (
		seeds    []seedCandidate
		relevant []bool
	)

	if !*reportOnly && *seededRestarts > 0 {
		presolve := rand.New(rand.NewSource(*seed))
		relevant = frequencyRelevant(target, bank, free,
			options.PartialFloorDB, reference.SampleRateHz, presolve)
		seeds = frequencySeeds(target, bank, free, min(*seededRestarts, *restarts),
			options.PartialFloorDB, reference.SampleRateHz, presolve, defaultSeedBudget)

		named := make([]string, 0, len(free))
		specs := drum.PhysicalTomSpecs()

		for i, index := range free {
			if relevant[i] {
				named = append(named, specs[index].Label)
			}
		}

		_, _ = fmt.Fprintf(stderr, "  seeding %d of %d free parameters: %s\n",
			len(named), len(free), strings.Join(named, " "))

		for index, candidate := range seeds {
			_, _ = fmt.Fprintf(stderr, "  seed %d: %.1f cents from the reference's partials\n",
				index+1, candidate.errorCents)

			report.Search.SeedErrorCents = append(report.Search.SeedErrorCents, candidate.errorCents)
		}

		report.Search.SeedWidth = *seedWidth
	}

	// Only meaningful when something is seeded, so an unseeded run does not
	// carry a width none of its restarts ever used into the fingerprint.
	fingerprintSeedWidth := 0.0
	if len(seeds) > 0 {
		fingerprintSeedWidth = *seedWidth
	}

	if !*reportOnly {
		// The baseline goes into the fingerprint, so it has to be measured
		// before the checkpoint is opened — which it is, just above.
		checkpoint, err := loadStore(*checkpointPath, Fingerprint{
			SeededRestarts:  len(seeds),
			SeedWidth:       fingerprintSeedWidth,
			Reference:       *referencePath,
			Channel:         *channel,
			Contact:         *contact,
			MalletGrams:     *malletGrams,
			LossScale:       *lossScale,
			Quality:         *quality,
			Variant:         *variant,
			DurationSeconds: *duration,
			Iterations:      *iterations,
			Population:      *population,
			Restarts:        *restarts,
			Seed:            *seed,
			Fixed:           fixed,
			BaselineCost:    report.Baseline.Terms.Total,
		})
		if err != nil {
			return err
		}

		if resumed := len(checkpoint.completed()); resumed > 0 {
			_, _ = fmt.Fprintf(stderr, "resuming:  %d of %d restarts already finished\n",
				resumed, *restarts)
		}

		// -inspect reads a checkpoint and stops. The point is to see a running
		// fit's best candidate broken down by term without interrupting it:
		// checkpoints are written atomically, so a reader gets one whole version
		// or another, and the search never learns it was read. The fingerprint
		// guard still applies, which is what stops this describing one run's
		// point against another run's reference.
		if *inspect {
			snapshot := checkpoint.best()
			if snapshot == nil {
				return fmt.Errorf("%w: %s holds no best point yet",
					errInvalidFitOption, *checkpointPath)
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

			// Same tail as a finished run, -wav included. A checkpoint holds the
			// bank, so the point it describes can be listened to as well as read
			// — which matters here more than the convenience suggests: twice now
			// a defect in this metric has been found by hearing something the
			// distance called good, and a run one has to finish before it can be
			// auditioned is one nobody auditions.
			return finish(stdout, stderr, report, *wavPath, *wavDuration, *outputPath)
		}

		// An interrupt asks the search to wind up, not the process to die: the
		// objective starts refusing work, every restart returns the best it
		// actually found, and the report and checkpoint are written as usual. A
		// second signal takes the default behaviour and kills it outright.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		offspring := *population - *population%2
		progress := newTracker(stderr, *progressEvery,
			expectedEvaluations(*restarts, *iterations, *population, offspring), time.Now())
		progress.checkpoint = checkpoint

		best, evaluations, err := search(
			ctx, base, *variant, *iterations, *population, *restarts, *seed,
			seeds, relevant, *seedWidth, stderr, progress, checkpoint,
		)
		if err != nil {
			return err
		}

		if err := checkpoint.flush(); err != nil {
			return err
		}

		if ctx.Err() != nil {
			report.Search.Interrupted = true

			_, _ = fmt.Fprintf(stderr,
				"interrupted; the report below is the best point found so far\n")
		}

		report.Best = &best
		report.Search.Evaluations = evaluations

		_, _ = fmt.Fprintf(stderr, "best:      %s\n", summarize(best.Terms))
	}

	return finish(stdout, stderr, report, *wavPath, *wavDuration, *outputPath)
}

// finish writes what a run leaves behind: the summary, the optional WAV, and
// the JSON report. Shared so that -inspect leaves the same things behind as a
// completed run, rather than a subset of them.
func finish(
	stdout, stderr io.Writer,
	report Report,
	wavPath string,
	wavDuration time.Duration,
	outputPath string,
) error {
	writeSummary(stdout, report)

	// The baseline is the fallback for the same reason writeSummary uses it: with
	// -report-only there is no fitted bank, and the shipped one is what the run
	// measured.
	if wavPath != "" {
		exported := report.Baseline
		if report.Best != nil {
			exported = *report.Best
		}

		peak, err := exportCandidate(wavPath, exported, wavDuration)
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintf(stderr, "wrote %s: %.2fs at velocity %.3f, source peak %.6g\n",
			wavPath, wavDuration.Seconds(), exported.Velocity01, peak)
	}

	return writeReport(outputPath, report)
}

// defaultVelocity is the audition level the VoiceEditor triggers at, and the
// obvious level at which to quote a baseline.
const defaultVelocity = 1.0

func pinQuality(fixed assignmentFlag, quality string) error {
	specs := drum.PhysicalTomSpecs()

	index := -1

	for i, spec := range specs {
		if spec.ID == "physicalTom.quality" {
			index = i
		}
	}

	if index < 0 {
		return fmt.Errorf("%w: the parameter table has no quality entry", errInvalidFitOption)
	}

	spec := specs[index]

	tier := -1

	for i, choice := range spec.Choices {
		if strings.EqualFold(choice, quality) {
			tier = i
		}
	}

	if tier < 0 {
		return fmt.Errorf("%w: quality %q is not one of %v",
			errInvalidFitOption, quality, spec.Choices)
	}

	if _, already := fixed[spec.ID]; !already {
		fixed[spec.ID] = float64(tier) / float64(len(spec.Choices)-1)
	}

	return nil
}

// search runs independent mayfly runs concurrently and keeps the best.
//
// The concurrency is between runs rather than inside one, because mayfly calls
// the objective sequentially. Multi-start is what this landscape wants anyway:
// the detent in every knob's mapping puts a small flat spot at each default,
// and a single swarm can settle into one.
func search(
	ctx context.Context,
	base *evaluator,
	variant string,
	iterations, population, restarts int,
	seed int64,
	seeds []seedCandidate,
	relevant []bool,
	seedWidth float64,
	stderr io.Writer,
	progress *tracker,
	checkpoint *store,
) (Candidate, int, error) {
	// Restarts a previous run finished are not re-run. Their positions are
	// replayed through describe rather than stored as candidates, so the report
	// is written by this build's measurement code and a resumed report says the
	// same thing an uninterrupted one would.
	done := checkpoint.completed()

	type outcome struct {
		candidate   Candidate
		evaluations int
		convergence []float64
		err         error
	}

	results := make([]outcome, restarts)

	var group sync.WaitGroup

	for run := range restarts {
		group.Add(1)

		go func() {
			defer group.Done()

			// Since mayfly v0.2.0 an oversized NC is a returned error rather
			// than a panic from inside the library, so this recover no longer
			// has a known trigger. It stays as a backstop: the objective runs
			// third-party numerics on adversarially-chosen parameters, and one
			// restart dying should never take the other seven with it.
			defer func() {
				if recovered := recover(); recovered != nil {
					results[run] = outcome{err: fmt.Errorf(
						"%w: restart %d panicked inside the optimizer: %v",
						errInvalidFitOption, run+1, recovered,
					)}
				}
			}()

			local := newEvaluator(base)

			if record, ok := done[run]; ok {
				candidate, err := local.describe(record.Position)
				if err != nil {
					results[run] = outcome{err: err}

					return
				}

				_, _ = fmt.Fprintf(stderr, "  restart %d/%d: total %.3f from the checkpoint\n",
					run+1, restarts, candidate.Terms.Total)

				results[run] = outcome{
					candidate:   candidate,
					evaluations: record.Evaluations,
					convergence: record.Convergence,
				}

				return
			}

			// Seeded restarts come first, so an -seeded-restarts below the
			// restart count leaves the remainder searching the whole cube. That
			// mix is the point: a seed is a hypothesis about where to look, and
			// the unseeded restarts are what would find it wrong.
			//
			// The box is applied here and nowhere else, so the position that
			// reaches the objective, the progress tracker, the checkpoint and
			// the report is always an ordinary bank position. A stored point
			// therefore means the same thing whether its restart was seeded.
			warp := boxAround(nil, nil, 0)
			if run < len(seeds) {
				warp = boxAround(seeds[run].position, relevant, seedWidth)
			}

			// Progress is reported from inside the objective because mayfly has
			// no per-iteration hook: without this the run says nothing until a
			// whole restart finishes.
			objective := func(raw []float64) float64 {
				position := warp(raw)

				// Cancellation is cooperative for the same reason: Optimize
				// cannot be interrupted, so an abandoned evaluation returns the
				// cost of a configuration that is not a drum. mayfly keeps its
				// incumbent, so the restart still reports the best it genuinely
				// found — the run unwinds in seconds without discarding it.
				if ctx.Err() != nil {
					return math.Inf(1)
				}

				cost := local.cost(position)
				progress.observe(cost, position)

				return cost
			}

			config, err := mayfly.NewBuilder(variant).
				ForProblem(objective, local.dimensions(), 0, 1).
				WithIterations(iterations).
				WithPopulation(population, population).
				WithConfig(func(settings *mayfly.Config) {
					settings.Rand = rand.New(rand.NewSource(seed + int64(run)))
					// One offspring per pair, which is the paper's ratio. The
					// library's fixed default of 20 would exceed the population
					// for any -pop below 10; v0.2.0 rejects that rather than
					// indexing past the end, but rejecting is still not what we
					// want here.
					settings.NC = population - population%2
				}).
				Build()
			if err != nil {
				results[run] = outcome{err: err}

				return
			}

			result, err := mayfly.Optimize(config)
			if err != nil {
				results[run] = outcome{err: err}

				return
			}

			candidate, err := local.describe(warp(result.GlobalBest.Position))
			if err != nil {
				results[run] = outcome{err: err}

				return
			}

			// A restart the interrupt cut short is recorded but not marked
			// complete: it saw fewer real evaluations than the run asked for, so
			// a later resume must run it again rather than adopt its answer.
			complete := ctx.Err() == nil

			state := "after"
			if !complete {
				state = "interrupted after"
			}

			_, _ = fmt.Fprintf(stderr, "  restart %d/%d: total %.3f %s %d evaluations\n",
				run+1, restarts, result.GlobalBest.Cost, state, result.FuncEvalCount)

			if err := checkpoint.recordRestart(RestartRecord{
				Run:         run,
				Seed:        seed + int64(run),
				Complete:    complete,
				Cost:        result.GlobalBest.Cost,
				Position:    result.GlobalBest.Position,
				Evaluations: result.FuncEvalCount,
				Convergence: result.ConvergenceCurve,
			}); err != nil {
				results[run] = outcome{err: err}

				return
			}

			results[run] = outcome{
				candidate:   candidate,
				evaluations: result.FuncEvalCount,
				convergence: result.ConvergenceCurve,
			}
		}()
	}

	group.Wait()

	best := Candidate{}
	evaluations := 0
	found := false

	for _, result := range results {
		if result.err != nil {
			return Candidate{}, 0, result.err
		}

		evaluations += result.evaluations

		if !found || result.candidate.Terms.Total < best.Terms.Total {
			best, found = result.candidate, true
			best.Convergence = result.convergence
		}
	}

	// The running snapshot is a candidate in its own right. It is usually
	// beaten by the restart that produced it — a swarm's incumbent is its own
	// global best — but after an interrupt it can be the only thing left, and a
	// point that measured better is better whatever produced it.
	if snapshot := checkpoint.best(); snapshot != nil && (!found || snapshot.Cost < best.Terms.Total) {
		candidate, err := newEvaluator(base).describe(snapshot.Position)
		if err != nil {
			return Candidate{}, 0, err
		}

		if !found || candidate.Terms.Total < best.Terms.Total {
			_, _ = fmt.Fprintf(stderr, "  best point came from the checkpoint: total %.3f\n",
				candidate.Terms.Total)

			best, found = candidate, true
		}
	}

	if !found {
		return Candidate{}, 0, fmt.Errorf("%w: no restart produced a result", errInvalidFitOption)
	}

	return best, evaluations, nil
}

func fundamentalHz(features match.Features) float64 {
	if len(features.Partials) == 0 {
		return 0
	}

	return features.Partials[0].FrequencyHz
}

func summarize(terms match.Terms) string {
	return fmt.Sprintf(
		"total %.3f (freq %.1f¢, level %.1f dB, decay %.3f, spectrum %.1f dB, "+
			"envelope %.1f dB, glide %.1f¢, attack %.1f dB, unmatched %.3f, spurious %.3f)",
		terms.Total, terms.PartialFrequency, terms.PartialLevel, terms.PartialDecay,
		terms.SpectralEnvelope, terms.Envelope, terms.Glide, terms.AttackBalance,
		terms.Unmatched, terms.Spurious,
	)
}

// writeSummary prints the table a person actually reads.
func writeSummary(stdout io.Writer, report Report) {
	_, _ = fmt.Fprintf(stdout, "\n%-8s %-10s %12s %12s\n", "PARAM", "LABEL", "BASELINE", "FITTED")

	best := report.Best
	if best == nil {
		best = &report.Baseline
	}

	for index, param := range report.Baseline.Params {
		fitted := best.Params[index]

		note := ""

		switch {
		case fitted.Fixed:
			note = " (fixed)"
		case fitted.Pinned:
			note = " (pinned at a bound)"
		}

		_, _ = fmt.Fprintf(stdout, "%-8d %-10s %12.4g %12.4g%s\n",
			index, param.Label, param.Value, fitted.Value, note)
	}

	_, _ = fmt.Fprintf(stdout, "%-8s %-10s %12.4g %12.4g\n",
		"-", "VEL", report.Baseline.Velocity01, best.Velocity01)

	_, _ = fmt.Fprintf(stdout, "\nreference partials (Hz / dB / T60 s):\n")
	for _, partial := range report.Target.Partials {
		_, _ = fmt.Fprintf(stdout, "  %8.2f %7.1f %6.2f\n",
			partial.FrequencyHz, partial.LevelDB, partial.T60Seconds)
	}

	_, _ = fmt.Fprintf(stdout, "\nfitted partials (Hz / dB / T60 s):\n")
	for _, partial := range best.Features.Partials {
		_, _ = fmt.Fprintf(stdout, "  %8.2f %7.1f %6.2f\n",
			partial.FrequencyHz, partial.LevelDB, partial.T60Seconds)
	}

	_, _ = fmt.Fprintf(stdout, "\nbaseline %s\n", summarize(report.Baseline.Terms))
	if report.Best != nil {
		_, _ = fmt.Fprintf(stdout, "fitted   %s\n", summarize(report.Best.Terms))
	}
}

// ensureDir creates the directory a run's artifacts are about to be written to.
// The defaults put reports and checkpoints under fits/ and WAVs under renders/,
// neither of which is committed (both are gitignored), so on a fresh clone the
// first run would otherwise fail on a missing directory rather than on anything
// the user did wrong.
func ensureDir(path string) error {
	directory := filepath.Dir(path)
	if directory == "" || directory == "." {
		return nil
	}

	return os.MkdirAll(directory, 0o755)
}

func writeReport(path string, report Report) error {
	if path == "-" {
		return nil
	}

	if err := ensureDir(path); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(report); err != nil {
		return err
	}

	return file.Close()
}
