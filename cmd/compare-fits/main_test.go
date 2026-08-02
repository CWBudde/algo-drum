package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cwbudde/algo-drum/internal/physical/match"
	"github.com/cwbudde/algo-drum/internal/physical/series"
)

// The fixtures below are *constructed*, not measured. Only one full fit report
// exists on disk, and inventing a second one that looks like a real run would put
// numbers in the test that could later be quoted as evidence. So the second
// report here is the first with a stated perturbation applied, and the only
// assertions are about arithmetic and about what the tool refuses — never about
// what a drum does.
//
// The one real quantity that is pinned is the shape of the finding this tool was
// built for: two velocity orderings that disagree must come out disagreeing, and
// the verdict must say so in words.

func writeReport(t *testing.T, dir, name string, value any) string {
	t.Helper()

	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encoding %s: %v", name, err)
	}

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}

	return path
}

// synthetic builds a report whose per-take velocities are exactly the values
// given, so a test can state the correlation it expects.
func synthetic(velocities []float64, weights match.Weights, params []param) report {
	takes := make([]take, 0, len(velocities))
	references := make([]reference, 0, len(velocities))

	for i, velocity := range velocities {
		path := filepath.Join("reference", "tt08x08", "lp", "hd", takeName(i))
		takes = append(takes, take{Path: path, Velocity01: velocity})
		references = append(references, reference{Path: path})
	}

	return report{
		References: references,
		Weights:    weights,
		Search:     search{Variant: "ma", Iterations: 30, Population: 12, Restarts: 4, Seed: 1, Evaluations: 5002},
		Baseline:   &stage{Terms: match.Terms{Total: 39.882034409395786}},
		Best: &stage{
			Terms:  match.Terms{PartialFrequency: 76.5, PartialLevel: 12.5, Total: 15.2},
			Params: params,
			Takes:  takes,
		},
	}
}

func takeName(index int) string {
	return "v" + string(rune('0'+(index+1)/10)) + string(rune('0'+(index+1)%10)) + ".wav"
}

