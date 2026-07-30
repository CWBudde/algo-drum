package match

import (
	"math"
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
		t.Errorf("dropping the fundamental (%.4f) cost no more than dropping the quietest partial (%.4f)",
			loud.Unmatched, quiet.Unmatched)
	}
	// The quietest of the four is 18.4 dB down, so it carries well under a
	// tenth of the partial energy; the loudest carries most of it.
	if quiet.Unmatched > 0.05 {
		t.Errorf("dropping an 18 dB-down partial cost %.4f of the energy, want a small share", quiet.Unmatched)
	}
	if loud.Unmatched < 0.5 {
		t.Errorf("dropping the fundamental cost %.4f of the energy, want most of it", loud.Unmatched)
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

	pairs, unmatched := matchPartials(reference, candidate, 200)

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
	nearlyNothing := extractTones(t, []tone{
		{frequencyHz: 923, amplitude: 1, t60Seconds: 0.05},
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
