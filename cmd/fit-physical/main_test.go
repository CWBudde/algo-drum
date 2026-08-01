package main

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cwbudde/algo-drum/internal/drum"
	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/algo-drum/internal/physical/match"
	"github.com/cwbudde/wav"
	"github.com/go-audio/audio"
)

// writeSyntheticReference builds a WAV of decaying sinusoids, so the CLI can be
// exercised end to end without depending on a recording the repository does not
// ship.
func writeSyntheticReference(t *testing.T) string {
	t.Helper()

	return writeSyntheticReferenceChannels(t, 1)
}

// writeSyntheticReferenceChannels writes the same hit into the requested number
// of channels, so the stereo case — the shape of the repository's own
// reference/tom.wav — can be exercised without shipping a recording. The
// channels carry identical samples: what is under test is which reduction the
// command was *told* to take, not what the reduction sounds like.
func writeSyntheticReferenceChannels(t *testing.T, channels int) string {
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

	interleaved := make([]float32, 0, len(data)*channels)
	for _, sample := range data {
		for range channels {
			interleaved = append(interleaved, sample)
		}
	}

	path := filepath.Join(t.TempDir(), "reference.wav")

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	encoder := wav.NewEncoder(file, sampleRate, 16, channels, 1)
	if err := encoder.Write(&audio.Float32Buffer{
		Format:         &audio.Format{NumChannels: channels, SampleRate: sampleRate},
		Data:           interleaved,
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
		"zero loss scale":   {"-reference", reference, "-loss-scale", "0"},
		"absurd loss scale": {"-reference", reference, "-loss-scale", "1000"},
		"silent velocity":   {"-reference", reference, "-report-only", "-velocity", "0"},
		"loud velocity":     {"-reference", reference, "-report-only", "-velocity", "1.5"},
		// A stereo file with no -channel is the trap this guard closes: the
		// default would have averaged the pair and fitted a signal nobody asked
		// for, silently. reference/tom.wav is exactly this shape.
		"undeclared channel": {"-reference", writeSyntheticReferenceChannels(t, 2), "-report-only"},
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

// TestRunRequiresAStatedChannelForMultiChannelReferences pins the difference
// between a default and a decision. -channel defaults to mono, which is a real
// reduction but a different target from either channel of a stereo capture, and
// reference/tom.wav — everything in docs/physical-measured-fit.md and in
// testdata/physical-fit-tom.json — is stereo fitted from its right channel. A
// defaulted flag once cost a full-budget run, so an unstated reduction of a
// multi-channel file is now an error; the same reduction typed out is a choice
// and is honoured, and a genuinely mono file still needs no flag at all.
func TestRunRequiresAStatedChannelForMultiChannelReferences(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		reference string
		channel   string
		wantError bool
	}{
		"stereo without a channel":     {writeSyntheticReferenceChannels(t, 2), "", true},
		"stereo with mono stated":      {writeSyntheticReferenceChannels(t, 2), "mono", false},
		"stereo with right stated":     {writeSyntheticReferenceChannels(t, 2), "right", false},
		"mono without a channel":       {writeSyntheticReferenceChannels(t, 1), "", false},
		"mono with the default stated": {writeSyntheticReferenceChannels(t, 1), "mono", false},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			args := []string{"-reference", test.reference, "-report-only", "-o", "-"}
			if test.channel != "" {
				args = append(args, "-channel", test.channel)
			}

			err := run(args, io.Discard, io.Discard)

			switch {
			case test.wantError && !errors.Is(err, errInvalidFitOption):
				t.Errorf("run() error = %v, want errInvalidFitOption", err)
			case test.wantError && !errors.Is(err, match.ErrChannelNotChosen):
				t.Errorf("run() error = %v, want match.ErrChannelNotChosen", err)
			case !test.wantError && err != nil:
				t.Errorf("run() error = %v, want nil", err)
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

// TestReportOnlyHonoursTheStrikeVelocity covers the one dimension of the search
// space -fix cannot reach. Re-scoring an archived bank is only apples to apples
// if it is re-scored at the velocity that run recorded, and velocity is not a
// gain the metric divides out — so a report measured at the wrong one quietly
// answers a different question.
func TestReportOnlyHonoursTheStrikeVelocity(t *testing.T) {
	t.Parallel()

	reference := writeSyntheticReference(t)

	baselineVelocity := func(extra ...string) float64 {
		t.Helper()

		path := filepath.Join(t.TempDir(), "report.json")

		args := append([]string{"-reference", reference, "-report-only", "-o", path}, extra...)
		if err := run(args, io.Discard, io.Discard); err != nil {
			t.Fatalf("run(%v) error = %v", extra, err)
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		var decoded struct {
			Baseline struct {
				Velocity01 float64 `json:"velocity01"`
			} `json:"baseline"`
		}

		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}

		return decoded.Baseline.Velocity01
	}

	if got := baselineVelocity("-velocity", "0.5"); got != 0.5 {
		t.Errorf("baseline velocity01 = %v, want 0.5", got)
	}

	// Every invocation written before the flag existed has to keep measuring
	// what it measured, or the archived reports stop being comparable to new
	// ones for exactly the reason the flag was added.
	if got := baselineVelocity(); got != defaultVelocity {
		t.Errorf("default baseline velocity01 = %v, want %v", got, defaultVelocity)
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

// TestLossScaleReachesPastTheDampingRange is the point of the flag: it has to
// move the loss law by exactly its own factor, or a run made to look past DAMP's
// bound measures something other than what it claims to.
func TestLossScaleReachesPastTheDampingRange(t *testing.T) {
	t.Parallel()

	specs := drum.PhysicalTomSpecs()

	bank := make([]float64, len(specs))
	for index, spec := range specs {
		bank[index] = spec.Default
	}

	subject := &evaluator{bank: bank, sampleRateHz: 44100, lossScale: 1}

	unscaled, err := subject.config()
	if err != nil {
		t.Fatalf("config() error = %v", err)
	}

	subject.lossScale = 0.25

	scaled, err := subject.config()
	if err != nil {
		t.Fatalf("config() error = %v", err)
	}

	if got, want := scaled.Batter.Loss0PerSecond, unscaled.Batter.Loss0PerSecond*0.25; got != want {
		t.Errorf("batter Loss0 = %v, want %v", got, want)
	}

	// The resonant head too: scaling one head alone would tilt the drum rather
	// than damp it, and the two are not independently reachable from the bank.
	if got, want := scaled.Resonant.Loss0PerSecond, unscaled.Resonant.Loss0PerSecond*0.25; got != want {
		t.Errorf("resonant Loss0 = %v, want %v", got, want)
	}

	// The frequency tilt is DAMP's companion, not DAMP, so the ratio between a
	// tilted term and an untilted one must survive the scaling untouched.
	before := unscaled.Batter.Loss1MPerSecond / unscaled.Batter.Loss0PerSecond
	after := scaled.Batter.Loss1MPerSecond / scaled.Batter.Loss0PerSecond

	if math.Abs(after-before) > 1e-12 {
		t.Errorf("loss tilt moved: %v -> %v", before, after)
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
