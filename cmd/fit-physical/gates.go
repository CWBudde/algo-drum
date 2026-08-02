package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"

	"github.com/cwbudde/algo-drum/internal/physical/match"
)

// GateRatios is match.Terms restated in the only unit in which the nine terms
// are comparable with each other: multiples of that term's adoption gate.
//
// The identity this rests on is not an approximation and is worth stating
// plainly, because every reading of a report depends on it. match.DefaultWeights
// sets weight = 1/gate for every term and match.Distance forms the total as the
// plain weighted sum, so
//
//	Total = Σ weight_i · term_i = Σ term_i / gate_i = Σ ratio_i
//
// exactly: a term sitting at its gate contributes exactly 1.0, and the total is
// the plain sum of these nine numbers with nothing else in it. Verified two
// ways — TestGateRatiosSumToTheTotal drives real match.Distance output through
// it, and the shipped fits/fit-tt08x08-lp-hd-series-deep.json reproduces its
// recorded 15.186 to six figures as 1.095 + 1.789 + 1.127 + 4.082 + 2.542 +
// 0.840 + 2.310 + 0.795 + 0.606.
//
// Which is why this type exists at all rather than the reader dividing: the
// ratio is the single most-used derived number in this repository, it was being
// re-derived by hand in throwaway scripts every time a report was read, and a
// raw term is close to meaningless on its own — 14.3 dB of spectral envelope
// error and 25.2 cents of glide error look like the same size of mistake and are
// a factor of five apart in what they cost.
//
// Field names mirror match.Weights rather than match.Terms, because these carry
// no unit: 4.08 is not decibels.
type GateRatios struct {
	PartialFrequency float64 `json:"partialFrequency"`
	PartialLevel     float64 `json:"partialLevel"`
	PartialDecay     float64 `json:"partialDecay"`
	SpectralEnvelope float64 `json:"spectralEnvelope"`
	Envelope         float64 `json:"envelope"`
	Glide            float64 `json:"glide"`
	AttackBalance    float64 `json:"attackBalance"`
	Unmatched        float64 `json:"unmatched"`
	Spurious         float64 `json:"spurious"`
	// Total is the sum of the nine above, which is match.Terms.Total again. It
	// is repeated here so that a reader who has the ratios in hand never has to
	// go back to the raw terms to check they add up.
	Total float64 `json:"total"`
}

// termField is one term ready to be printed: its value, its unit and the gate
// it is read against. The order is the order summarize has always printed in,
// kept identical so the two views of the same nine numbers line up.
type termField struct {
	Name  string
	Unit  string
	Value float64
	Gate  float64
}

// Ratio is Value/Gate — how many "just about audible" errors this term is.
//
// A zero gate is impossible through match.AdoptionGates, which derives every
// gate as 1/weight from a weight set whose entries are all positive, but the
// guard stays: a NaN leaking into a printed table would be read as a measurement
// rather than as a division by nothing.
func (t termField) Ratio() float64 {
	if t.Gate == 0 || math.IsNaN(t.Gate) {
		return math.NaN()
	}

	return t.Value / t.Gate
}

// String is one term as the one-line summaries print it: the value in its own
// unit, then what that is as a multiple of the gate.
//
// Three decimals for the dimensionless terms and one for the rest, which is the
// precision each has always been printed at and the precision each is readable
// at — a hundredth of a decibel is below anything the objective can resolve,
// and a hundredth of an unmatched share is a whole partial.
func (t termField) String() string {
	digits := 1
	if t.Unit == "" {
		digits = 3
	}

	// Cents attach to the number and word-like units stand off it, which is what
	// this tool has always printed and what reads correctly: "76.6¢", "12.5 dB".
	separator := " "
	if t.Unit == "" || t.Unit == "¢" {
		separator = ""
	}

	return fmt.Sprintf("%s %.*f%s%s ×%.2f", t.Name, digits, t.Value, separator, t.Unit, t.Ratio())
}

