//go:build drumassert

package drum

// assertValid panics on any broken engine invariant. Built only under the
// `drumassert` tag — dev builds and the CI test run — so corruption fails at
// the render that touches it rather than turning into plausible-sounding
// audio. See Validate for the invariant set.
func (e *Engine) assertValid() {
	if err := e.Validate(); err != nil {
		panic("drum: engine invariant violated: " + err.Error())
	}
}
