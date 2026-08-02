package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cwbudde/algo-drum/internal/physical/match"
	"github.com/cwbudde/algo-drum/internal/wavio"
)

// matchDefaults is the option set every run starts from, named here so the
// health tests read the same analysis span the command does.
func matchDefaults() match.Options { return match.DefaultOptions() }

// syntheticHit writes a WAV of one "drum hit": a sum of exponentially decaying
// sinusoids after a second of silence, so the tool has pre-onset material to
// read a noise floor from and a decay it can fit.
//
// Synthetic rather than a rendered physical Tom, because this test is about the
// command's own arithmetic — ratios, damping ratios, the doublet pick — and a
// signal whose partials are known exactly is the only way to check those
// without also asserting what the model sounds like.

// syntheticFloorDB is how far below the peak the fixtures' noise floor sits.
//
// It is not decoration. The decay estimator these tests run through models a
// partial as an exponential plus a **stationary floor**, and a floor is a fitted
// parameter: a signal that decays smoothly past the 16-bit quantisation step and
// on down has no identifiable one, so the fit trades floor against rate and the
// ring time comes back short. A 0.5 s partial at full scale reaches the PCM16
// floor at 0.80 s, which is the width of the refinement window
// (minimumRefinementSpanSeconds), so the fixture sat exactly on that edge: the
// measured damping ratio moved outside its two per cent tolerance when the
// window moved by 50 ms.
//
// -80 dB puts the floor at 0.67 s instead, leaving an eighth of a second of it
// inside the window to be identified from. Real recordings all have this — the
// reference the tool is pointed at has a room in it — so the fixture is more like
// its target with a floor than without. It is deliberately no louder: at -70 dB
// the noise itself grows peaks the partial detector picks up, and
// TestDoubletWarnsWhenThereIsNoStiffenedBranch found one at 208 Hz in a fixture
// that has nothing but a 150 Hz tone in it.
const syntheticFloorDB = -80

// dither adds a deterministic broadband floor at noiseDB below the signal's peak.
// Deterministic because a test that resynthesises different noise on every run
// reports a different damping ratio on every run.
func dither(samples []float64, noiseDB float64) {
	peak := 0.0
	for _, sample := range samples {
		peak = math.Max(peak, math.Abs(sample))
	}

	if peak <= 0 {
		return
	}

	// A fixed-seed linear congruential generator rather than math/rand, so the
	// sequence is pinned to this file and cannot move under a runtime change.
	state := uint64(0x2545F4914F6CDD1D)
	amplitude := peak * math.Pow(10, noiseDB/20)

	for index := range samples {
		state = state*6364136223846793005 + 1442695040888963407
		uniform := float64(state>>11)/float64(uint64(1)<<53)*2 - 1
		samples[index] += amplitude * uniform
	}
}

func syntheticHit(t *testing.T, dir, name string, frequencies, t60s, amplitudes []float64) string {
	t.Helper()

	const (
		sampleRate = 48000
		leadIn     = sampleRate
		tail       = 2 * sampleRate
	)

	samples := make([]float64, leadIn+tail)

	for index := range frequencies {
		decay := math.Log(1000) / t60s[index]

		for sample := range tail {
			time := float64(sample) / sampleRate
			samples[leadIn+sample] += amplitudes[index] *
				math.Exp(-decay*time) * math.Sin(2*math.Pi*frequencies[index]*time)
		}
	}

	dither(samples, syntheticFloorDB)

	path := filepath.Join(dir, name)

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}

	if _, err := wavio.WriteMonoPCM16(file, samples, sampleRate); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}

	return path
}

