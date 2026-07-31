package physical

import (
	"math"

	"github.com/cwbudde/algo-dsp/dsp/filter/biquad"
	"github.com/cwbudde/algo-dsp/dsp/filter/design"
)

// attackNoiseSeed fixes the noise sequence. Every render from a fresh or reset
// model is therefore bit-identical, which the suite depends on, and repeated hits
// within one render are identical to each other — deliberately. Per-trigger
// variation is a separate mechanism (PLAN.md S7) with a separate justification.
const attackNoiseSeed = 0x9E3779B97F4A7C15

// attackBandRatios place the layer's bands relative to Attack.CentreHz. At the
// default 4 kHz they land on 1.6, 4.0 and 10 kHz, so the group starts just above
// the top retained mode and covers the range the layer stands in for.
// Attack.CentreHz keeps meaning what it did: where the layer's weight sits.
//
// Three bands, not one, because the band this layer replaces does not decay at
// one rate. On a membrane γ grows with k, so the top of the range dies several
// times faster than the bottom; a single release makes the whole span ring for as
// long as its slowest part, which is heard as a noise burst sitting on the drum
// rather than as the drum's own attack. Measured at the default: 94 ms T60 at
// 1.6 kHz, 37 ms at 4 kHz, 15 ms at 10 kHz, against the flat 138 ms it had.
var attackBandRatios = [3]float64{0.4, 1, 2.5}

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
	enabled bool
	level   float64
	bands   [len(attackBandRatios)]attackBand
	// One source for all three bands. They barely overlap, so sharing it costs
	// no audible correlation and keeps the reproducible state to one word.
	noiseState uint64
}

// attackBand is one band of the layer: a filter, a release, and the envelope
// between them. Each band models a slice of the unresolved mode thicket, so each
// one gets that slice's own decay rate.
type attackBand struct {
	filter      biquad.Section
	decayFactor float64
	envelope    float64
}

// newAttackLayer builds the layer against the head whose modes it continues.
//
// The decay rates are *derived*, not fitted: each band's release is the head's
// own structural loss law evaluated at that band's centre wavenumber. That is the
// same law the resolved modes use, so the layer is an extrapolation of the mode
// series rather than an independent effect bolted beside it, and DAMP, DEC and
// D.TILT reach it for free because they have already been applied to the head.
//
// This replaced a single fitted 20 ms release, which was wrong in both size and
// shape. A one-pole 20 ms release is a 138 ms T60, where the loss law puts the
// band it stands for at 75 ms at 2 kHz and 18 ms at 8 kHz — so the layer rang
// about twice too long at the bottom of its range and seven times too long at
// the top, and rang equally long across the whole span. Broadband noise held that
// far past the strike is heard as a separate hiss instead of fusing into the
// attack, which is exactly what it sounded like.
func newAttackLayer(attack Attack, head Head, sampleRateHz float64) attackLayer {
	if !attack.Enabled || attack.LevelRelative == 0 {
		return attackLayer{}
	}

	layer := attackLayer{
		enabled:    true,
		level:      attack.LevelRelative,
		noiseState: attackNoiseSeed,
	}

	speed := WaveSpeedMPerS(head)

	for index, ratio := range attackBandRatios {
		centreHz := min(attack.CentreHz*ratio, sampleRateHz*0.45)
		decayRate := ModalDecayRatePerSecond(head, 2*math.Pi*centreHz/speed)
		layer.bands[index] = attackBand{
			filter: biquad.Section{Coefficients: design.Bandpass(
				centreHz,
				attack.QualityFactor,
				sampleRateHz,
			)},
			decayFactor: decayFactorPerSample(
				attackReleaseSeconds(attack.DecayScale, decayRate),
				sampleRateHz,
			),
		}
	}

	return layer
}

// maxAttackReleaseSeconds bounds the release the loss law is allowed to hand
// back, so that every validated head yields a layer that decays.
//
// It has to exist because the release is *derived*. All three structural loss
// coefficients validate down to zero, so a head with no structural loss at all
// is a configuration Validate accepts; on it ModalDecayRatePerSecond returns
// exactly zero, DecayScale/0 is +Inf, and decayFactorPerSample computes
// Inf/(Inf+1) = NaN. Every sample of the render is then NaN, and because the
// voice's NaN reaches the FDN reverb and the limiter's lookahead in internal/drum
// before the hard clamp, it poisons the delay lines permanently.
//
// The bound lives here rather than in Validate on purpose, and the choice is not
// the one the coupling coefficient's ceiling made. A validator bound would have to
// be a bound on the *combination* γ(k) = Loss0 + Loss1·k + Loss2·k², since no
// single knob is individually required to be positive, and "the combination is
// non-zero" is not enough: Loss2 = 1e-12 validates, gives a per-sample factor of
// 0.9999999999994, and leaves the layer at 0.68 of its peak four seconds after
// the strike with isRinging still true — audio that is finite and useless, and a
// voice that never goes inactive. A bound large enough to prevent that is a
// threshold read off this file's arithmetic imposed on the head's physics, and it
// would reject loss laws that are perfectly well behaved for everything else: the
// modal bank adds radiation loss on top of γ and decays fine without it, and the
// offline fitter searches these coefficients down to their validated floor.
// Clamping the derived quantity keeps the constraint where the constraint is.
//
// One second is the backstop, not a taste control. The shipped loss law puts the
// slowest band at a 13.6 ms release, and a head would have to be some seventy
// times less lossy than that before this binds — by which point the layer has
// stopped being an attack. It also bounds the time to inactivity, which +Inf and
// exactly-1.0 do not.
const maxAttackReleaseSeconds = 1

// attackReleaseSeconds is one band's release: the head's loss law at that band's
// wavenumber, scaled, and held finite. Both arguments are validated finite and
// non-negative, so no NaN can reach the comparison.
func attackReleaseSeconds(decayScale, decayRatePerSecond float64) float64 {
	// A zero scale is "no release" and decayFactorPerSample already means that
	// by a zero factor; it must not be read as a lossless head.
	if decayScale <= 0 {
		return 0
	}

	if decayRatePerSecond <= 0 {
		return maxAttackReleaseSeconds
	}

	return min(decayScale/decayRatePerSecond, maxAttackReleaseSeconds)
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

	// One noise sample feeds every band, so the draw happens whether or not the
	// envelopes have anything left: the sequence must not depend on the force.
	noise := a.noise()
	sum := 0.0

	for index := range a.bands {
		band := &a.bands[index]
		band.envelope = band.envelope*band.decayFactor + forceN
		sum += band.filter.ProcessSample(a.level * band.envelope * noise)
	}

	return sum
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
	a.noiseState = attackNoiseSeed

	for index := range a.bands {
		a.bands[index].envelope = 0
		a.bands[index].filter.Reset()
	}
}

// isRinging reports whether the layer still has audible envelope left. The modal
// energy threshold cannot speak for it: the voice stops calling Tick the moment
// IsActive is false, so a layer outlasting the modal tail would be cut off.
//
// The slowest band decides, which after the change to derived rates is always the
// lowest one.
func (a *attackLayer) isRinging() bool {
	if !a.enabled {
		return false
	}

	for index := range a.bands {
		if a.bands[index].envelope > attackInactiveEnvelope {
			return true
		}
	}

	return false
}

// attackInactiveEnvelope is in newtons of accumulated contact force. The
// quietest usable hit charges the envelope to several newtons, so this is far
// below anything audible while still terminating in a bounded time.
const attackInactiveEnvelope = 1e-6
