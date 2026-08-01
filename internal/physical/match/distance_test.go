package match

import (
	"math"
	"slices"
	"testing"
)

func TestDistanceToItselfIsZero(t *testing.T) {
	t.Parallel()

	features := extractTones(t, wellSeparatedTones())

	got := Distance(features, features, DefaultWeights())
	want := Terms{}

	if got != want {
		t.Errorf("Distance(f, f) = %+v, want every term zero", got)
	}
}

// TestWeightsAreReciprocalGates pins the rule that makes the total readable:
// every weight is 1/gate, so a term scoring at its gate contributes exactly 1.0.
//
// It exists because the rule was silently broken once. PartialDecay carried
// 1.0/0.35 against a gate documented as 0.25 everywhere else, so its
// "contribution" column was scaled by a threshold nobody had adopted. The gates
// themselves are the measured reproducibility floor described on DefaultWeights;
// changing one is a decision, and this test makes it one.
func TestWeightsAreReciprocalGates(t *testing.T) {
	t.Parallel()

	gates := []struct {
		name   string
		weight float64
		gate   float64
	}{
		{"partial frequency", DefaultWeights().PartialFrequency, 70},
		{"partial level", DefaultWeights().PartialLevel, 7},
		{"partial decay", DefaultWeights().PartialDecay, 0.55},
		{"spectral envelope", DefaultWeights().SpectralEnvelope, 3.5},
		{"envelope", DefaultWeights().Envelope, 1.5},
		{"glide", DefaultWeights().Glide, 30},
		{"attack balance", DefaultWeights().AttackBalance, 0.9},
		{"unmatched", DefaultWeights().Unmatched, 0.25},
		{"spurious", DefaultWeights().Spurious, 0.25},
	}

	reported := AdoptionGates()
	fields := []float64{
		reported.PartialFrequency, reported.PartialLevel, reported.PartialDecay,
		reported.SpectralEnvelope, reported.Envelope, reported.Glide,
		reported.AttackBalance, reported.Unmatched, reported.Spurious,
	}

	for index, term := range gates {
		if math.Abs(term.weight*term.gate-1) > 1e-12 {
			t.Errorf("%s: weight %g against gate %g contributes %.4f at the gate, want 1.0",
				term.name, term.weight, term.gate, term.weight*term.gate)
		}

		if math.Abs(fields[index]-term.gate) > 1e-9 {
			t.Errorf("%s: AdoptionGates reports %g, want %g", term.name, fields[index], term.gate)
		}
	}
}

