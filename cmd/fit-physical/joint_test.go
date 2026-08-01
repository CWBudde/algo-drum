package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cwbudde/algo-drum/internal/physical/match"
)

// jointEvaluator builds an evaluator over takeCount synthetic references, so
// the joint path can be exercised without a recording. Every reference is the
// model's own output at a different velocity, which is what a velocity series
// is, and keeps the test independent of reference/.
func jointEvaluator(tb testing.TB, velocities []float64) *evaluator {
	tb.Helper()

	const (
		sampleRateHz    = 44100
		durationSeconds = 0.3
	)

	bank, free, err := resolveFixed(assignmentFlag{}, true, false)
	if err != nil {
		tb.Fatalf("resolveFixed: %v", err)
	}

	options := match.DefaultOptions()
	options.AnalysisSeconds = durationSeconds
	options.DecayFitEndSeconds = durationSeconds / 2

	probe := &evaluator{
		options:         options,
		weights:         match.DefaultWeights(),
		bank:            bank,
		free:            free,
		sampleRateHz:    sampleRateHz,
		durationSeconds: durationSeconds,
		buffer:          make([]float64, int(durationSeconds*sampleRateHz)),
	}

	for _, velocity := range velocities {
		samples, err := probe.render(velocity)
		if err != nil {
			tb.Fatalf("render at %v: %v", velocity, err)
		}

		target, err := match.Extract(samples, sampleRateHz, options)
		if err != nil {
			tb.Fatalf("extract at %v: %v", velocity, err)
		}

		probe.references = append(probe.references, target)
		probe.referencePaths = append(probe.referencePaths,
			fmt.Sprintf("synthetic-v%.2f", velocity))
	}

	probe.velocities = make([]float64, len(probe.references))
	probe.rendered = make([]match.Features, len(probe.references))

	return probe
}

// TestJointSearchGivesEveryTakeItsOwnVelocity is the structural half of the
// claim that a joint fit does not read the file order: the search vector has one
// velocity per take, and each one reaches only its own take.
func TestJointSearchGivesEveryTakeItsOwnVelocity(t *testing.T) {
	t.Parallel()

	probe := jointEvaluator(t, []float64{0.3, 0.9})

	if got, want := probe.dimensions(), len(probe.free)+2; got != want {
		t.Fatalf("dimensions = %d, want %d", got, want)
	}

	position := probe.position(0.5)
	position[len(probe.free)] = 0.25
	position[len(probe.free)+1] = 0.75

	candidate, err := probe.describe(position)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}

	if len(candidate.Takes) != 2 {
		t.Fatalf("takes = %d, want 2", len(candidate.Takes))
	}

	if got := candidate.Takes[0].Velocity01; got != 0.25 {
		t.Errorf("take 1 velocity = %v, want 0.25", got)
	}

	if got := candidate.Takes[1].Velocity01; got != 0.75 {
		t.Errorf("take 2 velocity = %v, want 0.75", got)
	}
}

// TestJointCostIgnoresTheOrderTheTakesWereGivenIn is the behavioural half, and
// the one that matters for a hand-played series whose v01…v16 labelling may not
// be in strike order. Reversing the takes and their velocities together must
// leave the cost bit-identical: if it did not, the fit would be reading
// something from the file order, and a mislabelled series would score
// differently from the same recordings correctly labelled.
func TestJointCostIgnoresTheOrderTheTakesWereGivenIn(t *testing.T) {
	t.Parallel()

	forward := jointEvaluator(t, []float64{0.3, 0.9})
	reversed := jointEvaluator(t, []float64{0.9, 0.3})

	position := forward.position(0.5)
	position[len(forward.free)] = 0.4
	position[len(forward.free)+1] = 0.8

	swapped := append([]float64(nil), position...)
	swapped[len(forward.free)] = 0.8
	swapped[len(forward.free)+1] = 0.4

	got, want := reversed.cost(swapped), forward.cost(position)
	if got != want {
		t.Fatalf("cost with the takes reversed = %v, want the same %v", got, want)
	}
}

