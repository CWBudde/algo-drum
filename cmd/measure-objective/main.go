// Command measure-objective measures the fitting objective's disagreement with
// itself, and derives the adoption gates from it.
//
// A stereo recording of one drum hit contains two observations of one acoustic
// event. Reducing each channel independently and scoring one against the other
// is a direct empirical floor for every term in match.Distance: no model can be
// expected to match a recording more closely than the recording matches itself.
// Each gate is the 90th percentile of that disagreement, and each weight is
// 1/gate, so a candidate at its gate is indistinguishable from a second
// microphone at the same point in space.
//
// The pair has to be *coincident* for this to mean anything. On a spaced pair
// the disagreement is dominated by the two arrival times, which is a property of
// the microphone stand rather than of the estimator, and the gates come out
// generously wrong. The tool reports each file's inter-channel delay and
// correlation for exactly this reason, and refuses a pair whose delay says it is
// not coincident unless -allow-spaced is passed.
//
// This exists because the measurement it does was previously done by a
// standalone reimplementation of match.Distance, kept outside the repository and
// trustworthy only while it reproduced distance.go bit-exactly. That is a
// standing invitation for the two to drift, and the drift would be silent: every
// gate in the objective would go on being quoted from a copy that no longer
// matched the thing it was gating. Here the real Distance is called.
//
//	go run ./cmd/measure-objective reference/tt08x08/lp/hd/v*.wav
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"slices"

	"github.com/cwbudde/algo-drum/internal/physical/match"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "measure-objective: %v\n", err)
		os.Exit(1)
	}
}

// Report is the whole measurement, committed rather than pasted into a document
// by hand.
type Report struct {
	Options match.Options `json:"options"`
	Files   []File        `json:"files"`
	// Scorings is every pairing, both ways round, because Distance is not
	// symmetric: matching is greedy from the reference's side, so which channel
	// is the reference changes the answer.
	Scorings []Scoring `json:"scorings"`
	// Distribution is the per-term order statistics those scorings produce, and
	// is the table the gates are read off.
	Distribution map[string]Statistics `json:"distribution"`
	// Proposed is the gate set implied by this measurement, and the weights that
	// follow from it. It is *proposed* and not applied: DefaultWeights is source
	// code, and moving it is a decision with a paragraph of reasoning attached.
	Proposed match.Weights `json:"proposedGates"`
	// TotalUnderShipped is what these same scorings total under the weights the
	// repository currently ships — the objective's own noise floor, which no fit
	// result below is meaningful.
	TotalUnderShipped Statistics `json:"totalUnderShippedWeights"`
}

// File is one recording, and the evidence that it is a coincident pair.
type File struct {
	Path                string  `json:"path"`
	SampleRateHz        float64 `json:"sampleRateHz"`
	Channels            int     `json:"channels"`
	ChannelDelaySamples int     `json:"channelDelaySamples"`
	ChannelCorrelation  float64 `json:"channelCorrelation"`
}

// Scoring is one channel scored against the other.
type Scoring struct {
	Path      string      `json:"path"`
	Reference string      `json:"reference"`
	Candidate string      `json:"candidate"`
	Terms     match.Terms `json:"terms"`
}

// Statistics is one term's distribution over every scoring.
type Statistics struct {
	Min    float64 `json:"min"`
	Median float64 `json:"median"`
	P75    float64 `json:"p75"`
	P90    float64 `json:"p90"`
	Max    float64 `json:"max"`
}

// term names the nine terms once, so the table, the JSON and the proposed gates
// cannot fall out of step with each other.
type term struct {
	name string
	unit string
	of   func(match.Terms) float64
	gate func(*match.Weights) *float64
	// round is the granularity the proposed gate is rounded up to. A gate is a
	// published threshold, and 115 is a better one to defend than 113.034.
	round float64
}