// TestDistanceIsolatesWhatChanged is the property that makes the sum readable:
// perturbing one thing about the candidate must move that term and leave the
// others where they were. Without it a total is just a number, and a fit that
// improves it cannot be said to have improved anything in particular.
func TestDistanceIsolatesWhatChanged(t *testing.T) {
	t.Parallel()

	reference := extractTones(t, wellSeparatedTones())
	weights := DefaultWeights()

	t.Run("detuning moves only the frequency term", func(t *testing.T) {
		t.Parallel()

		previous := 0.0

		for _, cents := range []float64{25, 50, 100} {
			detuned := wellSeparatedTones()
			for i := range detuned {
				detuned[i].frequencyHz *= math.Pow(2, cents/1200)
			}

			terms := Distance(reference, extractTones(t, detuned), weights)

			if math.Abs(terms.PartialFrequency-cents) > 4 {
				t.Errorf("%.0f cents detune measured as %.1f cents", cents, terms.PartialFrequency)
			}
			if terms.PartialFrequency <= previous {
				t.Errorf("%.0f cents detune scored %.1f, not more than the previous %.1f",
					cents, terms.PartialFrequency, previous)
			}
			previous = terms.PartialFrequency

			if terms.PartialDecay > 0.05 {
				t.Errorf("detuning moved the decay term to %.3f", terms.PartialDecay)
			}
			if terms.Unmatched != 0 {
				t.Errorf("detuning left %.3f of the reference energy unmatched", terms.Unmatched)
			}
		}
	})

	t.Run("shortening the ring moves only the decay term", func(t *testing.T) {
		t.Parallel()

		damped := wellSeparatedTones()
		for i := range damped {
			damped[i].t60Seconds /= 2
		}

		terms := Distance(reference, extractTones(t, damped), weights)

		// Halved ring times: ln 2 = 0.693.
		if math.Abs(terms.PartialDecay-math.Ln2) > 0.05 {
			t.Errorf("halved T60 measured as %.3f in log ratio, want %.3f", terms.PartialDecay, math.Ln2)
		}
		if terms.PartialFrequency > 2 {
			t.Errorf("damping moved the frequency term to %.2f cents", terms.PartialFrequency)
		}
		if terms.PartialLevel > 1.5 {
			t.Errorf("damping moved the level term to %.2f dB", terms.PartialLevel)
		}
	})

	t.Run("rebalancing moves only the level term", func(t *testing.T) {
		t.Parallel()

		rebalanced := wellSeparatedTones()
		rebalanced[1].amplitude *= 2

		terms := Distance(reference, extractTones(t, rebalanced), weights)

		if terms.PartialLevel < 2 {
			t.Errorf("a 6 dB shift on one partial measured as %.2f dB", terms.PartialLevel)
		}
		if terms.PartialFrequency > 2 {
			t.Errorf("rebalancing moved the frequency term to %.2f cents", terms.PartialFrequency)
		}
		if terms.PartialDecay > 0.05 {
			t.Errorf("rebalancing moved the decay term to %.3f", terms.PartialDecay)
		}
	})
}

// TestUnmatchedIsWeightedByLoudness pins the choice that keeps the reference
// tom's genuine but inaudible 87 Hz component from dominating a fit: a missing
// partial costs what it was worth.
func TestUnmatchedIsWeightedByLoudness(t *testing.T) {
	t.Parallel()

	reference := extractTones(t, wellSeparatedTones())

	withoutQuiet := wellSeparatedTones()
	withoutQuiet = withoutQuiet[:len(withoutQuiet)-1]

	withoutLoud := wellSeparatedTones()[1:]

	quiet := Distance(reference, extractTones(t, withoutQuiet), DefaultWeights())
	loud := Distance(reference, extractTones(t, withoutLoud), DefaultWeights())

	if quiet.Unmatched <= 0 {
		t.Error("dropping a partial left nothing unmatched")
	}

	if loud.Unmatched <= quiet.Unmatched {
		t.Errorf("dropping the loudest partial (%.4f) cost no more than dropping the quietest (%.4f)",
			loud.Unmatched, quiet.Unmatched)
	}

	// The thresholds below are the point of the audibility weighting, and they
	// are deliberately not the ones a power weighting produces. Under
	// 10^(dB/10) the quietest of these four tones is 18.4 dB down and carries
	// under 2 % of the total, so losing it rounded to nothing and a candidate
	// could drop most of a drum for free. Weighted by dB above the detection
	// floor it costs a tenth or so — still much less than the fundamental, but
	// no longer free.
	if quiet.Unmatched < 0.10 {
		t.Errorf("dropping an 18 dB-down partial cost %.4f, want it to cost something", quiet.Unmatched)
	}

	if ratio := loud.Unmatched / quiet.Unmatched; ratio > 3 {
		t.Errorf("loudest cost %.1fx the quietest, want the weighting compressed enough to stay under 3x",
			ratio)
	}
}

