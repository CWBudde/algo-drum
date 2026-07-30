package main

import (
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cwbudde/algo-drum/internal/drum"
	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/wav"
	"github.com/go-audio/audio"
)

// writeSyntheticReference builds a WAV of decaying sinusoids, so the CLI can be
// exercised end to end without depending on a recording the repository does not
// ship.
func writeSyntheticReference(t *testing.T) string {
	t.Helper()

	const (
		sampleRate = 44100
		seconds    = 1.5
	)

	tones := []struct {
		frequencyHz, amplitude, t60Seconds float64
	}{
		{118, 1.00, 1.50},
		{190, 0.35, 1.00},
		{330, 0.20, 0.70},
	}

	data := make([]float32, int(seconds*sampleRate))
	for _, tone := range tones {
		decay := math.Log(1000) / tone.t60Seconds
		for n := range data {
			seconds := float64(n) / sampleRate
			data[n] += float32(tone.amplitude * math.Exp(-decay*seconds) *
				math.Sin(2*math.Pi*tone.frequencyHz*seconds))
		}
	}

	path := filepath.Join(t.TempDir(), "reference.wav")

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	encoder := wav.NewEncoder(file, sampleRate, 16, 1, 1)
	if err := encoder.Write(&audio.Float32Buffer{
		Format:         &audio.Format{NumChannels: 1, SampleRate: sampleRate},
		Data:           data,
		SourceBitDepth: 16,
	}); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestRunRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	reference := writeSyntheticReference(t)

	cases := map[string][]string{
		"no reference":      {},
		"missing file":      {"-reference", filepath.Join(t.TempDir(), "absent.wav")},
		"zero duration":     {"-reference", reference, "-duration", "0"},
		"zero iterations":   {"-reference", reference, "-iterations", "0"},
		"tiny population":   {"-reference", reference, "-pop", "3"},
		"negative restarts": {"-reference", reference, "-restarts", "-1"},
		"unknown quality":   {"-reference", reference, "-quality", "pristine"},
		"unknown parameter": {"-reference", reference, "-fix", "physicalTom.nope=0.5"},
		"unfixable value":   {"-reference", reference, "-fix", "DAMP=2"},
		"malformed fix":     {"-reference", reference, "-fix", "DAMP"},
		"stray argument":    {"-reference", reference, "extra"},
		"unknown channel":   {"-reference", reference, "-channel", "centre"},
		"unknown contact":   {"-reference", reference, "-contact", "impulse"},
		"negative mallet":   {"-reference", reference, "-mallet-g", "-1"},
		"absurd mallet":     {"-reference", reference, "-mallet-g", "5000"},
	}

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if err := run(args, io.Discard, io.Discard); err == nil {
				t.Error("run() succeeded, want an error")
			}
		})
	}
}

func TestRunRejectsFixingEveryParameter(t *testing.T) {
	t.Parallel()

	args := []string{"-reference", writeSyntheticReference(t)}
	for _, spec := range drum.PhysicalTomSpecs() {
		args = append(args, "-fix", spec.ID+"=0.5")
	}

	if err := run(args, io.Discard, io.Discard); !errors.Is(err, errInvalidFitOption) {
		t.Errorf("run() error = %v, want errInvalidFitOption", err)
	}
}

// TestReportOnlyAcceptsFixingEveryParameter is the same bank with no search
// after it, which is how a candidate the search already found gets re-measured
// — against a different quality tier, say. There is nothing left to search, but
// nothing was going to be searched.
func TestReportOnlyAcceptsFixingEveryParameter(t *testing.T) {
	t.Parallel()

	args := []string{"-reference", writeSyntheticReference(t), "-report-only", "-o", "-"}
	for _, spec := range drum.PhysicalTomSpecs() {
		args = append(args, "-fix", spec.ID+"=0.5")
	}

	if err := run(args, io.Discard, io.Discard); err != nil {
		t.Errorf("run() error = %v, want nil", err)
	}
}

func TestReportOnlyMeasuresTheShippedDefaults(t *testing.T) {
	t.Parallel()

	var stdout, stderr strings.Builder

	report := filepath.Join(t.TempDir(), "report.json")

	err := run([]string{
		"-reference", writeSyntheticReference(t),
		"-report-only", "-o", report,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stderr.String(), "baseline:") {
		t.Errorf("stderr did not report a baseline: %q", stderr.String())
	}
	// Every parameter must appear, so the table cannot silently lose one.
	for _, spec := range drum.PhysicalTomSpecs() {
		if !strings.Contains(stdout.String(), spec.Label) {
			t.Errorf("summary is missing %s", spec.Label)
		}
	}

	if info, err := os.Stat(report); err != nil || info.Size() == 0 {
		t.Errorf("report file: %v (size %v)", err, info)
	}
}

// TestContactOverridesReachTheRenderedDrum pins the two settings the fitter
// applies on top of the product bank. Neither is in PhysicalTomSpecs, so if
// they stopped being applied nothing else in the report would notice — a
// -contact hertzian run would quietly fit the prescribed model instead.
func TestContactOverridesReachTheRenderedDrum(t *testing.T) {
	t.Parallel()

	specs := drum.PhysicalTomSpecs()

	bank := make([]float64, len(specs))
	for index, spec := range specs {
		bank[index] = spec.Default
	}

	subject := &evaluator{bank: bank, sampleRateHz: 44100}

	unchanged, err := subject.config()
	if err != nil {
		t.Fatalf("config() error = %v", err)
	}

	if unchanged.Strike.Contact.Model != physical.ContactPrescribed {
		t.Errorf("without an override the model is %q, want %q",
			unchanged.Strike.Contact.Model, physical.ContactPrescribed)
	}

	subject.contact = physical.ContactHertzian
	subject.malletMassKg = 0.005

	overridden, err := subject.config()
	if err != nil {
		t.Fatalf("config() error = %v", err)
	}

	if overridden.Strike.Contact.Model != physical.ContactHertzian {
		t.Errorf("contact model = %q, want %q",
			overridden.Strike.Contact.Model, physical.ContactHertzian)
	}

	if overridden.Strike.MalletMassKg != 0.005 {
		t.Errorf("mallet mass = %v kg, want 0.005", overridden.Strike.MalletMassKg)
	}
}

// TestSearchIsDeterministic is what makes a fit reportable: the same seed must
// give the same answer, or the numbers written into the documentation cannot be
// reproduced.
func TestSearchIsDeterministic(t *testing.T) {
	t.Parallel()

	reference := writeSyntheticReference(t)

	fit := func() string {
		var stdout strings.Builder

		err := run([]string{
			"-reference", reference,
			// The smallest search that still exercises crossover and
			// mutation: determinism is a property of the seeding, not of how
			// long the swarm runs, and this test is on the fast path.
			"-restarts", "2", "-iterations", "1", "-pop", "4", "-seed", "7",
			"-duration", "0.3",
		}, &stdout, io.Discard)
		if err != nil {
			t.Fatalf("run() error = %v", err)
		}

		return stdout.String()
	}

	if first, second := fit(), fit(); first != second {
		t.Errorf("two runs at the same seed differed:\n%s\n---\n%s", first, second)
	}
}
