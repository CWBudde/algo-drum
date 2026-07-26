//go:build !drumassert

package drum

// assertValid is a no-op in shipped builds: Render is the hot loop, and a
// panic inside the WASM worker would take the whole engine down mid-playback
// for a defect the audio path already degrades gracefully around. Build with
// `-tags drumassert` (see assert_debug.go) to make Render check itself.
func (e *Engine) assertValid() {}