// The defect this whole weighting exists to prevent, stated directly.
//
// A reference whose energy sits almost entirely in one partial — which the
// repository's tom reference does, at 99.4 % — used to let a candidate holding
// only that partial score as though it had reproduced the drum: every partial
// term averaged over the single pair that matched, and the unmatched share
// rounded to 0.006. The distance must prefer the drum with all four partials by
// a wide margin.
func TestOneLoudPartialIsNotADrum(t *testing.T) {
	t.Parallel()

	lopsided := []tone{
		{frequencyHz: 120, amplitude: 0.05, t60Seconds: 1.20},
		{frequencyHz: 213, amplitude: 1.00, t60Seconds: 0.60},
		{frequencyHz: 331, amplitude: 0.04, t60Seconds: 0.55},
		{frequencyHz: 512, amplitude: 0.04, t60Seconds: 0.40},
	}

	reference := extractTones(t, lopsided)
	whole := Distance(reference, extractTones(t, lopsided), DefaultWeights())
	sparse := Distance(reference, extractTones(t, lopsided[1:2]), DefaultWeights())

	if sparse.Unmatched < 0.4 {
		t.Errorf("a candidate with only the loudest partial left %.4f unmatched, want a large share",
			sparse.Unmatched)
	}

	if sparse.Total <= whole.Total {
		t.Fatalf("one partial scored %.3f against the whole drum's %.3f", sparse.Total, whole.Total)
	}

	// The blend has to reach the partial terms too, or the sparse candidate
	// still reports an excellent frequency error over its one matched pair.
	if sparse.PartialFrequency <= whole.PartialFrequency*4 {
		t.Errorf("sparse candidate reported %.1f cents against the whole drum's %.1f; "+
			"the unmatched share is not reaching the partial terms",
			sparse.PartialFrequency, whole.PartialFrequency)
	}
}

// The mirror of TestOneLoudPartialIsNotADrum, and the defect that survived it.
//
// Charging for missing partials without charging for invented ones just moves
// the degenerate optimum: the first fit run under the audibility weighting
// covered all seven reference partials, reported an unmatched share of 0.000,
// and made its second-loudest component a 182 Hz mode the reference does not
// have. Only the spectral envelope noticed, and only faintly.
func TestInventedPartialsAreCharged(t *testing.T) {
	t.Parallel()

	reference := extractTones(t, wellSeparatedTones())

	// A loud mode between the second and third partials, far enough from both
	// that no matching tolerance reaches it.
	invented := append(wellSeparatedTones(), tone{frequencyHz: 253, amplitude: 0.5, t60Seconds: 0.7})

	faithful := Distance(reference, extractTones(t, wellSeparatedTones()), DefaultWeights())
	extra := Distance(reference, extractTones(t, invented), DefaultWeights())

	if extra.Unmatched != 0 {
		t.Fatalf("the candidate still covers every reference partial, but unmatched = %.4f", extra.Unmatched)
	}

	if extra.Spurious <= 0.1 {
		t.Errorf("an invented partial 6 dB down scored %.4f spurious, want it to cost something", extra.Spurious)
	}

	if extra.Total <= faithful.Total {
		t.Errorf("inventing a partial scored %.3f against the faithful candidate's %.3f",
			extra.Total, faithful.Total)
	}
}

// TestSpuriousDoesNotOutweighCompleteness is a structural guard, and it is worth
// being explicit that it is one.
//
// Spurious was first weighted at 1/0.2 against Unmatched's 2.0, reasoning that
// nothing else in the sum absorbs an invented partial while a missing one is
// charged twice. A fit run refuted it in fourteen minutes: the search abandoned
// the drum for two partials and a spurious share of 0.000, having found that
// inventing nothing is easiest when there is nothing.
//
// That failure cannot be reproduced as a unit test on the distance. Here,
// dropping a reference partial from a synthetic candidate leaves its invented
// partials untouched, so the total rises monotonically at either weight — which
// was verified before writing this. In the search, coverage and invention move
// together, because both are consequences of the same tuning; the degeneracy
// lives in the composition of the model with the metric, not in the metric. So
// what is pinned here is the inequality the run established, and the reason it
// holds is in DefaultWeights' comment rather than in an assertion.
func TestSpuriousDoesNotOutweighCompleteness(t *testing.T) {
	t.Parallel()

	weights := DefaultWeights()

	if weights.Spurious > weights.Unmatched {
		t.Errorf("spurious weight %.3f exceeds unmatched %.3f; a fit run has already shown "+
			"that this makes discarding the drum the cheapest way to invent nothing",
			weights.Spurious, weights.Unmatched)
	}
}

