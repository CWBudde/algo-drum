package match

import (
	"bytes"
	"errors"
	"io"
	"math"
	"testing"

	"github.com/cwbudde/wav"
	"github.com/go-audio/audio"
)

// seekableBuffer is the io.ReadWriteSeeker the WAV encoder needs and
// bytes.Buffer is not.
type seekableBuffer struct {
	data   []byte
	offset int64
}

func (b *seekableBuffer) Write(p []byte) (int, error) {
	end := b.offset + int64(len(p))
	if end > int64(len(b.data)) {
		b.data = append(b.data, make([]byte, end-int64(len(b.data)))...)
	}

	copy(b.data[b.offset:end], p)
	b.offset = end

	return len(p), nil
}

func (b *seekableBuffer) Read(p []byte) (int, error) {
	if b.offset >= int64(len(b.data)) {
		return 0, io.EOF
	}

	n := copy(p, b.data[b.offset:])
	b.offset += int64(n)

	return n, nil
}

func (b *seekableBuffer) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		b.offset = offset
	case io.SeekCurrent:
		b.offset += offset
	case io.SeekEnd:
		b.offset = int64(len(b.data)) + offset
	}

	return b.offset, nil
}

// encodeWAV writes interleaved frames as 16-bit PCM.
func encodeWAV(t *testing.T, frames [][]float64, sampleRate int) *seekableBuffer {
	t.Helper()

	channels := len(frames[0])
	data := make([]float32, 0, len(frames)*channels)

	for _, frame := range frames {
		for _, sample := range frame {
			data = append(data, float32(sample))
		}
	}

	buffer := &seekableBuffer{}

	encoder := wav.NewEncoder(buffer, sampleRate, 16, channels, 1)
	if err := encoder.Write(&audio.Float32Buffer{
		Format:         &audio.Format{NumChannels: channels, SampleRate: sampleRate},
		Data:           data,
		SourceBitDepth: 16,
	}); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := buffer.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	return buffer
}

func TestDecodeReferenceReducesToOneChannel(t *testing.T) {
	t.Parallel()

	frames := [][]float64{
		{0.50, 0.10},
		{-0.25, 0.75},
		{0.00, -0.50},
	}

	cases := map[Channel][]float64{
		ChannelLeft:  {0.50, -0.25, 0.00},
		ChannelRight: {0.10, 0.75, -0.50},
		ChannelMono:  {0.30, 0.25, -0.25},
	}

	for channel, want := range cases {
		t.Run(string(channel), func(t *testing.T) {
			t.Parallel()

			got, err := decodeReference(encodeWAV(t, frames, 44100), channel)
			if err != nil {
				t.Fatal(err)
			}

			if got.SampleRateHz != 44100 || got.Channels != 2 || got.BitDepth != 16 {
				t.Errorf("format = %v Hz / %d ch / %d bit, want 44100 / 2 / 16",
					got.SampleRateHz, got.Channels, got.BitDepth)
			}

			if len(got.Samples) != len(want) {
				t.Fatalf("samples = %d, want %d", len(got.Samples), len(want))
			}

			// One 16-bit quantum, which is what the round trip through PCM costs.
			const tolerance = 2.0 / 32768

			for i := range want {
				if math.Abs(got.Samples[i]-want[i]) > tolerance {
					t.Errorf("sample %d = %.6f, want %.6f", i, got.Samples[i], want[i])
				}
			}
		})
	}
}

func TestDecodeReferenceRejectsAnUnknownChannel(t *testing.T) {
	t.Parallel()

	source := encodeWAV(t, [][]float64{{0.5, 0.5}}, 44100)

	if _, err := decodeReference(source, Channel("centre")); !errors.Is(err, ErrInvalidReference) {
		t.Errorf("error = %v, want ErrInvalidReference", err)
	}
}

func TestDecodeReferenceRejectsARightChannelThatIsNotThere(t *testing.T) {
	t.Parallel()

	source := encodeWAV(t, [][]float64{{0.5}, {0.25}}, 44100)

	if _, err := decodeReference(source, ChannelRight); !errors.Is(err, ErrInvalidReference) {
		t.Errorf("error = %v, want ErrInvalidReference", err)
	}
}

func TestDecodeReferenceRejectsSomethingThatIsNotAWAV(t *testing.T) {
	t.Parallel()

	if _, err := decodeReference(bytes.NewReader([]byte("not a wav file at all")), ChannelMono); err == nil {
		t.Error("decoding arbitrary bytes succeeded")
	}
}

// A stereo pair of one hit is usually two microphones at different distances.
// Averaging that without taking the delay out is a comb filter, and the notches
// it carves are a property of the microphone spacing rather than of the drum —
// which is exactly the kind of thing a fit will happily chase. This is the
// regression for that: it is the defect that made the tom reference look as
// though it had nine partials between 476 and 700 Hz.
func TestMonoAlignsADelayedPairBeforeSummingIt(t *testing.T) {
	t.Parallel()

	const (
		sampleRate = 44100
		delay      = 69
		frames     = 8192
		toneHz     = 320.0 // sits on the comb's first notch for this delay
	)

	pair := make([][]float64, frames)
	for index := range pair {
		phase := 2 * math.Pi * toneHz * float64(index) / sampleRate
		late := 2 * math.Pi * toneHz * float64(index-delay) / sampleRate

		second := 0.0
		if index >= delay {
			second = 0.5 * math.Sin(late)
		}

		pair[index] = []float64{0.5 * math.Sin(phase), second}
	}

	got, err := decodeReference(encodeWAV(t, pair, sampleRate), ChannelMono)
	if err != nil {
		t.Fatal(err)
	}

	if got.ChannelDelaySamples != delay {
		t.Errorf("measured delay = %d samples, want %d", got.ChannelDelaySamples, delay)
	}

	if got.ChannelCorrelation < 0.9 {
		t.Errorf("aligned correlation = %.3f, want > 0.9", got.ChannelCorrelation)
	}

	// A naive sum puts this tone in the comb's first null and all but erases
	// it. Aligned, the two copies add and the tone survives at full level.
	peak := 0.0
	for _, sample := range got.Samples[delay : frames-delay] {
		peak = math.Max(peak, math.Abs(sample))
	}

	if peak < 0.45 {
		t.Errorf("aligned peak = %.4f, want the tone to survive at ~0.5", peak)
	}
}

// Two unrelated channels must be left alone: shifting one to chase the best of
// a bad set of correlations would invent an alignment rather than recover one.
func TestMonoLeavesAnUncorrelatedPairAlone(t *testing.T) {
	t.Parallel()

	const (
		sampleRate = 44100
		frames     = 8192
	)

	pair := make([][]float64, frames)
	for index := range pair {
		left := 0.4 * math.Sin(2*math.Pi*220*float64(index)/sampleRate)
		right := 0.4 * math.Sin(2*math.Pi*991*float64(index)/sampleRate)
		pair[index] = []float64{left, right}
	}

	got, err := decodeReference(encodeWAV(t, pair, sampleRate), ChannelMono)
	if err != nil {
		t.Fatal(err)
	}

	const tolerance = 4.0 / 32768

	for index := range got.Samples {
		want := (pair[index][0] + pair[index][1]) / 2
		if math.Abs(got.Samples[index]-want) > tolerance {
			t.Fatalf("sample %d = %.6f, want the plain average %.6f",
				index, got.Samples[index], want)
		}
	}
}
