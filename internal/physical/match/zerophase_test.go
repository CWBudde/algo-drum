package match

import (
	"math"
	"slices"
	"testing"

	"github.com/cwbudde/algo-dsp/dsp/filter/biquad"
	"github.com/cwbudde/algo-dsp/dsp/filter/design"
)

// referenceZeroPhaseLowpass is the plainest statement of what
// zeroPhaseLowpassPair has to compute: one signal, one sample at a time through
// the cascade, forwards and then over the reversed signal.
//
// zeroPhaseLowpassPair reorders that work — two signals interleaved, one
// section over the whole pass, the backward pass walked from the end instead of
// reversing — for the throughput its comment describes. None of those are
// approximations, so the two must agree to the last bit, and this is what says
// so.
func referenceZeroPhaseLowpass(signal, resonances []float64, cutoffHz, sampleRateHz float64) {
	run := func() {
		sections := make([]biquad.Section, len(resonances))
		for k, resonance := range resonances {
			sections[k] = biquad.Section{Coefficients: design.Lowpass(cutoffHz, resonance, sampleRateHz)}
		}

		for n, sample := range signal {
			for k := range sections {
				sample = sections[k].ProcessSample(sample)
			}

			signal[n] = sample
		}
	}

	run()
	slices.Reverse(signal)
	run()
	slices.Reverse(signal)
}

func TestZeroPhaseLowpassPairMatchesPerSignalReference(t *testing.T) {
	signal := synthesize(wellSeparatedTones(), testSampleRate, 1.2)

	// The two cascades heterodyne actually asks for: fourth-order Butterworth
	// for the decay envelopes, second-order for the pitch probe.
	cascades := [][]float64{{0.5411961, 1.3065630}, {math.Sqrt2 / 2}}

	for _, resonances := range cascades {
		for _, cutoffHz := range []float64{10, 20, 40, 60} {
			first := slices.Clone(signal)

			// An independent second signal: the point of the pair is that
			// neither leaks into the other.
			second := make([]float64, len(signal))
			for n, sample := range signal {
				second[n] = 0.37*sample - 0.11*signal[len(signal)-1-n]
			}

			wantFirst := slices.Clone(first)
			wantSecond := slices.Clone(second)
			referenceZeroPhaseLowpass(wantFirst, resonances, cutoffHz, testSampleRate)
			referenceZeroPhaseLowpass(wantSecond, resonances, cutoffHz, testSampleRate)

			zeroPhaseLowpassPair(first, second, resonances, cutoffHz, testSampleRate)

			for n := range first {
				if first[n] != wantFirst[n] || second[n] != wantSecond[n] {
					t.Fatalf("sections=%d cutoff=%v Hz: sample %d is %v/%v, want %v/%v",
						len(resonances), cutoffHz, n,
						first[n], second[n], wantFirst[n], wantSecond[n])
				}
			}
		}
	}
}

// TestZeroPhaseLowpassPairHandlesShortSignals guards the backward pass's
// termination, which counts down to -1 rather than iterating a range.
func TestZeroPhaseLowpassPairHandlesShortSignals(t *testing.T) {
	resonances := []float64{0.5411961, 1.3065630}

	for _, length := range []int{0, 1, 2, 3} {
		first := make([]float64, length)
		second := make([]float64, length)

		for n := range first {
			first[n], second[n] = 1, -1
		}

		zeroPhaseLowpassPair(first, second, resonances, 20, testSampleRate)

		for n := range first {
			if math.IsNaN(first[n]) || math.IsNaN(second[n]) {
				t.Fatalf("length %d: sample %d is NaN", length, n)
			}
		}
	}
}
