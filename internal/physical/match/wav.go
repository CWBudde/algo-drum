// Package match turns a drum hit — recorded or rendered — into a small set of
// perceptual features, and scores two of them against each other.
//
// It exists next to package analysis rather than inside it because analysis is
// pinned sample-for-sample by a committed reference fixture, while this package
// is expected to grow as the measures are refined. Nothing here is used at
// audio runtime; it is offline tooling for cmd/fit-physical.
package match

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/cwbudde/wav"
)

// ErrInvalidReference reports a reference file this package cannot use.
var ErrInvalidReference = errors.New("invalid reference signal")

// Channel selects how a multi-channel reference is reduced to mono.
type Channel string

const (
	// ChannelMono averages every channel. The default, and usually right: a
	// close-microphone tom recording spread to stereo is mostly the same signal
	// twice, and averaging suppresses the decorrelated room.
	ChannelMono Channel = "mono"
	// ChannelLeft takes the first channel only.
	ChannelLeft Channel = "left"
	// ChannelRight takes the second channel only.
	ChannelRight Channel = "right"
)

// Reference is a decoded reference signal in its own sample rate.
//
// The rate is deliberately *not* converted: the physical model accepts any rate
// from 8 kHz to 384 kHz, so a candidate can be rendered at the reference's rate
// instead, and no resampler ever enters the measurement path.
type Reference struct {
	Samples      []float64
	SampleRateHz float64
	Channels     int
	BitDepth     int
}

// LoadReference reads a WAV file and reduces it to a single channel.
func LoadReference(path string, channel Channel) (Reference, error) {
	file, err := os.Open(path)
	if err != nil {
		return Reference{}, err
	}
	defer func() { _ = file.Close() }()

	return decodeReference(file, channel)
}

func decodeReference(reader io.ReadSeeker, channel Channel) (Reference, error) {
	decoder := wav.NewDecoder(reader)

	buffer, err := decoder.FullPCMBuffer()
	if err != nil {
		return Reference{}, fmt.Errorf("decode reference: %w", err)
	}

	channels := int(decoder.NumChans)
	if channels < 1 {
		return Reference{}, fmt.Errorf("%w: %d channels", ErrInvalidReference, channels)
	}

	sampleRate := float64(decoder.SampleRate)
	if sampleRate <= 0 {
		return Reference{}, fmt.Errorf("%w: sample rate %v", ErrInvalidReference, sampleRate)
	}

	// The decoder already normalizes to ±1 whatever the source bit depth, so
	// nothing is scaled here.
	frames := len(buffer.Data) / channels
	samples := make([]float64, frames)

	switch channel {
	case ChannelLeft, ChannelRight:
		offset := 0
		if channel == ChannelRight {
			offset = 1
		}

		if offset >= channels {
			return Reference{}, fmt.Errorf("%w: no %s channel in %d",
				ErrInvalidReference, channel, channels)
		}

		for frame := range samples {
			samples[frame] = float64(buffer.Data[frame*channels+offset])
		}
	case ChannelMono:
		for frame := range samples {
			sum := 0.0
			for c := range channels {
				sum += float64(buffer.Data[frame*channels+c])
			}

			samples[frame] = sum / float64(channels)
		}
	default:
		return Reference{}, fmt.Errorf("%w: unknown channel %q", ErrInvalidReference, channel)
	}

	if len(samples) == 0 {
		return Reference{}, fmt.Errorf("%w: no samples", ErrInvalidReference)
	}

	return Reference{
		Samples:      samples,
		SampleRateHz: sampleRate,
		Channels:     channels,
		BitDepth:     buffer.SourceBitDepth,
	}, nil
}
