package drum

import (
	"testing"
)

// Scratch measurement for PLAN N12 gap 1. Deleted before the change lands.
func TestScratchReleaseTime(t *testing.T) {
	const sampleRate = 48000

	release := func(setup func(v *physicalTom)) float64 {
		v, err := newPhysicalTom(sampleRate)
		if err != nil {
			t.Fatal(err)
		}

		setup(v)
		v.Trigger(1)

		const limit = 300 * sampleRate
		for index := range limit {
			v.Tick()

			if !v.IsActive() {
				return float64(index) / sampleRate
			}
		}

		return -1
	}

	specs := PhysicalTomSpecs()

	t.Logf("default: %.3f s", release(func(*physicalTom) {}))

	for index, spec := range specs {
		for _, position := range []float64{0, 1} {
			seconds := release(func(v *physicalTom) {
				v.SetParam(index, position)
			})
			t.Logf("%-8s at %.0f: %.3f s", spec.Label, position, seconds)
		}
	}

	// The corner the plan predicts: DAMP at its lower stop, DEC at maximum,
	// D.TILT at zero.
	seconds := release(func(v *physicalTom) {
		v.SetParam(physicalTomParamDamping, 0)
		v.SetParam(physicalTomParamDampingTilt, 0)
		v.SetDecay(1)
	})
	t.Logf("DAMP=0 D.TILT=0 DEC=1: %.3f s", seconds)
}
