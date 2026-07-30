package main

import (
	"fmt"
	"io"
	"math"
	"sync"
	"time"
)

// tracker reports search progress from inside the objective function.
//
// It lives here rather than in the optimizer because mayfly exposes no
// per-iteration hook: without this, a run prints its first line only when a
// whole restart has finished, which for a real fit is tens of minutes of
// silence with no way to tell a slow search from a wedged one. Counting
// evaluations is the one place every restart passes through.
//
// Every method is safe for concurrent use — one tracker is shared by all
// restarts, which is the point, since the interesting number is the best any
// of them has reached.
type tracker struct {
	writer io.Writer
	// interval is the number of evaluations between reports; 0 disables them.
	interval int
	// expected is the predicted total evaluation count, used only to turn the
	// count into a percentage. It is an estimate: variants add evaluations the
	// formula does not model, so the percentage is clamped and may stall near
	// the end rather than overshoot.
	expected int
	started  time.Time

	mu        sync.Mutex
	count     int
	nextPrint int
	best      float64
}

func newTracker(writer io.Writer, interval, expected int, started time.Time) *tracker {
	return &tracker{
		writer:    writer,
		interval:  interval,
		expected:  expected,
		started:   started,
		nextPrint: interval,
		best:      math.Inf(1),
	}
}

// expectedEvaluations predicts how many objective calls a whole search makes.
//
// Per restart mayfly evaluates both populations once at start-up, then each
// iteration re-evaluates both populations, every offspring and every mutant.
// NM follows the library's "0 means 5% of NPop" default.
func expectedEvaluations(restarts, iterations, population, offspring int) int {
	mutants := int(math.Round(0.05 * float64(population)))
	perIteration := 2*population + offspring + mutants

	return restarts * (2*population + iterations*perIteration)
}

// observe records one objective evaluation and prints a line every interval.
//
// The cost is passed in rather than read back from the optimizer so that the
// reported best is the best actually evaluated, including candidates a restart
// discards later.
func (t *tracker) observe(cost float64) {
	if t == nil || t.interval <= 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.count++

	if cost < t.best {
		t.best = cost
	}

	if t.count < t.nextPrint {
		return
	}

	t.nextPrint += t.interval

	elapsed := time.Since(t.started).Round(time.Second)
	line := fmt.Sprintf("  %d evaluations in %s, best so far %s",
		t.count, elapsed, formatCost(t.best))

	if t.expected > 0 {
		share := float64(t.count) / float64(t.expected)
		line += fmt.Sprintf(" (~%.0f%%", math.Min(share, 1)*100)

		// Only project a finish once there is enough of a rate to trust, and
		// never past the estimate — an ETA that walks backwards is worse than
		// none at all.
		if share > 0.02 && share < 1 {
			remaining := time.Duration(float64(time.Since(t.started)) * (1/share - 1))
			line += fmt.Sprintf(", ~%s left", remaining.Round(time.Second))
		}

		line += ")"
	}

	_, _ = fmt.Fprintln(t.writer, line)
}

// formatCost renders a cost for the progress line, keeping +Inf readable —
// invalid configurations are a normal and frequent outcome here, not an error.
func formatCost(cost float64) string {
	if math.IsInf(cost, 1) {
		return "none yet"
	}

	return fmt.Sprintf("%.3f", cost)
}
