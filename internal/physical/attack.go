package physical

import (
	"github.com/cwbudde/algo-dsp/dsp/filter/biquad"
	"github.com/cwbudde/algo-dsp/dsp/filter/design"
)

// attackNoiseSeed fixes the noise sequence. Every render from a fresh or reset
// model is therefore bit-identical, which the suite depends on, and repeated hits
// within one render are identical to each other — deliberately. Per-trigger
// variation is a separate mechanism (PLAN.md S7) with a separate justification.
const attackNoiseSeed = 0x9E3779B97F4A7C15

// attackLayer is the non-modal half of the voice.
//
// Modal synthesis cannot reach a drum's real bandwidth in a browser: a membrane's
// mode count grows as f², so this head needs about 130 oscillators for 1 kHz,
// 530 for 2 kHz and 3300 for 5 kHz, against a budget of 96. What lives up there
// is not individually resolvable anyway — it is a dense, fast-decaying band that
// reads as the stick, not as pitch — so it is cheaper and more faithful to model
// it as filtered noise driven by the same contact force that drives the modes.
// That is what the published tom analysis does: Kirby and Sandler (DAFx-20)
// found 5 to 10 modes sufficient for the *sustain* of a central strike precisely
// because the attack is a separate object.
//
// Driving it from the contact force rather than triggering it alongside the
// strike means hardness and velocity carry into it for free: a harder stick has a
// shorter contact and so a brighter, tighter burst, with no second set of
// parameters to keep consistent.
type attackLayer struct {
	enabled     bool
	level       float64
	decayFactor float64
	band        biquad.Section
	envelope    float64
	noiseState  uint64
}

func newAttackLayer(attack Attack, sampleRateHz float64) attackLayer {
	if !attack.Enabled || attack.LevelRelative == 0 {
		return attackLayer{}
	}

	return attackLayer{
		enabled: true,
		level:   attack.LevelRelative,
		// One-pole release. The envelope is charged by the contact force and then
		// decays on its own, so the burst outlasts the 5.5-8 ms contact the way a
		// struck head's high band does.
		decayFactor: decayFactorPerSample(attack.DecaySeconds, sampleRateHz),
		band: biquad.Section{Coefficients: design.Bandpass(
			min(attack.CentreHz, sampleRateHz*0.45),
			attack.QualityFactor,
			sampleRateHz,
		)},
		noiseState: attackNoiseSeed,
	}
}

func decayFactorPerSample(decaySeconds, sampleRateHz float64) float64 {
	if decaySeconds <= 0 {
		return 0
	}

	// exp(-1/(tau*fs)) without the call: the argument is tiny and this keeps the
	// factor strictly inside (0,1) for every validated decay.
	samples := decaySeconds * sampleRateHz

	return samples / (samples + 1)
}

// tick advances the layer by one sample and returns its contribution to the
// radiated sum, in the same volume-acceleration units as the modal terms.
func (a *attackLayer) tick(forceN float64) float64 {
	if !a.enabled {
		return 0
	}

	if forceN < 0 {
		forceN = -forceN
	}

	a.envelope = a.envelope*a.decayFactor + forceN
	if a.envelope == 0 {
		return 0
	}

	return a.band.ProcessSample(a.level * a.envelope * a.noise())
}

// noise is xorshift64*, chosen because it is a few instructions, has no state
// beyond one word, and is exactly reproducible across platforms — which
// math/rand's global source is not, and which the bit-exact render tests need.
func (a *attackLayer) noise() float64 {
	a.noiseState ^= a.noiseState >> 12
	a.noiseState ^= a.noiseState << 25
	a.noiseState ^= a.noiseState >> 27

	// Scaled to [-1, 1) from the top 53 bits, which are the well-mixed ones.
	return float64(int64(a.noiseState*0x2545F4914F6CDD1D>>11))/(1<<52) - 1
}

func (a *attackLayer) reset() {
	a.envelope = 0
	a.noiseState = attackNoiseSeed
	a.band.Reset()
}

// isRinging reports whether the layer still has audible envelope left. The modal
// energy threshold cannot speak for it: the voice stops calling Tick the moment
// IsActive is false, so a layer outlasting the modal tail would be cut off.
func (a *attackLayer) isRinging() bool {
	return a.enabled && a.envelope > attackInactiveEnvelope
}

// attackInactiveEnvelope is in newtons of accumulated contact force. The
// quietest usable hit charges the envelope to several newtons, so this is far
// below anything audible while still terminating in a bounded time.
const attackInactiveEnvelope = 1e-6
