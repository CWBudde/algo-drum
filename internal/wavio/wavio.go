// Package wavio writes mono 16-bit PCM WAV files for offline auditioning.
//
// It exists so that both cmd/render-physical and cmd/fit-physical can export a
// render without one of them importing the other's dependencies: reading a WAV
// lives in internal/physical/match, but that package pulls in the whole FFT and
// feature-extraction stack, which is far more than an encoder needs.
//
// Nothing here runs at audio runtime.
package wavio

import (
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/cwbudde/wav"
	"github.com/go-audio/audio"
)

// ErrInvalidAudio reports samples or a format this package cannot encode.
var ErrInvalidAudio = errors.New("invalid audio for WAV export")

// NormalizedPeak is the peak every export is scaled to.
//
// Exports are peak-normalized because they exist to be listened to and compared
// against a reference that is itself normalized. It also means the caller's
// absolute level cannot clip the file — but it makes the WAV useless for
// measuring level, which is why the true peak is returned rather than discarded.
const NormalizedPeak = 0.9

// WriteMonoPCM16 encodes samples as a mono 16-bit PCM WAV, peak-normalized to
// NormalizedPeak, and returns the source peak before normalization.
func WriteMonoPCM16(writer io.WriteSeeker, samples []float64, sampleRate int) (float64, error) {
	if writer == nil {
		return 0, fmt.Errorf("%w: nil WAV writer", ErrInvalidAudio)
	}

	if sampleRate <= 0 {
		return 0, fmt.Errorf("%w: sample rate %d", ErrInvalidAudio, sampleRate)
	}

	peak := 0.0

	for index, sample := range samples {
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			return 0, fmt.Errorf("%w: non-finite sample %d", ErrInvalidAudio, index)
		}

		peak = math.Max(peak, math.Abs(sample))
	}

	scale := 1.0
	if peak > 0 {
		scale = NormalizedPeak / peak
	}

	data := make([]float32, len(samples))

	for index, sample := range samples {
		data[index] = float32(math.Max(-1, math.Min(1, sample*scale)))
	}

	buffer := &audio.Float32Buffer{
		Format: &audio.Format{
			NumChannels: 1,
			SampleRate:  sampleRate,
		},
		Data: data,
	}
	encoder := wav.NewEncoder(writer, sampleRate, 16, 1, 1)

	if err := encoder.Write(buffer); err != nil {
		return 0, fmt.Errorf("write WAV samples: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return 0, fmt.Errorf("finalize WAV: %w", err)
	}

	return peak, nil
}
