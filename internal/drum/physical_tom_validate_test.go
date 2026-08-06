package drum

import (
	"fmt"
	"testing"

	"github.com/cwbudde/algo-tom/tomparams"
)

// TestTheProductCannotBuildAConfigItsOwnValidatorRejects states the invariant
// that connects the two halves of the physical Tom and that nothing stated
// before.
//
// algo-tom's physical package validates SI configurations and its tomparams
// package maps normalized knob positions onto them. Those are two independently
// chosen sets of numbers, and nothing has ever checked that the second lands
// inside the first. A knob whose mapped range reached past a validated bound
// would let the UI produce a configuration the model refuses — which surfaces
// as a reconfigure that logs and silently reverts, i.e. a knob that stops
// working somewhere along its travel with no indication of why.
//
// It is stated here rather than upstream because it is a claim about *the
// product*: the bank a fresh voice starts from, swept one knob at a time.
// tomparams' own TestConfigIsValid moves every knob together, which cannot see
// a single knob whose extreme is only inadmissible beside the defaults.
//
// This is the check that has to hold before either validated ceiling can be
// tightened (PLAN.md N12b): a tightening that crossed a knob's reach would do
// exactly that, and would do it quietly.
//
// The sweep is one knob at a time to both stops against both stops of the strip
// decay trim. It does not attempt the full corner space, which is 3^18: joint
// reachability is a different and much larger question, and the useful thing to
// state first is that no single knob on its own leaves the validated region.
func TestTheProductCannotBuildAConfigItsOwnValidatorRejects(t *testing.T) {
	t.Parallel()

	// The bank a fresh voice starts from, so each case moves exactly one knob.
	defaults := newParamBank(physicalTomSpecs).vals

	for index, spec := range physicalTomSpecs {
		for _, position := range []float64{0, 1} {
			for _, decay := range []float64{0, tomparams.NeutralDecayAmount, 1} {
				name := fmt.Sprintf("%s=%g/DEC=%g", spec.Label, position, decay)

				values := make([]float64, len(defaults))
				copy(values, defaults)
				values[index] = position

				config, err := tomparams.Config(values, decay, testSampleRate)
				if err != nil {
					t.Errorf("%s: tomparams.Config: %v", name, err)

					continue
				}

				if err := config.Validate(); err != nil {
					t.Errorf(
						"%s: the product built a configuration its own validator "+
							"rejects: %v",
						name, err,
					)
				}
			}
		}
	}
}
