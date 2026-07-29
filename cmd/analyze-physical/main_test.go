package main

import (
	"bytes"
	"encoding/json"
	"testing"

	physicalanalysis "github.com/cwbudde/algo-drum/internal/physical/analysis"
)

func TestRunWritesReport(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{
		"-duration", "0.2",
		"-fft-size", "4096",
		"-pitch-frame", "2048",
		"-pitch-hop", "1024",
		"-peaks", "5",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v, stderr = %s", err, stderr.String())
	}

	var report physicalanalysis.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.SampleCount != 9600 {
		t.Fatalf("sample count = %d, want 9600", report.SampleCount)
	}
	if len(report.SpectralPeaks) != 5 {
		t.Fatalf("spectral peaks = %d, want 5", len(report.SpectralPeaks))
	}
}

func TestRunRejectsPositionalArgument(t *testing.T) {
	t.Parallel()

	err := run([]string{"unexpected"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("run() error = nil, want positional-argument error")
	}
}
