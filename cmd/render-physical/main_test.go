package main

import (
	"errors"
	"math"
	"os"
	"testing"
	"time"

	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/wav"
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

func TestWritePCM16WAV(t *testing.T) {
	t.Parallel()

	const sampleRate = 48_000

	samples := []float64{-2, -0.5, 0, 0.5, 2}
	output, err := os.CreateTemp(t.TempDir(), "physical-*.wav")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := output.Close(); err != nil {
			t.Errorf("close WAV output: %v", err)
		}
	})

	peak, err := writePCM16WAV(output, samples, sampleRate)
	if err != nil {
		t.Fatalf("writePCM16WAV() error = %v", err)
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

func TestRenderRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	config := physical.DefaultPhysicalDrum()
	if _, err := render(config, 0, 1); !errors.Is(err, errInvalidRenderOption) {
		t.Fatalf("render(zero duration) error = %v, want errInvalidRenderOption", err)
	}
	if _, err := render(config, time.Second, 2); !errors.Is(err, physical.ErrInvalidVelocity) {
		t.Fatalf("render(invalid velocity) error = %v, want ErrInvalidVelocity", err)
	}
	output, err := os.CreateTemp(t.TempDir(), "invalid-*.wav")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := output.Close(); err != nil {
			t.Errorf("close invalid WAV output: %v", err)
		}
	})

	if _, err := writePCM16WAV(output, []float64{math.NaN()}, 48_000); !errors.Is(err, errInvalidRenderOption) {
		t.Fatalf("writePCM16WAV(NaN) error = %v, want errInvalidRenderOption", err)
	}
}