// termFields pairs each term with its adoption gate.
//
// The gates come from match.AdoptionGates rather than from the weight set the
// run scored under, and the two are the same object stated two ways — AdoptionGates
// is literally 1/DefaultWeights, and the evaluator scores under DefaultWeights.
// Reading a report written by an older build under this build's gates would be
// exactly the mistake weightsFingerprint exists to catch, so the fingerprint is
// what guards this, not a second source of gates here.
func termFields(terms match.Terms) []termField {
	gates := match.AdoptionGates()

	return []termField{
		{"freq", "¢", terms.PartialFrequency, gates.PartialFrequency},
		{"level", "dB", terms.PartialLevel, gates.PartialLevel},
		{"decay", "", terms.PartialDecay, gates.PartialDecay},
		{"spectrum", "dB", terms.SpectralEnvelope, gates.SpectralEnvelope},
		{"envelope", "dB", terms.Envelope, gates.Envelope},
		{"glide", "¢", terms.Glide, gates.Glide},
		{"attack", "dB", terms.AttackBalance, gates.AttackBalance},
		{"unmatched", "", terms.Unmatched, gates.Unmatched},
		{"spurious", "", terms.Spurious, gates.Spurious},
	}
}

// gateRatios divides every term by its gate.
func gateRatios(terms match.Terms) GateRatios {
	fields := termFields(terms)

	ratios := GateRatios{
		PartialFrequency: fields[0].Ratio(),
		PartialLevel:     fields[1].Ratio(),
		PartialDecay:     fields[2].Ratio(),
		SpectralEnvelope: fields[3].Ratio(),
		Envelope:         fields[4].Ratio(),
		Glide:            fields[5].Ratio(),
		AttackBalance:    fields[6].Ratio(),
		Unmatched:        fields[7].Ratio(),
		Spurious:         fields[8].Ratio(),
	}

	// Summed from the ratios rather than copied from terms.Total, so that the
	// nine printed numbers and the total printed beside them are the same
	// arithmetic. If those two ever disagree the objective has stopped being a
	// plain weighted sum, and a reader should see it here rather than be shown a
	// total the column does not add up to.
	for _, field := range fields {
		ratios.Total += field.Ratio()
	}

	return ratios
}

// weightsFingerprint identifies the exact weight set a total was computed under.
//
// A total is a property of a weight set and of nothing else, because tightening
// a gate raises that term's weight and the same raw disagreement then scores
// higher. This repository has already paid for that once: the objective's own
// floor was recorded as 6.54/7.86 from a run made minutes before the gates in
// match.DefaultWeights were edited, and every per-term number in that record was
// right while every total was wrong — the p90 had even moved the *other way*, so
// not even the sign of a comparison was safe. See the note on DefaultWeights.
//
// So a report records which weight set it was scored under, and a tool comparing
// two totals can refuse the comparison rather than perform it wrongly. It is a
// fingerprint rather than the weights themselves — those are in the report too —
// because the question a comparison asks is only "the same, or not".
//
// Hashed over the IEEE-754 bit patterns rather than over formatted decimals: the
// weights are reciprocals of round gates and none of them is exact, so 1/0.55
// must fingerprint as itself and not as whatever %g rounds it to. The tolerance
// span is included because it decides which partials are compared at all, which
// changes every partial term without changing any weight.
func weightsFingerprint(weights match.Weights) string {
	digest := sha256.New()

	for _, value := range []float64{
		weights.PartialFrequency,
		weights.PartialLevel,
		weights.PartialDecay,
		weights.SpectralEnvelope,
		weights.Envelope,
		weights.Glide,
		weights.AttackBalance,
		weights.Unmatched,
		weights.Spurious,
		weights.MatchToleranceCents,
	} {
		_, _ = fmt.Fprintf(digest, "%016x\n", math.Float64bits(value))
	}

	// Twelve hex digits: short enough to quote in a commit message or a table,
	// and 48 bits against the handful of weight sets this repository will ever
	// hold is not a collision risk. It is a comparison key, not a signature.
	return hex.EncodeToString(digest.Sum(nil))[:12]
}
