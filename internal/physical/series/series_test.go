package series

import (
	"errors"
	"math"
	"testing"
)

func TestRanksAveragesTies(t *testing.T) {
	t.Parallel()

	got := Ranks([]float64{10, 20, 20, 30})
	want := []float64{0, 1.5, 1.5, 3}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rank %d = %v, want %v (got %v)", i, got[i], want[i], got)
		}
	}
}

// Reordering the inputs must permute the ranks and nothing more. This is the
// property averaging ties buys, and the reason it is not an implementation
// detail: a statistic that changes when the file list is reordered would make
// the take order evidence, which it is not.
func TestRanksAreOrderIndependent(t *testing.T) {
	t.Parallel()

	forward := Ranks([]float64{5, 5, 1})
	reversed := Ranks([]float64{1, 5, 5})

	if forward[0] != reversed[1] || forward[1] != reversed[2] || forward[2] != reversed[0] {
		t.Fatalf("ranks depend on input order: %v against %v", forward, reversed)
	}
}

func TestSpearmanPerfectMonotone(t *testing.T) {
	t.Parallel()

	// Deliberately curved: a monotone ramp that is not linear must still score
	// exactly +1, which is why these comparisons use rank correlation.
	values := []float64{1, 4, 9, 16, 25}

	got, err := Spearman(values, Indices(len(values)))
	if err != nil {
		t.Fatalf("Spearman: %v", err)
	}

	if math.Abs(got-1) > 1e-12 {
		t.Fatalf("Spearman = %v, want +1", got)
	}
}

func TestSpearmanPerfectInverse(t *testing.T) {
	t.Parallel()

	got, err := Spearman([]float64{5, 4, 3, 2, 1}, Indices(5))
	if err != nil {
		t.Fatalf("Spearman: %v", err)
	}

	if math.Abs(got+1) > 1e-12 {
		t.Fatalf("Spearman = %v, want -1", got)
	}
}

func TestPearsonKnownValue(t *testing.T) {
	t.Parallel()

	// y = 2x + 1 exactly, so the linear correlation is +1.
	got, err := Pearson([]float64{1, 2, 3, 4}, []float64{3, 5, 7, 9})
	if err != nil {
		t.Fatalf("Pearson: %v", err)
	}

	if math.Abs(got-1) > 1e-12 {
		t.Fatalf("Pearson = %v, want +1", got)
	}
}

// Spearman and Pearson part company on curved monotone data; if they ever
// agreed there, Spearman would not be ranking.
func TestSpearmanIsNotPearson(t *testing.T) {
	t.Parallel()

	values := []float64{1, 2, 4, 8, 16, 32}

	spearman, err := Spearman(values, Indices(len(values)))
	if err != nil {
		t.Fatalf("Spearman: %v", err)
	}

	pearson, err := Pearson(values, Indices(len(values)))
	if err != nil {
		t.Fatalf("Pearson: %v", err)
	}

	if math.Abs(spearman-pearson) < 0.05 {
		t.Fatalf("Spearman %v and Pearson %v agree on curved data", spearman, pearson)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()

	for name, run := range map[string]func() (float64, error){
		"length mismatch": func() (float64, error) {
			return Spearman([]float64{1, 2, 3}, []float64{1, 2})
		},
		"too few pairs": func() (float64, error) {
			return Spearman([]float64{1, 2}, []float64{2, 1})
		},
		"constant series": func() (float64, error) {
			return Pearson([]float64{1, 1, 1}, []float64{1, 2, 3})
		},
	} {
		if _, err := run(); !errors.Is(err, ErrSeries) {
			t.Fatalf("%s: err = %v, want ErrSeries", name, err)
		}
	}
}

// The session's own finding, pinned as a regression: two independent fits of
// the same sixteen takes returned velocity vectors that barely agree, which is
// what disqualified the fitted velocities as a measurement of anything. If this
// ever computes a high correlation from these numbers, the statistic is wrong
// rather than the conclusion.
func TestFittedVelocitiesDisagreeAcrossRuns(t *testing.T) {
	t.Parallel()

	deep := []float64{
		0.6431, 0.5419, 0.6142, 0.7012, 0.4586, 0.3069, 0.6128, 0.6940,
		0.5500, 0.9086, 0.1396, 0.5099, 0.9275, 0.1872, 0.3906, 0.8354,
	}
	first := []float64{
		0.4261, 0.3602, 0.8202, 0.4551, 0.5691, 0.4881, 0.8574, 0.7665,
		0.5468, 0.3339, 0.5194, 0.8249, 0.5107, 0.0751, 0.0293, 0.7719,
	}

	got, err := Spearman(deep, first)
	if err != nil {
		t.Fatalf("Spearman: %v", err)
	}

	if math.Abs(got-0.15) > 0.01 {
		t.Fatalf("Spearman = %.3f, want +0.15", got)
	}
}
