package main

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/cwbudde/algo-drum/internal/physical"
)

func TestRenderProducesFiniteAudio(t *testing.T) {
	t.Parallel()

	const sampleRate = 48_000

	config := physical.DefaultPhysicalDrum()
	samples, err := render(config, 100*time.Millisecond, 0.8)
	if err != nil {
		t.Fatalf("render() error = %v", err)
	}
	if len(samples) != sampleRate/10 {
		t.Fatalf("sample count = %d, want %d", len(samples), sampleRate/10)
	}

	peak := 0.0
	for index, sample := range samples {
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			t.Fatalf("sample %d is non-finite: %v", index, sample)
		}

		peak = math.Max(peak, math.Abs(sample))
	}
	if peak == 0 {
		t.Fatal("render produced silence")
	}
}

func TestEncodePCM16WAV(t *testing.T) {
	t.Parallel()

	const sampleRate = 48_000

	samples := []float64{-2, -0.5, 0, 0.5, 2}
	wav, peak, err := encodePCM16WAV(samples, sampleRate)
	if err != nil {
		t.Fatalf("encodePCM16WAV() error = %v", err)
	}
	if peak != 2 {
		t.Fatalf("peak = %v, want 2", peak)
	}
	if got, want := len(wav), wavHeaderBytes+len(samples)*pcm16Bytes; got != want {
		t.Fatalf("WAV length = %d, want %d", got, want)
	}
	if got := string(wav[0:4]); got != "RIFF" {
		t.Fatalf("chunk ID = %q, want RIFF", got)
	}
	if got := string(wav[8:12]); got != "WAVE" {
		t.Fatalf("format = %q, want WAVE", got)
	}
	if got := binary.LittleEndian.Uint32(wav[24:28]); got != sampleRate {
		t.Fatalf("sample rate = %d, want %d", got, sampleRate)
	}
	if got := binary.LittleEndian.Uint32(wav[40:44]); got != uint32(len(samples)*pcm16Bytes) {
		t.Fatalf("data length = %d, want %d", got, len(samples)*pcm16Bytes)
	}
}

func TestRenderRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	config := physical.DefaultPhysicalDrum()
	if _, err := render(config, 0, 1); !errors.Is(err, errInvalidRenderOption) {
		t.Fatalf("render(zero duration) error = %v, want errInvalidRenderOption", err)
	}
	if _, err := render(config, time.Second, 2); !errors.Is(err, physical.ErrInvalidVelocity) {
		t.Fatalf("render(invalid velocity) error = %v, want ErrInvalidVelocity", err)
	}
	if _, _, err := encodePCM16WAV([]float64{math.NaN()}, 48_000); !errors.Is(err, errInvalidRenderOption) {
		t.Fatalf("encodePCM16WAV(NaN) error = %v, want errInvalidRenderOption", err)
	}
}