// Above and below the reference's own partials there is no evidence: a room
// recording's noise floor hides modes a model legitimately has. Charging for
// them would fit the recording's limits rather than the drum.
func TestPartialsOutsideTheReferenceSpanAreNotCharged(t *testing.T) {
	t.Parallel()

	reference := extractTones(t, wellSeparatedTones())
	beyond := append(wellSeparatedTones(), tone{frequencyHz: 780, amplitude: 0.5, t60Seconds: 0.7})

	terms := Distance(reference, extractTones(t, beyond), DefaultWeights())

	if terms.Spurious != 0 {
		t.Errorf("a partial above the reference's highest scored %.4f spurious, want 0", terms.Spurious)
	}
}

// The audibility weighting is meaningless if it disagrees with the level at
// which partials actually stop being detected.
func TestAudibilityFloorTracksTheDetectionFloor(t *testing.T) {
	t.Parallel()

	if got := DefaultOptions().PartialFloorDB; got != partialAudibilityFloorDB {
		t.Fatalf("detection floor %v and audibility floor %v have drifted apart",
			got, partialAudibilityFloorDB)
	}
}

func TestMatchPartialsClaimsEachCandidateOnce(t *testing.T) {
	t.Parallel()

	reference := []Partial{
		{FrequencyHz: 100, LevelDB: 0},
		{FrequencyHz: 104, LevelDB: 0},
	}
	// One candidate sits between the two reference partials. It must be
	// identified with the nearer of them and leave the other unmatched, not be
	// counted twice.
	candidate := []Partial{{FrequencyHz: 101, LevelDB: 0}}

	pairs, unmatched, _ := matchPartials(reference, candidate, 200)

	if len(pairs) != 1 {
		t.Fatalf("pairs = %d, want 1", len(pairs))
	}
	if pairs[0].reference.FrequencyHz != 100 {
		t.Errorf("matched %v Hz, want the nearer 100 Hz", pairs[0].reference.FrequencyHz)
	}
	if math.Abs(unmatched-0.5) > 1e-9 {
		t.Errorf("unmatched energy = %v, want half", unmatched)
	}
}

// TestSilenceIsNeverCheaperThanADrum is the regression for the objective's one
// degenerate optimum.
//
// Errors averaged over matched pairs are zero when there are no pairs, so a
// candidate with a single partial in the wrong place used to score better on
// three terms than any real drum could. The search found it: 11.2 against the
// shipped default's 39.2, from a render that sounded like nothing.
func TestSilenceIsNeverCheaperThanADrum(t *testing.T) {
	t.Parallel()

	reference := extractTones(t, wellSeparatedTones())
	weights := DefaultWeights()

	// One partial, nowhere near any of the reference's four.
	//
	// The ring time has to be long enough that the partial is still there when
	// detection looks, which is what makes this candidate "one partial in the
	// wrong place" rather than "nothing at all". It was 0.05 s, chosen against a
	// 1.2 s analysis window; at the 2.0 s window of PLAN N17 such a tone is 60 dB
	// down before the sustain window even opens, so what got detected was sixteen
	// peaks of the fixture's own noise floor and the term this test is about
	// stopped being the one under test. 0.2 s restores the intent — one partial,
	// Unmatched exactly 1.0 — and the degenerate candidate is still scored at
	// 176 against a wrong drum's far lower total.
	nearlyNothing := extractTones(t, []tone{
		{frequencyHz: 923, amplitude: 1, t60Seconds: 0.2},
	})

	// A tom that is plainly the wrong tom: every partial a whole tone sharp
	// and ringing half as long.
	wrongDrum := wellSeparatedTones()
	for i := range wrongDrum {
		wrongDrum[i].frequencyHz *= math.Pow(2, 200.0/1200)
		wrongDrum[i].t60Seconds /= 2
	}

	empty := Distance(reference, nearlyNothing, weights)
	wrong := Distance(reference, extractTones(t, wrongDrum), weights)

	if empty.Total <= wrong.Total {
		t.Errorf("a one-partial candidate scored %.3f, no worse than a wrong drum's %.3f",
			empty.Total, wrong.Total)
	}

	if empty.Unmatched < 0.99 {
		t.Errorf("unmatched energy = %.3f, want essentially all of it", empty.Unmatched)
	}

	// The blend has to reach the partial terms, not only the Unmatched line.
	if empty.PartialFrequency < unmatchedFrequencyCents*0.9 {
		t.Errorf("frequency term with nothing matched = %.1f cents, want the full penalty",
			empty.PartialFrequency)
	}
	if empty.PartialDecay < unmatchedDecayLogRatio*0.9 {
		t.Errorf("decay term with nothing matched = %.3f, want the full penalty", empty.PartialDecay)
	}
}

