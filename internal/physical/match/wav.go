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
	"math"
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

	// ChannelDelaySamples is how far the second channel lags the first, and
	// ChannelCorrelation is how alike they are once that lag is taken out.
	//
	// Both are reported because a stereo pair of the same hit is usually two
	// microphones at different distances, and summing it without aligning it
	// first is a comb filter, not a mono reduction. On this repository's own
	// tom reference the offset is 69 samples at 44.1 kHz — 1.56 ms — which
	// combs the sum with a notch at 320 Hz and a peak at 639 Hz. Neither is a
	// property of the drum, and a model fitted to it is fitted to the
	// microphone geometry. ChannelMono therefore aligns before it averages.
	ChannelDelaySamples int
	ChannelCorrelation  float64
}

const (
	// maxAlignmentSeconds bounds the delay search. Microphones metres apart are
	// still inside this; anything beyond it is not a pair of views of one hit,
	// and silently "aligning" it would be worse than leaving it alone.
	maxAlignmentSeconds = 0.02
	// alignmentConfidence is how alike the two channels must be at the found
	// lag before the sum is realigned, and alignmentMargin how much better than
	// simply summing. Two genuinely different signals — a close mic and a room
	// mic, or a stereo overhead pair — correlate poorly at every lag, and
	// shifting one to chase the best of a bad set would invent an alignment
	// rather than recover one. The tom reference clears both easily: 0.94 at
	// its 69-sample lag against 0.36 at zero.
	alignmentConfidence = 0.5
	alignmentMargin     = 0.05
)

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

	reference := Reference{
		Samples:      samples,
		SampleRateHz: sampleRate,
		Channels:     channels,
		BitDepth:     buffer.SourceBitDepth,
	}

	if channels == 2 {
		first, second := deinterleave(buffer.Data, channels)
		maxLag := int(maxAlignmentSeconds * sampleRate)

		// Too short to measure a lag against: the search would fit noise, and
		// "aligning" two unrelated channels is worse than summing them.
		if len(first) > 2*maxLag {
			lag, best := alignment(first, second, maxLag)
			reference.ChannelDelaySamples, reference.ChannelCorrelation = lag, best

			zero := correlationAt(first, second, 0)
			if channel == ChannelMono && lag != 0 && best >= alignmentConfidence && best > zero+alignmentMargin {
				reference.Samples = averageAligned(first, second, lag)
			}
		}
	}

	return reference, nil
}

func deinterleave(data []float32, channels int) (first, second []float64) {
	frames := len(data) / channels
	first = make([]float64, frames)
	second = make([]float64, frames)

	for frame := range frames {
		first[frame] = float64(data[frame*channels])
		second[frame] = float64(data[frame*channels+1])
	}

	return first, second
}

// alignment finds the lag at which the second channel best matches the first,
// and the normalized correlation there. A positive lag means the second channel
// arrives later.
func alignment(first, second []float64, maxLag int) (int, float64) {
	if maxLag < 1 || len(first) == 0 || len(second) == 0 {
		return 0, 0
	}

	bestLag, best := 0, 0.0

	for lag := -maxLag; lag <= maxLag; lag++ {
		if correlation := correlationAt(first, second, lag); math.Abs(correlation) > math.Abs(best) {
			bestLag, best = lag, correlation
		}
	}

	return bestLag, best
}

func correlationAt(first, second []float64, lag int) float64 {
	var product, firstEnergy, secondEnergy float64

	for index := range first {
		shifted := index + lag
		if shifted < 0 || shifted >= len(second) {
			continue
		}

		product += first[index] * second[shifted]
		firstEnergy += first[index] * first[index]
		secondEnergy += second[shifted] * second[shifted]
	}

	if firstEnergy == 0 || secondEnergy == 0 {
		return 0
	}

	return product / math.Sqrt(firstEnergy*secondEnergy)
}

// averageAligned sums the pair with the delay taken out, so the result is a
// mono reduction rather than a comb filter. Frames with no counterpart keep the
// channel that has one; the alternative is a level step at each end.
func averageAligned(first, second []float64, lag int) []float64 {
	out := make([]float64, len(first))

	for index := range first {
		shifted := index + lag
		if shifted < 0 || shifted >= len(second) {
			out[index] = first[index]

			continue
		}

		out[index] = (first[index] + second[shifted]) / 2
	}

	return out
}
