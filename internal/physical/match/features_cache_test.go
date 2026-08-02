package match

import (
	"math"
	"math/rand"
	"reflect"
	"sync"
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

// TestExtractIsConcurrencySafe is the licence for the scratch pool.
//
// cmd/fit-physical runs one goroutine per restart and every one of them is
// inside Extract at once, so the working buffers may be shared between
// successive extractions but never between simultaneous ones. Run under -race
// this catches the sharing; the comparison catches a buffer handed to two
// callers at once even where the race detector does not see the write.
func TestExtractIsConcurrencySafe(t *testing.T) {
	t.Parallel()

	samples := benchHit(t)
	options := DefaultOptions()

	want, err := Extract(samples, testSampleRate, options)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 8

	results := make([]Features, workers)

	var group sync.WaitGroup

	for worker := range workers {
		group.Add(1)

		go func() {
			defer group.Done()

			got, err := Extract(samples, testSampleRate, options)
			if err != nil {
				t.Error(err)

				return
			}

			results[worker] = got
		}()
	}

	group.Wait()

	for worker, got := range results {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("worker %d disagreed with the serial extraction", worker)
		}
	}
}