// TestTheEstimatorsTailIsTrimmedAndTheModelsIsNot is the one property that makes
// trimming defensible rather than a way of scoring better. A handful of
// catastrophic mis-assignments in a large table is the estimator's known failure
// mode and is trimmed; a candidate that simply fails to produce the reference's
// partials is charged in full, because that failure is not in the trimmed terms
// at all.
func TestTheEstimatorsTailIsTrimmedAndTheModelsIsNot(t *testing.T) {
	t.Parallel()

	// Sixteen partials, spaced far enough apart to match unambiguously.
	reference := make([]Partial, 0, 16)
	for index := range 16 {
		reference = append(reference, Partial{
			FrequencyHz: 120 * float64(index+1),
			LevelDB:     -2 * float64(index),
			T60Seconds:  0.5,
			FitQuality:  1,
		})
	}

	weights := DefaultWeights()

	exact := Distance(
		Features{Partials: reference}, Features{Partials: slices.Clone(reference)}, weights,
	)
	if exact.PartialFrequency != 0 || exact.Unmatched != 0 {
		t.Fatalf("a table against itself scored %+v, want zero", exact)
	}

	// Three of sixteen displaced by a semitone: the size and the count of the
	// estimator tail this trimming exists for.
	mangled := slices.Clone(reference)
	for _, index := range []int{3, 8, 13} {
		mangled[index].FrequencyHz *= 1.05
	}

	tail := Distance(Features{Partials: reference}, Features{Partials: mangled}, weights)
	if tail.PartialFrequency != 0 {
		t.Errorf("three outliers in sixteen measured as %.3f cents, want them trimmed",
			tail.PartialFrequency)
	}

	// The same three partials missing entirely. Nothing about that is trimmed:
	// Unmatched is not a trimmed term, and it is where a missing mode is
	// charged.
	missing := slices.Concat(reference[:3], reference[4:8], reference[9:13], reference[14:])

	absent := Distance(Features{Partials: reference}, Features{Partials: missing}, weights)
	if absent.Unmatched <= 0 {
		t.Errorf("three missing partials scored an unmatched share of %.4f, want them charged",
			absent.Unmatched)
	}
}

// TestTrimmingNeedsATableToTrim pins the small-table behaviour. Below five
// values nothing is discarded, because with four partials there is no basis for
// calling any one of them the estimator's tail.
func TestTrimmingNeedsATableToTrim(t *testing.T) {
	t.Parallel()

	// Four values: a plain RMS, so the single large one shows in full.
	if got, want := trimmedRootMeanSquare([]float64{0, 0, 0, 16}), 2.0; math.Abs(got-want) > 1e-12 {
		t.Errorf("four values trimmed to %.4f, want the plain RMS %.4f", got, want)
	}

	// Five: one discarded.
	if got := trimmedRootMeanSquare([]float64{0, 0, 0, 0, 16}); got != 0 {
		t.Errorf("five values measured %.4f, want the outlier trimmed", got)
	}

	if got := trimmedRootMeanSquare(nil); got != 0 {
		t.Errorf("no values measured %.4f, want 0", got)
	}

	// One value is always retained, so a single-partial table degrades to that
	// partial rather than to zero.
	if got, want := trimmedRootMeanSquare([]float64{9}), 3.0; got != want {
		t.Errorf("one value measured %.4f, want %.4f", got, want)
	}
}
