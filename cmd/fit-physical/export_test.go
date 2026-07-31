package main

import (
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/algo-drum/internal/physical/match"
)

func testCandidate() Candidate {
	return Candidate{
		Velocity01: 0.5,
		Config:     physical.DefaultPhysicalDrum(),
	}
}

func TestExportRendersTheCandidatesOwnConfig(t *testing.T) {
	t.Parallel()

	candidate := testCandidate()
	// A tuning nothing else in this test would produce, so decoding it back
	// proves the export used the candidate's config and not a fresh default.
	candidate.Config.Batter.TensionNPerM = 3000

	path := filepath.Join(t.TempDir(), "fit.wav")

	peak, err := exportCandidate(path, candidate, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("exportCandidate() error = %v", err)
	}

	if peak <= 0 || math.IsNaN(peak) {
		t.Fatalf("source peak = %v, want a positive number", peak)
	}

	decoded, err := match.LoadReference(path, match.ChannelMono)
	if err != nil {
		t.Fatalf("read exported WAV: %v", err)
	}

	if decoded.SampleRateHz != candidate.Config.SampleRateHz {
		t.Fatalf("sample rate = %v, want %v", decoded.SampleRateHz, candidate.Config.SampleRateHz)
	}

	want := int(math.Round(0.25 * candidate.Config.SampleRateHz))
	if len(decoded.Samples) != want {
		t.Fatalf("sample count = %d, want %d", len(decoded.Samples), want)
	}

	reference, err := exportedPeak(candidate.Config, candidate.Velocity01, 250*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	if math.Abs(peak-reference) > 1e-12 {
		t.Fatalf("exported peak = %v, want the retuned drum's %v", peak, reference)
	}
}

func exportedPeak(
	config physical.PhysicalDrum,
	velocity01 float64,
	duration time.Duration,
) (float64, error) {
	samples, err := renderConfig(config, velocity01, duration)
	if err != nil {
		return 0, err
	}

	peak := 0.0
	for _, sample := range samples {
		peak = math.Max(peak, math.Abs(sample))
	}

	return peak, nil
}

func TestExportRejectsWhatItCannotWrite(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		path     string
		duration time.Duration
	}{
		{name: "no path", path: "", duration: time.Second},
		{name: "zero duration", path: "fit.wav", duration: 0},
		{name: "negative duration", path: "fit.wav", duration: -time.Second},
		{name: "absurd duration", path: "fit.wav", duration: maxExportDuration + time.Second},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := testCase.path
			if path != "" {
				path = filepath.Join(t.TempDir(), path)
			}

			if _, err := exportCandidate(path, testCandidate(), testCase.duration); !errors.Is(err, errInvalidExport) {
				t.Fatalf("error = %v, want errInvalidExport", err)
			}
		})
	}
}