func terms() []term {
	return []term{
		{
			"partial frequency", "cents", func(t match.Terms) float64 { return t.PartialFrequency },
			func(w *match.Weights) *float64 { return &w.PartialFrequency }, 5,
		},
		{
			"partial level", "dB", func(t match.Terms) float64 { return t.PartialLevel },
			func(w *match.Weights) *float64 { return &w.PartialLevel }, 1,
		},
		{
			"partial decay", "log ratio", func(t match.Terms) float64 { return t.PartialDecay },
			func(w *match.Weights) *float64 { return &w.PartialDecay }, 0.05,
		},
		{
			"spectral envelope", "dB", func(t match.Terms) float64 { return t.SpectralEnvelope },
			func(w *match.Weights) *float64 { return &w.SpectralEnvelope }, 0.5,
		},
		{
			"envelope", "dB", func(t match.Terms) float64 { return t.Envelope },
			func(w *match.Weights) *float64 { return &w.Envelope }, 0.5,
		},
		{
			"glide", "cents", func(t match.Terms) float64 { return t.Glide },
			func(w *match.Weights) *float64 { return &w.Glide }, 10,
		},
		{
			"attack balance", "dB", func(t match.Terms) float64 { return t.AttackBalance },
			func(w *match.Weights) *float64 { return &w.AttackBalance }, 0.1,
		},
		{
			"unmatched share", "", func(t match.Terms) float64 { return t.Unmatched },
			func(w *match.Weights) *float64 { return &w.Unmatched }, 0.05,
		},
		{
			"spurious share", "", func(t match.Terms) float64 { return t.Spurious },
			func(w *match.Weights) *float64 { return &w.Spurious }, 0.05,
		},
	}
}

// coincidentMaxDelay is how far apart two channels may be and still be treated
// as one observation. Two samples at 48 kHz is 42 microseconds, well inside one
// period of anything this objective measures.
const coincidentMaxDelay = 2

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("measure-objective", flag.ContinueOnError)
	flags.SetOutput(stderr)

	defaults := match.DefaultOptions()

	outputPath := flags.String("o", "", "JSON output path, - for stdout, or empty for none")
	quiet := flags.Bool("quiet", false, "suppress the human-readable tables")
	allowSpaced := flags.Bool("allow-spaced", false,
		"measure a pair whose channels are not coincident; the result bounds a "+
			"microphone-position-independent model rather than the estimator")
	analysisSeconds := flags.Float64("analysis-seconds", defaults.AnalysisSeconds,
		"how much of each hit is measured, from the onset")
	maxPartials := flags.Int("max-partials", defaults.MaxPartials, "how many partials to retain")

	if err := flags.Parse(args); err != nil {
		return err
	}

	paths := flags.Args()
	if len(paths) == 0 {
		return fmt.Errorf("no input files: pass one or more stereo recordings of single hits")
	}

	options := defaults
	options.AnalysisSeconds = *analysisSeconds
	options.MaxPartials = *maxPartials

	report := Report{Options: options}

	for _, path := range paths {
		if err := measure(&report, path, options, *allowSpaced); err != nil {
			return err
		}
	}

	if len(report.Scorings) == 0 {
		return fmt.Errorf("no scorings: every input failed to reduce to two channels")
	}

	summarize(&report)

	if !*quiet {
		print(stdout, &report)
	}

	return write(*outputPath, stdout, &report)
}

// measure reduces one file both ways and scores it in both directions.
func measure(report *Report, path string, options match.Options, allowSpaced bool) error {
	left, err := match.LoadReference(path, match.ChannelLeft)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	if left.Channels < 2 {
		return fmt.Errorf("%s: %d channel(s): this measurement needs a stereo pair of one hit",
			path, left.Channels)
	}

	right, err := match.LoadReference(path, match.ChannelRight)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	// The delay is reported on the mono reduction, which is the only one that
	// looks for it.
	sum, err := match.LoadReference(path, match.ChannelMono)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	if !allowSpaced && abs(sum.ChannelDelaySamples) > coincidentMaxDelay {
		return fmt.Errorf(
			"%s: channels are %d samples apart, so this is a spaced pair and its "+
				"disagreement is mostly two arrival times; pass -allow-spaced to measure it anyway",
			path, sum.ChannelDelaySamples,
		)
	}

	report.Files = append(report.Files, File{
		Path:                path,
		SampleRateHz:        left.SampleRateHz,
		Channels:            left.Channels,
		ChannelDelaySamples: sum.ChannelDelaySamples,
		ChannelCorrelation:  sum.ChannelCorrelation,
	})

	leftFeatures, err := match.Extract(left.Samples, left.SampleRateHz, options)
	if err != nil {
		return fmt.Errorf("%s (left): %w", path, err)
	}

	rightFeatures, err := match.Extract(right.Samples, right.SampleRateHz, options)
	if err != nil {
		return fmt.Errorf("%s (right): %w", path, err)
	}

	// Unit weights, so every term is reported in its own unit and the gates are
	// read off the raw disagreement rather than off the scaling being replaced.
	// The match tolerance is not a weight and does come from the shipped set.
	raw := match.Weights{
		PartialFrequency: 1, PartialLevel: 1, PartialDecay: 1,
		SpectralEnvelope: 1, Envelope: 1, Glide: 1, AttackBalance: 1,
		Unmatched: 1, Spurious: 1,
		MatchToleranceCents: match.DefaultWeights().MatchToleranceCents,
	}

	report.Scorings = append(report.Scorings,
		Scoring{path, "left", "right", match.Distance(leftFeatures, rightFeatures, raw)},
		Scoring{path, "right", "left", match.Distance(rightFeatures, leftFeatures, raw)})

	return nil
}

