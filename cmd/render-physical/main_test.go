package main

import (
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

// The WAV encoder itself moved to internal/wavio, which owns its round-trip and
// rejection tests. What is still this command's own is the render.
func TestRenderRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	config := physical.DefaultPhysicalDrum()
	if _, err := render(config, 0, 1); !errors.Is(err, errInvalidRenderOption) {
		t.Fatalf("render(zero duration) error = %v, want errInvalidRenderOption", err)
	}
	if _, err := render(config, time.Second, 2); !errors.Is(err, physical.ErrInvalidVelocity) {
		t.Fatalf("render(invalid velocity) error = %v, want ErrInvalidVelocity", err)
	}
}