func TestRunMeasuresPartialsRatiosAndDamping(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := syntheticHit(t, dir, "hit.wav",
		[]float64{150, 240, 320},
		[]float64{0.5, 0.4, 0.3},
		[]float64{1, 0.5, 0.25})

	var stdout, stderr bytes.Buffer

	err := run([]string{"-o", "-", "-quiet", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v, stderr = %s", err, stderr.String())
	}

	var report Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}

	if len(report.Takes) != 1 {
		t.Fatalf("takes = %d, want 1", len(report.Takes))
	}

	take := report.Takes[0]
	if math.Abs(take.BaseHz-150) > 1 {
		t.Fatalf("base frequency = %.2f Hz, want 150", take.BaseHz)
	}

	found := map[float64]Row{}

	for _, row := range take.Partials {
		for _, want := range []float64{150, 240, 320} {
			if math.Abs(row.FrequencyHz-want) < 2 {
				found[want] = row
			}
		}
	}

	if len(found) != 3 {
		t.Fatalf("resolved %d of 3 planted partials: %+v", len(found), take.Partials)
	}

	if ratio := found[240].RatioToBase; math.Abs(ratio-240.0/150.0) > 0.01 {
		t.Errorf("ratio of the 240 Hz partial = %.3f, want %.3f", ratio, 240.0/150.0)
	}

	// zeta = ln(1000)/(T60*2*pi*f): 0.5 s at 150 Hz is 0.147 %.
	wantZeta := 100 * math.Log(1000) / (0.5 * 2 * math.Pi * 150)
	if zeta := found[150].DampingRatioPercent; math.Abs(zeta-wantZeta) > 0.02*wantZeta {
		t.Errorf("damping ratio = %.4f %%, want %.4f %%", zeta, wantZeta)
	}

	if math.Abs(found[150].T60Seconds-0.5) > 0.05 {
		t.Errorf("T60 = %.3f s, want 0.5", found[150].T60Seconds)
	}
}

// The doublet is the measurement M3 calls the direct read of what
// Cavity.StiffnessScale is fitted to, so the pick has to come out of a pair of
// files whose answer is known: 150 Hz alone, and a 155/180 pair.
func TestRunMeasuresTheDoublet(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	single := syntheticHit(t, dir, "single.wav",
		[]float64{150, 320}, []float64{0.5, 0.3}, []float64{1, 0.2})
	double := syntheticHit(t, dir, "double.wav",
		[]float64{155, 180, 320}, []float64{0.5, 0.4, 0.3}, []float64{1, 0.6, 0.2})

	var stdout, stderr bytes.Buffer

	err := run([]string{"-doublet", "-o", "-", "-quiet", single, double}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v, stderr = %s", err, stderr.String())
	}

	var report Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}

	if report.Doublet == nil {
		t.Fatal("no doublet in the report")
	}

	doublet := *report.Doublet

	if math.Abs(doublet.SingleHz-150) > 1 {
		t.Errorf("single-head frequency = %.2f, want 150", doublet.SingleHz)
	}

	if math.Abs(doublet.LowerHz-155) > 1 {
		t.Errorf("lower branch = %.2f, want 155", doublet.LowerHz)
	}

	if math.Abs(doublet.UpperHz-180) > 1 {
		t.Errorf("upper branch = %.2f, want 180", doublet.UpperHz)
	}

	if want := 180.0 / 155.0; math.Abs(doublet.SplitRatio-want) > 0.01 {
		t.Errorf("split ratio = %.3f, want %.3f", doublet.SplitRatio, want)
	}

	// 320 Hz is above the search window, so it must appear as a candidate and
	// must not have been chosen.
	if len(doublet.Candidates) < 2 {
		t.Errorf("candidates = %+v, want the 180 Hz and 320 Hz partials", doublet.Candidates)
	}
}

// A 1.16 split is inside the band the shipped fit sits in, so it must not warn;
// the rigid formula's ~1.9 is outside the search window entirely and must.
func TestDoubletWarnsWhenThereIsNoStiffenedBranch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	single := syntheticHit(t, dir, "single.wav",
		[]float64{150}, []float64{0.5}, []float64{1})
	double := syntheticHit(t, dir, "double.wav",
		[]float64{155, 900}, []float64{0.5, 0.3}, []float64{1, 0.5})

	var stdout, stderr bytes.Buffer

	err := run([]string{"-doublet", "-o", "-", "-quiet", single, double}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v, stderr = %s", err, stderr.String())
	}

	var report Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}

	if report.Doublet == nil || len(report.Doublet.Warnings) == 0 {
		t.Fatalf("expected a warning that no stiffened branch was found, got %+v", report.Doublet)
	}

	if report.Doublet.UpperHz != 0 {
		t.Errorf("upper branch = %.2f, want none", report.Doublet.UpperHz)
	}
}