func TestCompareReportsVelocityDisagreement(t *testing.T) {
	dir := t.TempDir()

	// Reversed order: the strongest possible statement of "these two runs did not
	// find the same strike ordering", and one whose Spearman is known exactly
	// without measuring anything.
	first := []float64{0.10, 0.20, 0.30, 0.40, 0.50, 0.60}
	second := []float64{0.60, 0.50, 0.40, 0.30, 0.20, 0.10}

	weights := match.DefaultWeights()

	a := writeReport(t, dir, "a.json", synthetic(first, weights, nil))
	b := writeReport(t, dir, "b.json", synthetic(second, weights, nil))

	var stdout, stderr bytes.Buffer
	if err := run([]string{a, b}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	if rho, err := series.Spearman(first, second); err != nil || rho != -1 {
		t.Fatalf("fixture is not a perfect disagreement: rho = %v, err = %v", rho, err)
	}

	out := stdout.String()

	if !strings.Contains(out, "rho = -1.000") {
		t.Errorf("expected the reversed velocities to report rho = -1.000:\n%s", out)
	}

	if !strings.Contains(out, "which is not agreement") {
		t.Errorf("expected the disagreement verdict:\n%s", out)
	}

	if !strings.Contains(out, "nuisance parameter") {
		t.Errorf("expected the verdict to say what a disagreement means:\n%s", out)
	}
}

func TestCompareReportsAgreementVerdict(t *testing.T) {
	dir := t.TempDir()

	same := []float64{0.10, 0.20, 0.30, 0.40, 0.50, 0.60}
	weights := match.DefaultWeights()

	a := writeReport(t, dir, "a.json", synthetic(same, weights, nil))
	b := writeReport(t, dir, "b.json", synthetic(same, weights, nil))

	var stdout, stderr bytes.Buffer
	if err := run([]string{a, b}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()

	if !strings.Contains(out, "rho = +1.000") {
		t.Errorf("expected identical velocities to report rho = +1.000:\n%s", out)
	}

	if !strings.Contains(out, "identifies the strike ordering") {
		t.Errorf("expected the agreement verdict:\n%s", out)
	}
}

// A term at its gate must print as exactly 1.0, because that identity — weight =
// 1/gate — is the whole reason the ratio column is readable.
func TestGateRatioIsOneAtTheGate(t *testing.T) {
	dir := t.TempDir()

	weights := match.DefaultWeights()
	gates := match.AdoptionGates()

	at := synthetic([]float64{0.1, 0.2, 0.3, 0.4}, weights, nil)
	at.Best.Terms = match.Terms{
		PartialFrequency: gates.PartialFrequency,
		Envelope:         gates.Envelope,
	}

	a := writeReport(t, dir, "a.json", at)
	b := writeReport(t, dir, "b.json", at)

	var stdout, stderr bytes.Buffer
	if err := run([]string{a, b}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "partial frequency") && !strings.Contains(line, "1.000") {
			t.Errorf("a term at its gate must contribute 1.000: %q", line)
		}
	}
}

func TestPinnedFreeParameterIsFlagged(t *testing.T) {
	dir := t.TempDir()

	weights := match.DefaultWeights()

	// The motivating case: damping free in both runs and against its low stop in
	// both. diameter is fixed at the same near-stop-free position and must not be
	// flagged — a pinned *fixed* parameter is a person's decision, not a defect
	// in the range.
	params := []param{
		{ID: "physicalTom.diameter", Label: "SIZE", Unit: "m", Normalized: 0.0050, Value: 0.2032, Fixed: true},
		{ID: "physicalTom.damping", Label: "DAMP", Unit: "", Normalized: 0.0084, Value: 0.0084},
		{ID: "physicalTom.batterTension", Label: "B.TUNE", Unit: "N/m", Normalized: 0.6377, Value: 1437.2},
	}

	a := writeReport(t, dir, "a.json", synthetic([]float64{0.1, 0.2, 0.3, 0.4}, weights, params))

	shifted := append([]param(nil), params...)
	shifted[1].Normalized = 0.0091

	b := writeReport(t, dir, "b.json", synthetic([]float64{0.1, 0.2, 0.3, 0.4}, weights, shifted))

	var stdout, stderr bytes.Buffer
	if err := run([]string{a, b}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, line := range strings.Split(stdout.String(), "\n") {
		switch {
		case strings.HasPrefix(line, "physicalTom.damping"):
			if !strings.Contains(line, "PINNED") {
				t.Errorf("a free parameter at a stop must be flagged: %q", line)
			}
		case strings.HasPrefix(line, "physicalTom.diameter"), strings.HasPrefix(line, "physicalTom.batterTension"):
			if strings.Contains(line, "PINNED") {
				t.Errorf("only free parameters away from their stops may be unflagged: %q", line)
			}
		}
	}
}

func TestRefusesDifferentWeights(t *testing.T) {
	dir := t.TempDir()

	velocities := []float64{0.1, 0.2, 0.3, 0.4}

	shipped := match.DefaultWeights()

	tightened := shipped
	tightened.PartialDecay *= 2

	a := writeReport(t, dir, "a.json", synthetic(velocities, shipped, nil))
	b := writeReport(t, dir, "b.json", synthetic(velocities, tightened, nil))

	var stdout, stderr bytes.Buffer

	err := run([]string{a, b}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected a refusal: totals under different weight sets are not comparable")
	}

	if !strings.Contains(err.Error(), "different weights") {
		t.Errorf("refusal should name the reason: %v", err)
	}

	// The escape hatch has to work, and has to be loud when it does.
	stdout.Reset()
	stderr.Reset()

	if err := run([]string{"-allow-incomparable", a, b}, &stdout, &stderr); err != nil {
		t.Fatalf("-allow-incomparable: %v", err)
	}

	if !strings.Contains(stderr.String(), "WARNING") {
		t.Errorf("an allowed incomparable run must warn: %q", stderr.String())
	}
}

func TestRefusesDifferentReferences(t *testing.T) {
	dir := t.TempDir()

	weights := match.DefaultWeights()

	a := writeReport(t, dir, "a.json", synthetic([]float64{0.1, 0.2, 0.3, 0.4}, weights, nil))

	other := synthetic([]float64{0.1, 0.2, 0.3, 0.4}, weights, nil)
	other.References[0].Path = "reference/tt12x08/lp/hd/v01.wav"

	b := writeReport(t, dir, "b.json", other)

	var stdout, stderr bytes.Buffer

	err := run([]string{a, b}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected a refusal: a fit report is only meaningful beside its own reference")
	}

	if !strings.Contains(err.Error(), "different references") {
		t.Errorf("refusal should name the reason: %v", err)
	}
}

// Reordering the reference list is not a difference. The take order is a claim
// about the session, so a tool that changed its answer when the list was
// reordered would be building that claim into itself.
func TestReferenceOrderIsNotEvidence(t *testing.T) {
	dir := t.TempDir()

	weights := match.DefaultWeights()

	a := writeReport(t, dir, "a.json", synthetic([]float64{0.1, 0.2, 0.3, 0.4}, weights, nil))

	reordered := synthetic([]float64{0.1, 0.2, 0.3, 0.4}, weights, nil)
	reordered.References[0], reordered.References[3] = reordered.References[3], reordered.References[0]

	b := writeReport(t, dir, "b.json", reordered)

	var stdout, stderr bytes.Buffer
	if err := run([]string{a, b}, &stdout, &stderr); err != nil {
		t.Fatalf("a reordered reference list is the same reference list: %v", err)
	}
}

// A report that grows fields must still decode. This is the property the lenient
// structs exist for, and it is easy to lose by accident.
func TestUnknownFieldsAreIgnored(t *testing.T) {
	dir := t.TempDir()

	weights := match.DefaultWeights()

	encoded, err := json.Marshal(synthetic([]float64{0.1, 0.2, 0.3, 0.4}, weights, nil))
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	generic["somethingAddedLater"] = map[string]any{"nested": []float64{1, 2, 3}}
	generic["best"].(map[string]any)["alsoNew"] = 7

	a := writeReport(t, dir, "a.json", generic)
	b := writeReport(t, dir, "b.json", generic)

	var stdout, stderr bytes.Buffer
	if err := run([]string{a, b}, &stdout, &stderr); err != nil {
		t.Fatalf("unknown report fields must not break the comparison: %v", err)
	}

	if !strings.Contains(stdout.String(), "rho = +1.000") {
		t.Errorf("expected the comparison to still run:\n%s", stdout.String())
	}
}

func TestRejectsUnfinishedReport(t *testing.T) {
	dir := t.TempDir()

	weights := match.DefaultWeights()

	unfinished := synthetic([]float64{0.1, 0.2, 0.3, 0.4}, weights, nil)
	unfinished.Best = nil

	a := writeReport(t, dir, "a.json", unfinished)
	b := writeReport(t, dir, "b.json", synthetic([]float64{0.1, 0.2, 0.3, 0.4}, weights, nil))

	var stdout, stderr bytes.Buffer
	if err := run([]string{a, b}, &stdout, &stderr); err == nil {
		t.Fatal("expected a report with no best point to be rejected")
	}
}
