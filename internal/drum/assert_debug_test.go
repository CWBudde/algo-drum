//go:build drumassert

package drum

import (
	"strings"
	"testing"
)

// The tagged build turns a broken invariant into a panic at the render that
// touches it, instead of the wrong-but-plausible audio it would otherwise
// produce. Run with: go test -tags drumassert ./...

func TestRenderPanicsOnBrokenInvariant(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetRunning(true)
	engine.stepDuration[0] = 0

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("Render accepted a broken engine under the drumassert tag")
		}

		if msg, ok := recovered.(string); !ok || !strings.Contains(msg, "step 0 duration is zero") {
			t.Fatalf("panic value %v does not name the broken invariant", recovered)
		}
	}()

	renderTotal(engine, 64)
}

func TestRenderDoesNotPanicOnSoundEngine(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetStepCount(0, 7)
	engine.SetSwing(maxSwing)
	engine.SetHumanize(1)
	engine.SetCell(0, 0, 0, 1)
	engine.SetRunning(true)

	renderTotal(engine, samplesForStep(engine, 0)*20)
}