func TestRunReportsRepeatabilityAcrossTakes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := syntheticHit(t, dir, "a.wav",
		[]float64{150, 240}, []float64{0.5, 0.4}, []float64{1, 0.5})
	second := syntheticHit(t, dir, "b.wav",
		[]float64{151, 240}, []float64{0.5, 0.4}, []float64{1, 0.5})

	var stdout, stderr bytes.Buffer

	err := run([]string{"-o", "-", "-quiet", first, second}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v, stderr = %s", err, stderr.String())
	}

	var report Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}

	if report.Repeatability == nil {
		t.Fatal("no repeatability block for two takes")
	}

	// One semitone is 100 cents; 150 to 151 Hz is 11.5, so the spread must land
	// near that and cannot be zero.
	if spread := report.Repeatability.BaseSpreadCents; spread < 5 || spread > 20 {
		t.Errorf("base frequency spread = %.1f cents, want about 11.5", spread)
	}
}

// Clipping cannot be produced through wavio, which peak-normalizes every
// export, so the guard is checked on the function that raises it. It matters
// because a clipped attack puts energy at every frequency and the partial table
// above it would be fiction.
func TestHealthWarningsFlagClippingAndANoisyFloor(t *testing.T) {
	t.Parallel()

	floor := -30.0
	health := Health{
		PeakAmplitude:   1,
		ClippedSamples:  12,
		DCOffset:        0.01,
		PreOnsetSeconds: 1,
		PreOnsetFloorDB: &floor,
		AnalyzedSeconds: 2,
	}

	warnings := strings.Join(healthWarnings(health, 48000, matchDefaults()), "\n")

	for _, want := range []string{"full scale", "noise floor", "DC offset"} {
		if !strings.Contains(warnings, want) {
			t.Errorf("warnings do not mention %q:\n%s", want, warnings)
		}
	}
}

// The health block is printed before the partial table because a truncated take
// makes everything under it worthless. This asserts the tool says so rather
// than reporting numbers from it silently.
func TestTruncatedTakeWarns(t *testing.T) {
	t.Parallel()

	const sampleRate = 48000

	dir := t.TempDir()
	path := filepath.Join(dir, "clipped.wav")

	// No lead-in, a square-ish full-scale body: clipped, no pre-roll, and too
	// short for the default analysis span.
	samples := make([]float64, sampleRate/2)
	for index := range samples {
		samples[index] = math.Copysign(1, math.Sin(2*math.Pi*150*float64(index)/sampleRate))
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := wavio.WriteMonoPCM16(file, samples, sampleRate); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	var stdout, stderr bytes.Buffer

	if err := run([]string{path}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v, stderr = %s", err, stderr.String())
	}

	text := stdout.String()
	for _, want := range []string{"before the onset", "the tail is missing"} {
		if !strings.Contains(text, want) {
			t.Errorf("output does not warn about %q:\n%s", want, text)
		}
	}
}

// The series reduction has to say which way a quantity goes, and it has to keep
// saying it when the files are listed in a different order — a coefficient that
// moved with the shell's glob expansion would be evidence of nothing. The
// fixtures ramp the 240 Hz partial's amplitude, so its level relative to the
// 150 Hz base falls monotonically with the take index.
func TestRunReportsSeriesTrendsAndSurvivesReordering(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	var paths []string

	for take, amplitude := range []float64{0.10, 0.18, 0.32, 0.56} {
		paths = append(paths, syntheticHit(t, dir, fmt.Sprintf("v%02d.wav", take+1),
			[]float64{150, 240}, []float64{0.5, 0.4}, []float64{1, amplitude}))
	}

	forward := seriesReport(t, paths)

	reversed := slices.Clone(paths)
	slices.Reverse(reversed)

	backward := seriesReport(t, reversed)

	if forward.Series == nil || backward.Series == nil {
		t.Fatal("no series block for four takes")
	}

	// The correspondence rows are a property of the recordings; the trends are a
	// property of the order they were claimed to be in. Reversing the list must
	// therefore leave the rows themselves untouched — only their columns move —
	// and flip the sign of every coefficient.
	if len(forward.Correspondence.Modes) != len(backward.Correspondence.Modes) {
		t.Fatalf("correspondence has %d rows forwards and %d backwards",
			len(forward.Correspondence.Modes), len(backward.Correspondence.Modes))
	}

	for index, mode := range forward.Correspondence.Modes {
		other := backward.Correspondence.Modes[index]

		cells := slices.Clone(other.LevelDB)
		slices.Reverse(cells)
		other.LevelDB = cells

		if !reflect.DeepEqual(mode, other) {
			t.Errorf("row %d changed when the files were listed in reverse:\n%+v\n%+v",
				index, mode, other)
		}
	}

	for index, item := range forward.Series.Trends {
		other := backward.Series.Trends[index]

		if item.Measured != other.Measured {
			t.Errorf("%s: measured = %v forwards, %v backwards", item.Quantity, item.Measured, other.Measured)

			continue
		}

		if item.Measured && math.Abs(item.Spearman+other.Spearman) > 1e-9 {
			t.Errorf("%s: rho = %+.3f forwards but %+.3f backwards, want the negation",
				item.Quantity, item.Spearman, other.Spearman)
		}
	}
}

// The finding the correspondence table exists for is a partial that some takes
// have and others do not, so that is what the fixture plants: a 320 Hz mode in
// the second half of the series only.
func TestCorrespondenceSeparatesPresentFromAbsent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	var paths []string

	for take := range 4 {
		frequencies, t60s, amplitudes := []float64{150, 240}, []float64{0.5, 0.4}, []float64{1, 0.5}
		if take >= 2 {
			frequencies = append(frequencies, 320)
			t60s = append(t60s, 0.3)
			amplitudes = append(amplitudes, 0.25)
		}

		paths = append(paths, syntheticHit(t, dir, fmt.Sprintf("v%02d.wav", take+1),
			frequencies, t60s, amplitudes))
	}

	report := seriesReport(t, paths)

	if report.Correspondence == nil {
		t.Fatal("no correspondence block for four takes")
	}

	var planted *ModeRow

	for index, mode := range report.Correspondence.Modes {
		if math.Abs(mode.MeanHz-320) < 2 {
			planted = &report.Correspondence.Modes[index]
		}

		// Every row is one mode of one take at most, which is what the tolerance
		// is chosen to guarantee; a row holding two partials of the same take
		// would be the alignment merging distinct modes.
		if mode.Present > len(paths) {
			t.Errorf("row at %.1f Hz is present %d times across %d takes",
				mode.MeanHz, mode.Present, len(paths))
		}
	}

	if planted == nil {
		t.Fatalf("no row near 320 Hz: %+v", report.Correspondence.Modes)
	}

	if planted.Present != 2 {
		t.Errorf("320 Hz row present in %d takes, want 2", planted.Present)
	}

	for take, cell := range planted.LevelDB {
		if (cell != nil) != (take >= 2) {
			t.Errorf("320 Hz row, take %d: cell = %v, want present only from take 2", take, cell)
		}
	}
}