// summarize turns the scorings into the distribution, the proposed gates, and
// the total under the shipped weights.
func summarize(report *Report) {
	report.Distribution = make(map[string]Statistics, len(terms()))
	report.Proposed = match.Weights{MatchToleranceCents: match.DefaultWeights().MatchToleranceCents}

	for _, definition := range terms() {
		values := make([]float64, 0, len(report.Scorings))
		for _, scoring := range report.Scorings {
			values = append(values, definition.of(scoring.Terms))
		}

		statistics := describe(values)
		report.Distribution[definition.name] = statistics

		// Rounded *up*: a gate is what a candidate has to beat, and rounding a
		// measured floor down would set a threshold below the floor.
		*definition.gate(&report.Proposed) = math.Ceil(statistics.P90/definition.round) * definition.round
	}

	shipped := match.DefaultWeights()
	totals := make([]float64, 0, len(report.Scorings))

	for _, scoring := range report.Scorings {
		total := 0.0
		for _, definition := range terms() {
			total += *definition.gate(&shipped) * definition.of(scoring.Terms)
		}

		totals = append(totals, total)
	}

	report.TotalUnderShipped = describe(totals)
}

func describe(values []float64) Statistics {
	ordered := slices.Clone(values)
	slices.Sort(ordered)

	return Statistics{
		Min:    ordered[0],
		Median: quantile(ordered, 0.50),
		P75:    quantile(ordered, 0.75),
		P90:    quantile(ordered, 0.90),
		Max:    ordered[len(ordered)-1],
	}
}

// quantile is the nearest-rank order statistic. Not interpolated: with a few
// dozen scorings an interpolated p90 invents a value between two measurements,
// and a gate should be a number the objective actually produced.
func quantile(ordered []float64, fraction float64) float64 {
	rank := int(math.Ceil(fraction * float64(len(ordered))))

	return ordered[min(max(rank, 1), len(ordered))-1]
}

func print(out io.Writer, report *Report) {
	_, _ = fmt.Fprintf(out, "%d file(s), %d scorings\n\n", len(report.Files), len(report.Scorings))

	_, _ = fmt.Fprintf(out, "%-24s %8s %8s\n", "file", "delay", "corr")

	for _, file := range report.Files {
		_, _ = fmt.Fprintf(out, "%-24s %8d %8.3f\n",
			trimPath(file.Path), file.ChannelDelaySamples, file.ChannelCorrelation)
	}

	_, _ = fmt.Fprintf(out, "\n%-20s %10s %10s %10s %10s %10s %10s\n",
		"term", "min", "median", "p75", "p90", "max", "gate")

	for _, definition := range terms() {
		statistics := report.Distribution[definition.name]
		proposed := report.Proposed

		_, _ = fmt.Fprintf(out, "%-20s %10.3f %10.3f %10.3f %10.3f %10.3f %10.3f\n",
			definition.name, statistics.Min, statistics.Median,
			statistics.P75, statistics.P90, statistics.Max, *definition.gate(&proposed))
	}

	total := report.TotalUnderShipped
	_, _ = fmt.Fprintf(out, "\ntotal under the shipped weights: min %.3f median %.3f p90 %.3f max %.3f\n",
		total.Min, total.Median, total.P90, total.Max)
	_, _ = fmt.Fprint(out, "no fit total below the median is distinguishable from the objective's own noise\n")
}

func trimPath(path string) string {
	if len(path) <= 24 {
		return path
	}

	return "..." + path[len(path)-21:]
}

func write(path string, stdout io.Writer, report *Report) error {
	if path == "" {
		return nil
	}

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding report: %w", err)
	}

	encoded = append(encoded, '\n')

	if path == "-" {
		_, err = stdout.Write(encoded)

		return err
	}

	return os.WriteFile(path, encoded, 0o644)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}

	return value
}
