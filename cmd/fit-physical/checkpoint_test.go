package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tinySearch is the smallest run that still exercises the whole path. The
// numbers are chosen for speed, not for quality of fit.
func tinySearch(reference, checkpoint string, extra ...string) []string {
	args := []string{
		"-reference", reference,
		"-restarts", "2", "-iterations", "2", "-pop", "4",
		"-duration", "0.4", "-progress", "0", "-o", os.DevNull,
	}

	if checkpoint != "" {
		args = append(args, "-checkpoint", checkpoint)
	}

	return append(args, extra...)
}

// TestCheckpointResumesFinishedRestarts is the property the whole file exists
// for: a second run must adopt what the first one finished instead of paying
// for it again, and must reach the same answer either way.
func TestCheckpointResumesFinishedRestarts(t *testing.T) {
	t.Parallel()

	reference := writeSyntheticReference(t)
	path := filepath.Join(t.TempDir(), "fit.checkpoint")

	var first, firstErrors strings.Builder
	if err := run(tinySearch(reference, path), &first, &firstErrors); err != nil {
		t.Fatalf("first run: %v", err)
	}

	if strings.Contains(firstErrors.String(), "from the checkpoint") {
		t.Error("the first run resumed from a checkpoint that did not exist yet")
	}

	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("checkpoint file: %v (size %v)", err, info)
	}

	var second, secondErrors strings.Builder
	if err := run(tinySearch(reference, path), &second, &secondErrors); err != nil {
		t.Fatalf("resumed run: %v", err)
	}

	if !strings.Contains(secondErrors.String(), "resuming:  2 of 2 restarts") {
		t.Errorf("the resumed run did not report resuming:\n%s", secondErrors.String())
	}

	// Same report from a resume as from the original run — the point of storing
	// positions rather than measurements is that the second run re-derives the
	// numbers instead of trusting them.
	if first.String() != second.String() {
		t.Error("the resumed run produced a different report")
	}
}

// TestCheckpointRefusesAnotherRunsFile guards the failure that would be worst:
// a best-of taken across two different models or two different references,
// which nothing downstream would reveal.
func TestCheckpointRefusesAnotherRunsFile(t *testing.T) {
	t.Parallel()

	reference := writeSyntheticReference(t)
	path := filepath.Join(t.TempDir(), "fit.checkpoint")

	var stdout, stderr strings.Builder
	if err := run(tinySearch(reference, path), &stdout, &stderr); err != nil {
		t.Fatalf("first run: %v", err)
	}

	err := run(tinySearch(reference, path, "-contact", "hertzian"), &stdout, &stderr)
	if !errors.Is(err, errCheckpointMismatch) {
		t.Fatalf("error = %v, want errCheckpointMismatch", err)
	}

	if !strings.Contains(err.Error(), "contact") {
		t.Errorf("the error does not name the field that changed: %v", err)
	}
}

// TestFingerprintNamesWhatChanged covers the entries a CLI test cannot reach,
// above all the baseline cost — the one field that catches an edit to the model
// or the measurement rather than to a flag.
func TestFingerprintNamesWhatChanged(t *testing.T) {
	t.Parallel()

	base := Fingerprint{
		Reference: "tom.wav", Channel: "mono", Quality: "draft", Variant: "ma",
		DurationSeconds: 1.2, Iterations: 80, Population: 16, Restarts: 8, Seed: 1,
		Fixed: map[string]float64{"DAMP": 0.5}, BaselineCost: 33.455,
	}

	if field := base.disagreement(base); field != "" {
		t.Errorf("a fingerprint disagrees with itself over %q", field)
	}

	cases := map[string]struct {
		mutate func(*Fingerprint)
		want   string
	}{
		"reference":    {func(f *Fingerprint) { f.Reference = "other.wav" }, "reference"},
		"mallet":       {func(f *Fingerprint) { f.MalletGrams = 5 }, "mallet mass"},
		"duration":     {func(f *Fingerprint) { f.DurationSeconds = 0.8 }, "duration"},
		"seed":         {func(f *Fingerprint) { f.Seed = 2 }, "seed"},
		"fixed value":  {func(f *Fingerprint) { f.Fixed = map[string]float64{"DAMP": 0.6} }, "fixed parameters"},
		"fixed absent": {func(f *Fingerprint) { f.Fixed = nil }, "fixed parameters"},
		"model":        {func(f *Fingerprint) { f.BaselineCost = 33.456 }, "baseline cost"},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			changed := base
			testCase.mutate(&changed)

			field := base.disagreement(changed)
			if !strings.HasPrefix(field, testCase.want) {
				t.Errorf("disagreement() = %q, want it to start with %q", field, testCase.want)
			}
		})
	}
}

// TestStoreKeepsOnlyTheBestPoint pins the snapshot's contract, including that it
// copies the position: mayfly reuses the slice it passes the objective, so
// keeping a reference would leave the checkpoint holding whatever the swarm
// last tried.
func TestStoreKeepsOnlyTheBestPoint(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fit.checkpoint")

	subject, err := loadStore(path, Fingerprint{})
	if err != nil {
		t.Fatal(err)
	}

	scratch := []float64{0.25, 0.75}

	if !subject.recordBest(5, scratch, 1) {
		t.Error("the first point was not recorded")
	}

	if subject.recordBest(6, []float64{0, 0}, 2) {
		t.Error("a worse point replaced the best")
	}

	scratch[0] = 0.9

	best := subject.best()
	if best == nil || best.Cost != 5 || best.Position[0] != 0.25 {
		t.Fatalf("best = %+v, want cost 5 at 0.25 — the position was not copied", best)
	}

	if !subject.recordBest(4, []float64{0.1, 0.2}, 3) {
		t.Error("a better point was not recorded")
	}

	if err := subject.flush(); err != nil {
		t.Fatal(err)
	}

	reopened, err := loadStore(path, Fingerprint{})
	if err != nil {
		t.Fatal(err)
	}

	if reopened.best() == nil || reopened.best().Cost != 4 {
		t.Errorf("after reopening, best = %+v, want cost 4", reopened.best())
	}
}

// TestNoCheckpointIsNotAnError keeps the nil store honest: every method has to
// work on it, since that is the default and the path most runs take.
func TestNoCheckpointIsNotAnError(t *testing.T) {
	t.Parallel()

	subject, err := loadStore("", Fingerprint{})
	if err != nil || subject != nil {
		t.Fatalf("loadStore(\"\") = %v, %v; want nil, nil", subject, err)
	}

	if got := subject.completed(); len(got) != 0 {
		t.Errorf("completed() = %v", got)
	}

	if got := subject.best(); got != nil {
		t.Errorf("best() = %v", got)
	}

	if subject.recordBest(1, []float64{0}, 1) {
		t.Error("recordBest reported an improvement with nowhere to store it")
	}

	if err := subject.recordRestart(RestartRecord{}); err != nil {
		t.Errorf("recordRestart() = %v", err)
	}

	if err := subject.flush(); err != nil {
		t.Errorf("flush() = %v", err)
	}
}