// TestJointRenderMatchesASingleTakeRender pins the optimization measure relies
// on: one model serves every take, Reset between them, on the claim that Reset
// leaves it in the state NewDoubleHead would. Bit-exact, not close — the
// checkpoint fingerprint carries the baseline cost, so a difference of one bit
// makes two runs of the same search refuse to resume each other.
func TestJointRenderMatchesASingleTakeRender(t *testing.T) {
	t.Parallel()

	probe := jointEvaluator(t, []float64{0.3, 0.9})

	position := probe.position(0.5)
	position[len(probe.free)] = 0.4
	position[len(probe.free)+1] = 0.8

	joint, err := probe.measure(position)
	if err != nil {
		t.Fatalf("measure: %v", err)
	}

	second := joint[1]

	// A fresh evaluator, so the second take is the *first* thing it renders and
	// no Reset is involved on this side of the comparison.
	solo := jointEvaluator(t, []float64{0.9})

	soloPosition := solo.position(0.8)

	for index := range solo.free {
		soloPosition[index] = position[index]
	}

	rendered, err := solo.measure(soloPosition)
	if err != nil {
		t.Fatalf("measure solo: %v", err)
	}

	if len(second.Partials) != len(rendered[0].Partials) {
		t.Fatalf("partial count = %d after a reset, want the fresh model's %d",
			len(second.Partials), len(rendered[0].Partials))
	}

	for index, partial := range second.Partials {
		fresh := rendered[0].Partials[index]
		if partial.FrequencyHz != fresh.FrequencyHz || partial.LevelDB != fresh.LevelDB {
			t.Fatalf("partial %d = %.6f Hz / %.6f dB after a reset, want the fresh model's %.6f / %.6f",
				index, partial.FrequencyHz, partial.LevelDB, fresh.FrequencyHz, fresh.LevelDB)
		}
	}
}

// TestJointAggregateIsTheMeanOfTheTakes pins the aggregation, which is the one
// decision in the joint objective that could quietly become something else. A
// trimmed or median aggregate would drop whichever hits the model fits worst,
// which is the evidence a joint fit exists to use.
func TestJointAggregateIsTheMeanOfTheTakes(t *testing.T) {
	t.Parallel()

	probe := jointEvaluator(t, []float64{0.3, 0.9})

	position := probe.position(0.5)

	candidate, err := probe.describe(position)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}

	want := 0.0
	for _, take := range candidate.Takes {
		want += take.Terms.Total
	}

	want /= float64(len(candidate.Takes))

	if math.Abs(candidate.Terms.Total-want) > 1e-12 {
		t.Errorf("aggregate total = %v, want the mean %v", candidate.Terms.Total, want)
	}

	if got := probe.cost(position); math.Abs(got-want) > 1e-12 {
		t.Errorf("cost = %v, want the same mean %v", got, want)
	}
}

// TestRunFitsEveryReferenceGiven is the end-to-end check that a repeated
// -reference reaches the report: as many references, targets and baseline takes
// as files, each named.
func TestRunFitsEveryReferenceGiven(t *testing.T) {
	t.Parallel()

	first := writeSyntheticReference(t)
	second := writeNamedSyntheticReference(t, "second.wav")

	path := filepath.Join(t.TempDir(), "report.json")

	err := run([]string{
		"-reference", first, "-reference", second,
		"-report-only", "-o", path,
	}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var decoded struct {
		References []struct {
			Path string `json:"path"`
		} `json:"references"`
		Targets  []json.RawMessage `json:"targets"`
		Baseline struct {
			Takes []struct {
				Path       string  `json:"path"`
				Velocity01 float64 `json:"velocity01"`
			} `json:"takes"`
		} `json:"baseline"`
	}

	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}

	if len(decoded.References) != 2 || len(decoded.Targets) != 2 {
		t.Fatalf("references = %d, targets = %d, want 2 of each",
			len(decoded.References), len(decoded.Targets))
	}

	if len(decoded.Baseline.Takes) != 2 {
		t.Fatalf("baseline takes = %d, want 2", len(decoded.Baseline.Takes))
	}

	for index, want := range []string{first, second} {
		if got := decoded.References[index].Path; got != want {
			t.Errorf("reference %d = %q, want %q", index, got, want)
		}

		if got := decoded.Baseline.Takes[index].Path; got != want {
			t.Errorf("take %d = %q, want %q", index, got, want)
		}
	}
}

