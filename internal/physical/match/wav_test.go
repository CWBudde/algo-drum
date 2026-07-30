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
