//go:build !drumassert

package drum

import "testing"

// The shipped build carries no assertion: Render must stay a hot loop with no
// per-buffer validation, so a broken invariant degrades to bad audio rather
// than killing the WASM runtime mid-playback. Its counterpart under the
// `drumassert` tag is assert_debug_test.go.
func TestRenderHasNoAssertionInDefaultBuild(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetRunning(true)
	engine.stepLen[0] = 0

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("default build asserted in Render: %v", recovered)
		}
	}()

	renderTotal(engine, 64)
}