// Crest factor is in the health block because peak amplitude is not enough: a
// normalized file says nothing about the strike. This asserts the tool reads one
// at all and reads it gain-invariantly, which is the whole reason it is there.
func TestCrestFactorIsMeasuredAndGainInvariant(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	loud := syntheticHit(t, dir, "loud.wav", []float64{150, 240}, []float64{0.5, 0.4}, []float64{1, 0.5})
	quiet := syntheticHit(t, dir, "quiet.wav", []float64{150, 240}, []float64{0.5, 0.4}, []float64{0.2, 0.1})

	report := seriesReport(t, []string{loud, quiet})

	first, second := report.Takes[0].Health.CrestFactor, report.Takes[1].Health.CrestFactor
	if first <= 0 {
		t.Fatalf("crest factor = %.3f, want a reading", first)
	}

	// wavio peak-normalizes on export, so the two files differ only in
	// quantization noise; a gain-dependent statistic would still have to agree
	// here, but one that failed to would be broken beyond doubt.
	if math.Abs(first-second) > 0.05*first {
		t.Errorf("crest factor = %.3f and %.3f for the same hit at two gains", first, second)
	}
}

func seriesReport(t *testing.T, paths []string) Report {
	t.Helper()

	var stdout, stderr bytes.Buffer

	if err := run(append([]string{"-series", "-o", "-", "-quiet"}, paths...), &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v, stderr = %s", err, stderr.String())
	}

	var report Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}

	return report
}

func TestRunRejectsBadInvocations(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := syntheticHit(t, dir, "hit.wav", []float64{150}, []float64{0.5}, []float64{1})

	cases := map[string][]string{
		"no files":           {},
		"doublet needs two":  {"-doublet", path},
		"missing input file": {filepath.Join(dir, "absent.wav")},
	}

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if err := run(args, &stdout, &stderr); err == nil {
				t.Fatalf("run(%v) succeeded, want an error", args)
			}
		})
	}
}
