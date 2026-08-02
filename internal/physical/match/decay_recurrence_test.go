package match

import (
	"math"
	"slices"
	"testing"
)

// realDecayTraceTimes returns the decimated trace times measureDecays built for
// each partial of a synthetic drum hit — the real slices decaySignalStep is
// asked about, rather than an idealised progression written out by hand.
//
// The scratch buffers hold only the last partial measureDecays processed, so the
// takes are collected by handing it one prefix of the detected table at a time.
// The prefix changes that partial's neighbour spacing, and so its envelope
// cutoff, by a few hertz against the full-table run. That is immaterial here:
// the cutoff sets how *long* the fit window is, not how the samples inside it
// are spaced, and the spacing is the whole subject.
func realDecayTraceTimes(tb testing.TB) [][]float64 {
	tb.Helper()

	var times [][]float64

	hit := normalizePeak(benchHit(tb))
	options := DefaultOptions()

	work := acquireExtractScratch()

	detected, err := detectPartials(work, hit, testSampleRate, options)
	if err != nil {
		tb.Fatal(err)
	}

	for count := 1; count <= len(detected); count++ {
		measureDecays(work, hit, testSampleRate, options, slices.Clone(detected[:count]))

		if len(work.fullTimes) < 16 {
			continue
		}

		times = append(times, slices.Clone(work.fullTimes))
	}

	if len(times) == 0 {
		tb.Fatal("no decay traces were built")
	}

	return times
}

// TestDecaySignalRecurrenceDrift is the measurement decaySignalStep's doc
// comment cites, rather than the arithmetic argument it also makes.
//
// The replacement of exp(logPower - a*t_i) by a running product is only sound if
// the products do not walk away from the exponentials over the length of a
// sweep. The bound is ~3,100 roundings of half an ulp, so ~3.4e-13 relative, and
// the measurement below lands at 1.94e-13 — the bound met, not beaten. That is
// still four orders inside the fit's own 1e-9 convergence tolerance, so nothing
// the recurrence does can move a fitted ring time.
func TestDecaySignalRecurrenceDrift(t *testing.T) {
	t.Parallel()

	times := realDecayTraceTimes(t)

	// The ring times the benchmark's own partial table contains, converted to
	// the power decay rate a in exp(-a*t): a T60 is a fall of 60 dB, so
	// a = 60/(decibelsPerPower*T60).
	ringTimes := []float64{0.15, 0.30, 0.62, 1.40}

	worst := 0.0
	longest := 0

	for _, trace := range times {
		longest = max(longest, len(trace))

		stride, uniform := decaySignalStep(trace)
		if !uniform {
			t.Fatalf("a decimated decay trace of %d points was not a uniform progression", len(trace))
		}

		for _, ringTime := range ringTimes {
			rate := 60 / (decibelsPerPower * ringTime)

			// logPower is irrelevant to the relative deviation — it is a common
			// factor of both sequences — so it is held at 0.
			signal := math.Exp(-rate * trace[0])
			ratio := math.Exp(-rate * stride)

			for _, elapsed := range trace {
				exact := math.Exp(-rate * elapsed)
				if exact == 0 {
					// Both sequences have underflowed; there is no relative
					// deviation to speak of below this point.
					break
				}

				worst = max(worst, math.Abs(signal-exact)/exact)
				signal *= ratio
			}
		}
	}

	t.Logf("max relative deviation %.3e over %d traces, longest %d points",
		worst, len(times), longest)

	// Measured: 1.94e-13 over the longest trace this table produces, which is the
	// ~3.4e-13 arithmetic bound met almost exactly rather than beaten. The
	// threshold is that measurement with a factor of five of room, and it is
	// still four orders inside the fit's 1e-9 convergence tolerance.
	if worst > 1e-12 {
		t.Fatalf("recurrence drifted %.3e from the per-point exponential, want <= 1e-12", worst)
	}
}
