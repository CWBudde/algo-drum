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
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	Variant         string             `json:"variant"`
	Iterations      int                `json:"iterations"`
	Population      int                `json:"population"`
	Restarts        int                `json:"restarts"`
	Seed            int64              `json:"seed"`
	Evaluations     int                `json:"evaluations"`
	DurationSeconds float64            `json:"durationSeconds"`
	Quality         string             `json:"quality"`
	FixedParams     map[string]float64 `json:"fixedParams,omitempty"`
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
	variant := flags.String("variant", "ma", "mayfly variant: ma, desma, olce, eobbma, gsasma, mpma or aoblmoa")
	iterations := flags.Int("iterations", 150, "mayfly iterations per restart")
	population := flags.Int("pop", 20, "mayfly males, and as many females")
	restarts := flags.Int("restarts", 0, "independent seeded runs, best of; 0 picks one per available core")
	seed := flags.Int64("seed", 1, "base RNG seed; restart n uses seed+n")
	reportOnly := flags.Bool("report-only", false, "measure the shipped defaults against the reference and stop")
	progressEvery := flags.Int("progress", 500,
		"print a progress line every N objective evaluations; 0 silences it")

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

	if !*reportOnly {
		offspring := *population - *population%2
		progress := newTracker(stderr, *progressEvery,
			expectedEvaluations(*restarts, *iterations, *population, offspring), time.Now())

		best, evaluations, err := search(
			base, *variant, *iterations, *population, *restarts, *seed, stderr, progress,
		)
		if err != nil {
			return err
		}

		report.Best = &best
		report.Search.Evaluations = evaluations

		_, _ = fmt.Fprintf(stderr, "best:      %s\n", summarize(best.Terms))
	}

	writeSummary(stdout, report)

	return writeReport(*outputPath, report)
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
	base *evaluator,
	variant string,
	iterations, population, restarts int,
	seed int64,
	stderr io.Writer,
	progress *tracker,
) (Candidate, int, error) {
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

			// Progress is reported from inside the objective because mayfly has
			// no per-iteration hook: without this the run says nothing until a
			// whole restart finishes.
			objective := func(position []float64) float64 {
				cost := local.cost(position)
				progress.observe(cost)

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

			candidate, err := local.describe(result.GlobalBest.Position)
			if err != nil {
				results[run] = outcome{err: err}

				return
			}

			_, _ = fmt.Fprintf(stderr, "  restart %d/%d: total %.3f after %d evaluations\n",
				run+1, restarts, result.GlobalBest.Cost, result.FuncEvalCount)

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
			"envelope %.1f dB, glide %.1f¢, attack %.1f dB, unmatched %.3f)",
		terms.Total, terms.PartialFrequency, terms.PartialLevel, terms.PartialDecay,
		terms.SpectralEnvelope, terms.Envelope, terms.Glide, terms.AttackBalance,
		terms.Unmatched,
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

func writeReport(path string, report Report) error {
	if path == "-" {
		return nil
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
