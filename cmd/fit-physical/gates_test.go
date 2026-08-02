package main

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/cwbudde/algo-drum/internal/physical/match"
)

// TestGateRatiosSumToTheTotal is the claim GateRatios is built on, checked
// through the shipped match.Distance rather than by re-deriving its arithmetic
// here: every term divided by its own gate adds up to the total, so a term at
// its gate contributes exactly 1.0.
//
// Driven from real Distance output because the interesting way for this to break
// is not a mistake in gateRatios. It is match.DefaultWeights ceasing to be the
// reciprocal of match.AdoptionGates, or Distance's total ceasing to be the plain
// weighted sum — either of which would leave the arithmetic here correct and the
// number it produces meaningless.
func TestGateRatiosSumToTheTotal(t *testing.T) {
	t.Parallel()

	// Two hits that disagree in every term at once, so no term is zero and a
	// dropped one would show. The frequencies are close enough to match and far
	// enough apart to cost, the levels and ring times differ, one partial on each
	// side has no counterpart, and the glide, envelope and attack readings differ.
	reference := match.Features{
		SampleRateHz: 48000,
		Partials: []match.Partial{
			{FrequencyHz: 212.8, LevelDB: 0, T60Seconds: 1.2},
			{FrequencyHz: 358.0, LevelDB: -8, T60Seconds: 0.9},
			{FrequencyHz: 511.0, LevelDB: -14, T60Seconds: 0.6},
			{FrequencyHz: 702.0, LevelDB: -21, T60Seconds: 0.4},
		},
		GlideCents:    -35,
		GlideMeasured: true,
		Windows: []match.WindowFeature{
			{BandDB: []float64{0, -3, -6, -9}},
		},
		EnvelopeDB:    []float64{0, -4, -9, -15, -22},
		AttackBalance: 3.5,
	}

	candidate := match.Features{
		SampleRateHz: 48000,
		Partials: []match.Partial{
			{FrequencyHz: 218.1, LevelDB: 0, T60Seconds: 0.7},
			{FrequencyHz: 349.2, LevelDB: -3, T60Seconds: 1.4},
			{FrequencyHz: 525.4, LevelDB: -19, T60Seconds: 0.3},
			{FrequencyHz: 618.0, LevelDB: -11, T60Seconds: 0.5},
		},
		GlideCents:    -8,
		GlideMeasured: true,
		Windows: []match.WindowFeature{
			{BandDB: []float64{0, -7, -4, -13}},
		},
		EnvelopeDB:    []float64{0, -1, -12, -11, -26},
		AttackBalance: 1.1,
	}

	terms := match.Distance(reference, candidate, match.DefaultWeights())
	ratios := gateRatios(terms)

	if terms.Total <= 0 {
		t.Fatalf("total %v: the two hits were meant to disagree", terms.Total)
	}

	sum := ratios.PartialFrequency + ratios.PartialLevel + ratios.PartialDecay +
		ratios.SpectralEnvelope + ratios.Envelope + ratios.Glide +
		ratios.AttackBalance + ratios.Unmatched + ratios.Spurious

	if math.Abs(sum-terms.Total) > 1e-9 {
		t.Errorf("nine ratios sum to %v, total is %v", sum, terms.Total)
	}

	if math.Abs(ratios.Total-terms.Total) > 1e-9 {
		t.Errorf("GateRatios.Total %v, terms.Total %v", ratios.Total, terms.Total)
	}

	// Every term must be represented: a ratio silently left at zero would still
	// satisfy the sum above if the term it came from were zero too.
	for _, field := range termFields(terms) {
		if field.Value <= 0 {
			t.Errorf("term %s is zero, so this test is not exercising it", field.Name)
		}

		if math.Abs(field.Ratio()-field.Value/field.Gate) > 1e-12 {
			t.Errorf("term %s: ratio %v against %v/%v", field.Name, field.Ratio(), field.Value, field.Gate)
		}
	}
}

// TestATermAtItsGateContributesOne is the statement of the identity a reader
// uses: whatever the term, sitting exactly at its gate costs 1.0.
func TestATermAtItsGateContributesOne(t *testing.T) {
	t.Parallel()

	gates, weights := match.AdoptionGates(), match.DefaultWeights()

	for _, field := range termFields(match.Terms{}) {
		if field.Gate <= 0 {
			t.Fatalf("term %s has gate %v", field.Name, field.Gate)
		}
	}

	atGate := termFields(match.Terms{
		PartialFrequency: gates.PartialFrequency,
		PartialLevel:     gates.PartialLevel,
		PartialDecay:     gates.PartialDecay,
		SpectralEnvelope: gates.SpectralEnvelope,
		Envelope:         gates.Envelope,
		Glide:            gates.Glide,
		AttackBalance:    gates.AttackBalance,
		Unmatched:        gates.Unmatched,
		Spurious:         gates.Spurious,
	})

	for _, field := range atGate {
		if math.Abs(field.Ratio()-1) > 1e-12 {
			t.Errorf("term %s at its gate contributes %v, not 1", field.Name, field.Ratio())
		}
	}

	// And the gates really are the reciprocals of the weights the run scores
	// under, which is what lets a printed gate caption a measured total.
	if math.Abs(gates.PartialDecay*weights.PartialDecay-1) > 1e-12 {
		t.Errorf("gate and weight are not reciprocal: %v, %v", gates.PartialDecay, weights.PartialDecay)
	}
}

// TestTheWeightsFingerprintTracksTheWeights: the fingerprint must move when any
// weight moves and must not move otherwise, since a comparison tool's only
// question is whether two reports were scored under the same set.
func TestTheWeightsFingerprintTracksTheWeights(t *testing.T) {
	t.Parallel()

	base := match.DefaultWeights()

	if weightsFingerprint(base) != weightsFingerprint(match.DefaultWeights()) {
		t.Fatal("the same weight set fingerprints two ways")
	}

	for name, mutate := range map[string]func(*match.Weights){
		"partialFrequency":    func(w *match.Weights) { w.PartialFrequency *= 1.000001 },
		"spurious":            func(w *match.Weights) { w.Spurious += 1e-9 },
		"matchToleranceCents": func(w *match.Weights) { w.MatchToleranceCents++ },
	} {
		changed := base
		mutate(&changed)

		if weightsFingerprint(changed) == weightsFingerprint(base) {
			t.Errorf("%s moved and the fingerprint did not", name)
		}
	}
}

// TestSchemaDescribesTheReportWithoutARun: -schema answers a question about the
// format, so it must work with no reference, no checkpoint and nothing on disk.
func TestSchemaDescribesTheReportWithoutARun(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	if err := run([]string{"-schema"}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	text := stdout.String()

	// One field from each depth the walk has to reach: the top level, a struct
	// behind a pointer, an element of a slice of structs, and the derived views
	// this change added — which are precisely the fields a reader goes looking
	// for and cannot guess the path of.
	for _, wanted := range []string{
		"weightsFingerprint", "objectiveFloor", "gates",
		"best", "termsVsGate", "params[]", "pinnedAt", "takes[]", "velocity01",
	} {
		if !strings.Contains(text, wanted) {
			t.Errorf("the schema does not mention %q", wanted)
		}
	}

	if stderr.Len() > 0 {
		t.Errorf("schema wrote to stderr: %s", stderr.String())
	}
}
