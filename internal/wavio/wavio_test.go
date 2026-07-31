package wavio_test

import (
	"errors"
	"math"
	"os"
	"testing"

	"github.com/cwbudde/algo-drum/internal/wavio"
	"github.com/cwbudde/wav"
)

func tempWAV(t *testing.T) *os.File {
	t.Helper()

	output, err := os.CreateTemp(t.TempDir(), "wavio-*.wav")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := output.Close(); err != nil {
			t.Errorf("close WAV output: %v", err)
		}
	})

	return output
}

func TestWriteMonoPCM16RoundTrips(t *testing.T) {
	t.Parallel()

	const sampleRate = 48_000

	samples := []float64{-2, -0.5, 0, 0.5, 2}
	output := tempWAV(t)

	peak, err := wavio.WriteMonoPCM16(output, samples, sampleRate)
	if err != nil {
		t.Fatalf("WriteMonoPCM16() error = %v", err)
	}

	if peak != 2 {
		t.Fatalf("peak = %v, want 2", peak)
	}

	if _, err := output.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	decoder := wav.NewDecoder(output)

	buffer, err := decoder.FullPCMBuffer()
	if err != nil {
		t.Fatalf("decode exported WAV: %v", err)
	}

	if decoder.SampleRate != sampleRate {
		t.Fatalf("sample rate = %d, want %d", decoder.SampleRate, sampleRate)
	}

	if decoder.NumChans != 1 {
		t.Fatalf("channel count = %d, want 1", decoder.NumChans)
	}

	if decoder.BitDepth != 16 {
		t.Fatalf("bit depth = %d, want 16", decoder.BitDepth)
	}

	if len(buffer.Data) != len(samples) {
		t.Fatalf("decoded samples = %d, want %d", len(buffer.Data), len(samples))
	}
}

// The export is peak-normalized, so the loudest sample must land at
// NormalizedPeak however quiet the source was. A fitted bank rendered at a
// fitted velocity is nowhere near full scale, and an export nobody can hear
// would defeat the point of having one.
func TestWriteMonoPCM16NormalizesAQuietSource(t *testing.T) {
	t.Parallel()

	samples := []float64{0, 0.001, -0.0005}
	output := tempWAV(t)

	peak, err := wavio.WriteMonoPCM16(output, samples, 44_100)
	if err != nil {
		t.Fatalf("WriteMonoPCM16() error = %v", err)
	}

	if math.Abs(peak-0.001) > 1e-12 {
		t.Fatalf("reported peak = %v, want 0.001", peak)
	}

	if _, err := output.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	buffer, err := wav.NewDecoder(output).FullPCMBuffer()
	if err != nil {
		t.Fatalf("decode exported WAV: %v", err)
	}

	loudest := 0.0
	for _, sample := range buffer.Data {
		loudest = math.Max(loudest, math.Abs(float64(sample)))
	}

	// One 16-bit step is 1/32768, so that is the tolerance the format allows.
	if math.Abs(loudest-wavio.NormalizedPeak) > 1.0/32768 {
		t.Fatalf("exported peak = %v, want %v", loudest, wavio.NormalizedPeak)
	}
}

func TestWriteMonoPCM16RejectsWhatItCannotEncode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		writer     bool
		samples    []float64
		sampleRate int
	}{
		{name: "nil writer", writer: false, samples: []float64{0.5}, sampleRate: 48_000},
		{name: "zero sample rate", writer: true, samples: []float64{0.5}, sampleRate: 0},
		{name: "NaN sample", writer: true, samples: []float64{math.NaN()}, sampleRate: 48_000},
		{name: "infinite sample", writer: true, samples: []float64{math.Inf(1)}, sampleRate: 48_000},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var writer *os.File
			if testCase.writer {
				writer = tempWAV(t)
			}

			// A nil *os.File in an io.WriteSeeker is not a nil interface, so the
			// guard has to be reached through a genuinely nil interface value.
			if !testCase.writer {
				if _, err := wavio.WriteMonoPCM16(nil, testCase.samples, testCase.sampleRate); !errors.Is(err, wavio.ErrInvalidAudio) {
					t.Fatalf("error = %v, want ErrInvalidAudio", err)
				}

				return
			}

			if _, err := wavio.WriteMonoPCM16(writer, testCase.samples, testCase.sampleRate); !errors.Is(err, wavio.ErrInvalidAudio) {
				t.Fatalf("error = %v, want ErrInvalidAudio", err)
			}
		})
	}
}
