package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cwbudde/algo-drum/internal/physical/match"
)

// writeStereo writes a 16-bit PCM WAV of two channels, each an exponentially
// decaying tone struck at the same instant. It is the shape of the recording
// this tool is pointed at: one event, two observations.
func writeStereo(t *testing.T, path string, delaySamples int) {
	t.Helper()

	const (
		rate   = 48000
		frames = 48000
	)

	samples := make([]int16, 2*frames)

	for n := range frames {
		seconds := float64(n) / rate

		left := 0.6 * math.Exp(-seconds/0.15) * math.Sin(2*math.Pi*180*seconds)
		samples[2*n] = int16(left * 32767)

		shifted := n - delaySamples
		if shifted < 0 {
			continue
		}

		delayed := float64(shifted) / rate
		right := 0.55 * math.Exp(-delayed/0.15) * math.Sin(2*math.Pi*180*delayed)
		samples[2*n+1] = int16(right * 32767)
	}

	var body bytes.Buffer
	if err := binary.Write(&body, binary.LittleEndian, samples); err != nil {
		t.Fatalf("encoding samples: %v", err)
	}

	var file bytes.Buffer

	data := body.Bytes()

	file.WriteString("RIFF")
	binary.Write(&file, binary.LittleEndian, uint32(36+len(data))) //nolint:errcheck // bytes.Buffer
	file.WriteString("WAVEfmt ")
	binary.Write(&file, binary.LittleEndian, uint32(16))       //nolint:errcheck // bytes.Buffer
	binary.Write(&file, binary.LittleEndian, uint16(1))        //nolint:errcheck // bytes.Buffer
	binary.Write(&file, binary.LittleEndian, uint16(2))        //nolint:errcheck // bytes.Buffer
	binary.Write(&file, binary.LittleEndian, uint32(rate))     //nolint:errcheck // bytes.Buffer
	binary.Write(&file, binary.LittleEndian, uint32(rate*2*2)) //nolint:errcheck // bytes.Buffer
	binary.Write(&file, binary.LittleEndian, uint16(4))        //nolint:errcheck // bytes.Buffer
	binary.Write(&file, binary.LittleEndian, uint16(16))       //nolint:errcheck // bytes.Buffer
	file.WriteString("data")
	binary.Write(&file, binary.LittleEndian, uint32(len(data))) //nolint:errcheck // bytes.Buffer
	file.Write(data)

	if err := os.WriteFile(path, file.Bytes(), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestMeasuresACoincidentPairAndProposesGates(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "coincident.wav")
	writeStereo(t, path, 0)

	var stdout, stderr bytes.Buffer
	if err := run([]string{path}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v (stderr %s)", err, stderr.String())
	}

	printed := stdout.String()
	for _, want := range []string{"2 scorings", "partial frequency", "spectral envelope", "gate"} {
		if !strings.Contains(printed, want) {
			t.Errorf("output does not mention %q:\n%s", want, printed)
		}
	}
}

// TestASpacedPairIsRefused is the guard that matters most. The whole measurement
// rests on the two channels being one observation; on a spaced pair the
// disagreement is mostly two arrival times, and gates derived from it would be
// generously wrong in a way nothing downstream could detect.
func TestASpacedPairIsRefused(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "spaced.wav")
	writeStereo(t, path, 69)

	var stdout, stderr bytes.Buffer

	err := run([]string{path}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() accepted a spaced pair")
	}

	if !strings.Contains(err.Error(), "spaced pair") {
		t.Errorf("run() error = %v, want it to name the spacing", err)
	}

	if err := run([]string{"-allow-spaced", path}, &stdout, &stderr); err != nil {
		t.Errorf("run(-allow-spaced) error = %v", err)
	}
}

func TestRejectsInputItCannotMeasure(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	if err := run(nil, &stdout, &stderr); err == nil {
		t.Error("run() with no files did not fail")
	}

	if err := run([]string{"does-not-exist.wav"}, &stdout, &stderr); err == nil {
		t.Error("run() with a missing file did not fail")
	}
}

// TestGatesAreRoundedUpFromTheMeasuredFloor pins the direction. A gate is what a
// candidate has to beat, so rounding a measured floor down would publish a
// threshold below the floor and make it unreachable again — which is the exact
// defect the measured gates exist to remove.
func TestGatesAreRoundedUpFromTheMeasuredFloor(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "coincident.wav")
	writeStereo(t, path, 0)

	report := Report{}
	if err := measure(&report, path, match.DefaultOptions(), false); err != nil {
		t.Fatalf("measure() error = %v", err)
	}

	summarize(&report)

	for _, definition := range terms() {
		proposed := report.Proposed

		gate := *definition.gate(&proposed)
		if floor := report.Distribution[definition.name].P90; gate < floor {
			t.Errorf("%s: gate %g is below the measured p90 %g", definition.name, gate, floor)
		}
	}
}