// TestRunRejectsAnUnusableReferenceList covers the three ways a list of takes
// can be wrong in a way no later number would reveal.
func TestRunRejectsAnUnusableReferenceList(t *testing.T) {
	t.Parallel()

	reference := writeSyntheticReference(t)
	other := writeNamedSyntheticReference(t, "other.wav")

	cases := []struct {
		name string
		args []string
	}{
		{
			// Scored twice, and so weighted double in the mean.
			name: "the same take twice",
			args: []string{"-reference", reference, "-reference", reference},
		},
		{
			// No take to render at that velocity.
			name: "a wav take past the end",
			args: []string{"-reference", reference, "-reference", other, "-wav-take", "3"},
		},
		{
			name: "a wav take before the start",
			args: []string{"-reference", reference, "-wav-take", "0"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			args := append(append([]string{}, testCase.args...), "-report-only", "-o", "-")
			if err := run(args, io.Discard, io.Discard); !errors.Is(err, errInvalidFitOption) {
				t.Fatalf("error = %v, want errInvalidFitOption", err)
			}
		})
	}
}

// TestRunRejectsTakesAtDifferentSampleRates. Every take is rendered from one
// buffer at one rate, and no resampler is allowed into the measurement path, so
// a mixed-rate list is refused rather than converted.
func TestRunRejectsTakesAtDifferentSampleRates(t *testing.T) {
	t.Parallel()

	reference := writeSyntheticReference(t)
	other := writeSyntheticReferenceAt(t, "48k.wav", 48000)

	err := run([]string{
		"-reference", reference, "-reference", other,
		"-report-only", "-o", "-",
	}, io.Discard, io.Discard)

	if !errors.Is(err, errInvalidFitOption) {
		t.Fatalf("error = %v, want errInvalidFitOption", err)
	}

	if !strings.Contains(err.Error(), "Hz") {
		t.Errorf("error = %q, want it to name the two rates", err)
	}
}

// TestWriteTakesReportsWhereTheOrderBreaks. The velocity series is named in what
// the source pack says is increasing strike order and was played by hand, so
// the fitted velocities are an independent read on that claim. The summary has
// to say when the two disagree; silence would read as agreement.
func TestWriteTakesReportsWhereTheOrderBreaks(t *testing.T) {
	t.Parallel()

	rising := Candidate{Takes: []TakeResult{
		{Path: "v01.wav", Velocity01: 0.2},
		{Path: "v02.wav", Velocity01: 0.5},
		{Path: "v03.wav", Velocity01: 0.8},
	}}

	var out strings.Builder

	writeTakes(&out, rising)

	if !strings.Contains(out.String(), "rise monotonically") {
		t.Errorf("summary = %q, want it to report a monotone series", out.String())
	}

	// The middle two swapped: one step down out of two.
	swapped := Candidate{Takes: []TakeResult{
		{Path: "v01.wav", Velocity01: 0.2},
		{Path: "v02.wav", Velocity01: 0.8},
		{Path: "v03.wav", Velocity01: 0.5},
	}}

	out.Reset()
	writeTakes(&out, swapped)

	if !strings.Contains(out.String(), "fall at 1 of 2 steps") {
		t.Errorf("summary = %q, want it to count the inversion", out.String())
	}

	// A baseline strikes every take at one velocity. Reading an ordering out of
	// a constant would be reporting the flag, not the drum.
	flat := Candidate{Takes: []TakeResult{
		{Path: "v01.wav", Velocity01: 1},
		{Path: "v02.wav", Velocity01: 1},
		{Path: "v03.wav", Velocity01: 1},
	}}

	out.Reset()
	writeTakes(&out, flat)

	if !strings.Contains(out.String(), "says nothing about the order") {
		t.Errorf("summary = %q, want it to decline to judge a flat series", out.String())
	}

	// A single take has no order to report on, and saying nothing is right.
	out.Reset()
	writeTakes(&out, Candidate{Takes: []TakeResult{{Path: "v01.wav", Velocity01: 0.5}}})

	if out.Len() != 0 {
		t.Errorf("summary = %q, want nothing for a single take", out.String())
	}
}
