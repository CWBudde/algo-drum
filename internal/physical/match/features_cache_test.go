package match

import (
	"math"
	"math/rand"
	"testing"

	"github.com/cwbudde/algo-dsp/dsp/spectrum"
	"github.com/cwbudde/algo-dsp/dsp/window"
	algofft "github.com/cwbudde/algo-fft"
)

// referenceMagnitudeSpectrum is magnitudeSpectrum as it read before the window
// and plan caches: everything rebuilt from scratch on every call.
func referenceMagnitudeSpectrum(t *testing.T, segment []float64, fftSize int) []float64 {
	t.Helper()

	size := min(len(segment), fftSize)
	coefficients := window.Generate(window.TypeHann, size, window.WithPeriodic())

	input := make([]float64, fftSize)
	for i := range size {
		input[i] = segment[i] * coefficients[i]
	}

	plan, err := algofft.NewPlanReal64(fftSize)
	if err != nil {
		t.Fatal(err)
	}

	bins := make([]complex128, plan.SpectrumLen())
	if err := plan.Forward(bins, input); err != nil {
		t.Fatal(err)
	}

	return spectrum.Magnitude(bins)
}

// TestCachedSpectrumMatchesFreshBuild proves the caches are an optimization
// and not a change: a cached window and a reused plan must produce the same
// bits as building both per call.
func TestCachedSpectrumMatchesFreshBuild(t *testing.T) {
	rng := rand.New(rand.NewSource(7))

	for _, fftSize := range []int{1024, 4096, 16384} {
		for _, length := range []int{fftSize / 3, fftSize, fftSize * 2} {
			segment := make([]float64, length)
			for i := range segment {
				segment[i] = rng.NormFloat64()
			}

			// Run twice so the second call is served entirely from the caches.
			for pass := range 2 {
				got, err := magnitudeSpectrum(segment, fftSize)
				if err != nil {
					t.Fatal(err)
				}

				want := referenceMagnitudeSpectrum(t, segment, fftSize)

				if len(got) != len(want) {
					t.Fatalf("fft=%d len=%d pass=%d: %d bins, want %d",
						fftSize, length, pass, len(got), len(want))
				}

				for i := range want {
					if got[i] != want[i] {
						t.Fatalf("fft=%d len=%d pass=%d bin %d: %v, want %v (delta %g)",
							fftSize, length, pass, i, got[i], want[i],
							math.Abs(got[i]-want[i]))
					}
				}
			}
		}
	}
}

// TestCachedWindowMatchesFreshBuild covers decayAttenuation's window.
func TestCachedWindowMatchesFreshBuild(t *testing.T) {
	for _, length := range []int{1, 2, 64, 4095, 35280} {
		want := window.Generate(window.TypeHann, length, window.WithPeriodic())

		for pass := range 2 {
			got := hannWindow(length)
			if len(got) != len(want) {
				t.Fatalf("length %d pass %d: %d coefficients, want %d",
					length, pass, len(got), len(want))
			}

			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("length %d pass %d index %d: %v, want %v",
						length, pass, i, got[i], want[i])
				}
			}
		}
	}
}
