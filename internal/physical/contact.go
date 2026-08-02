package physical

import "math"

// The stick contact endpoints.
//
// These are *contact times* — the interval over which stick and head touch —
// and they are well measured: Dahl 1997 for a 12-inch tom, corroborated by
// Wagner (KTH 2006) Fig. 4.7, whose centre-strike crescendo runs 7.5 ms at
// piano to 5.9 ms at forte.
//
// ContactPrescribed spends them as the width of a single smooth half-sine, and
// that is wrong, because a contact time is not a force-pulse width. Wagner
// measured both and found the stick "would already leave the drumhead after
// approximately 3.5 ms", staying in contact only because the wave it launched
// returns off the rim and touches it again, as two weaker impacts at 3.75 ms
// and 5.6 ms (§4.1.1, §4.2.1). His Fig. 4.7 plots that same forte stroke at
// 5.9 ms with the re-contacts counted and 4.6 ms without. So the real
// excitation is three short impacts inside a ~6 ms window, and the prescribed
// pulse replaces them with one smooth lobe spanning the whole of it — which is
// the measured cause of the missing 500-1000 Hz band in
// docs/physical-excitation-gap.md.
//
// Correcting the width alone does not help and was measured not to: it raises
// the 60-1000 Hz peak of the default drum by 14 dB, because a half-sine of
// width tau nulls at 1.5/tau and shortening the pulse drags that zero from
// 273 Hz to 429 Hz, straight through the low mode cluster. Since tau varies with
// both velocity and hardness it does that differently at every dynamic, so it is
// not a level error that a gain could absorb. Width and shape have to be fixed
// together, which is what ContactHertzian does — there the duration is not spent
// at all, it is predicted.
//
// The constants remain here because ContactHertzian is calibrated against them
// and because ContactPrescribed is still the default.
const (
	quietStickContactSeconds = 0.008
	loudStickContactSeconds  = 0.0055
	referenceStickHardness   = 0.7
)

// contactMalletSlots bounds how many strikes may be in contact at once.
//
// One stick cannot be in two places, but two hands can, and the model is also
// asked to superpose a flam onto a still-ringing hit. Four is past any real
// sticking and costs one branch per slot per sample.
const contactMalletSlots = 4

// contactSubstepTarget is the fraction of a contact-oscillation radian the
// contact integrator is held to.
//
// It is far below the stability limit, and deliberately: stability is not the
// binding constraint here, grazing is. Contact ends where the compression
// crosses zero, and there the spring is arbitrarily soft, so a step that is fine
// for the peak decides by rounding whether the stick has separated. Too coarse a
// step turns a single smooth touch into a chattering sequence of them — which is
// entirely convincing, because a separating and re-contacting stick is a real
// measured phenomenon, and entirely false. At 0.015 the default drum's contact
// spectrum is within 0.2 dB of the substep-converged value up to 1 kHz and
// within 1.4 dB at 1.5 kHz.
const contactSubstepTarget = 0.015

// contactSeparationSeconds is how long the force must stay below
// contactSeparationFraction of the peak before a touch counts as having ended,
// and contactSeparationFraction is that level.
//
// Separation is measured at a level rather than at exact zero because the end of
// a contact is a grazing event: the two surfaces part slowly, the spring is
// arbitrarily soft there, and whether the compression dips negative for a
// fraction of a millisecond is decided by the step size rather than by the drum.
// Judged at zero, the default contact reports two touches at 17 substeps and one
// at 64; judged at a hundredth of its own peak, it reports one at both, because
// the force in the disputed interval is around 0.01 N against a 17 N peak.
//
// Wagner's re-contacts are substantial impacts 1.85 ms apart, so neither
// threshold is near hiding the phenomenon this metric exists to detect.
const (
	contactSeparationSeconds  = 1e-4
	contactSeparationFraction = 0.01
)

// contactMaxSubsteps caps the cost of a very hard tip. Contact lasts a few
// hundred samples out of a note, so even the cap is a rounding error against the
// modal bank, but it has to be finite for the no-allocation, bounded-work
// guarantee to mean anything.
const contactMaxSubsteps = 64

// ContactMetrics reports what a strike's contact actually did, which under
// ContactHertzian is an output of the model rather than an input to it.
//
// FirstLobeSeconds is the first uninterrupted touch and DwellSeconds spans the
// first touch to the last release, so the two differ exactly when the head
// returns and catches the stick again. Those are the two numbers Wagner's
// Fig. 4.7 plots separately, and comparing them is how this model is checked.
type ContactMetrics struct {
	FirstLobeSeconds float64
	DwellSeconds     float64
	TouchCount       int
	PeakForceN       float64
	ImpulseNS        float64
}

// mallet is one striking tip: a free mass, its contact spring, and the
// bookkeeping that turns its touches into ContactMetrics.
type mallet struct {
	active        bool
	touching      bool
	positionM     float64
	velocityMPerS float64
	stiffness     float64
	remaining     int
	elapsed       int

	firstLobeEnd int
	lastTouch    int
	touchCount   int
	peakForceN   float64
	impulseNS    float64
}

// contact produces the force the batter head is driven by, under either model.
//
// It owns both paths because they are alternatives for one quantity and because
// SingleHead and DoubleHead must drive their modes from bit-identical force
// sequences — the zero-coupling equivalence test compares the two models sample
// for sample.
type contact struct {
	model        ContactModel
	sampleRateHz float64
	massKg       float64
	maxVelocity  float64
	hardness01   float64
	exponent     float64
	hysteresis   float64
	windowLength int
	substeps     int
	// A touch must be released for this long before the next one counts as a
	// separate impact; see contactSeparationSeconds.
	separationSamples int

	// The head's driving-point compliance, as seen inside one sample. Stored
	// inverted because the substep loop only ever divides by it.
	inverseHeadMassKg float64

	// The reference stiffness is stored already resolved for the configured
	// hardness, because hardness is a property of the stick rather than of the
	// stroke and cannot change between triggers without a Reconfigure.
	stiffness float64

	mallets   [contactMalletSlots]mallet
	nextSlot  int
	liveCount int

	pending        []float64
	pendingIndex   int
	pendingSamples int

	contactSamples int
	lastMetrics    ContactMetrics
}

// newContact resolves the configured contact model into the per-sample form.
// Both paths size their storage for the longest contact the configuration
// admits, so nothing allocates after this point.
func newContact(config PhysicalDrum) contact {
	strike := config.Strike
	built := contact{
		model: strike.Contact.Model,
		separationSamples: max(1, int(math.Round(
			contactSeparationSeconds*config.SampleRateHz,
		))),
		sampleRateHz: config.SampleRateHz,
		massKg:       strike.MalletMassKg,
		maxVelocity:  strike.VelocityMPerS,
		hardness01:   strike.Hardness01,
		exponent:     strike.Contact.Exponent,
		hysteresis:   strike.Contact.HysteresisSPerM,
	}

	if built.model != ContactHertzian {
		built.model = ContactPrescribed
		built.pending = make(
			[]float64,
			contactSampleCount(config.SampleRateHz, strike.Hardness01, 0),
		)
		built.windowLength = len(built.pending)

		return built
	}

	built.stiffness = hertzStiffnessNPerMAlpha(strike)
	built.windowLength = max(
		1,
		int(math.Round(strike.Contact.MaxDurationSeconds*config.SampleRateHz)),
	)

	return built
}

// setSubsteps installs the contact integrator's substep count and the head's
// driving-point inertia. Both depend on the modes, so they cannot be resolved in
// newContact; until this has run, triggerHertzian declines to arm a mallet
// rather than integrate against an unset inertia.
func (c *contact) setSubsteps(headPointMassKg float64) {
	if c.model != ContactHertzian {
		c.substeps = 1

		return
	}

	if headPointMassKg > 0 && !math.IsInf(headPointMassKg, 0) {
		c.inverseHeadMassKg = 1 / headPointMassKg
	}

	c.substeps = hertzSubsteps(
		c.stiffness,
		c.exponent,
		reducedMassKg(c.massKg, headPointMassKg),
		c.maxVelocity,
		c.sampleRateHz,
	)
}

// pulseSamples reports the window over which the contact may still act. Under
// ContactPrescribed that is the force pulse itself; under ContactHertzian the
// force is not known in advance, so it is the tracking window inside which the
// stick is allowed to touch again. Callers use it as a loop bound, which both
// readings satisfy.
func (c *contact) pulseSamples() int { return c.contactSamples }

// metrics reports the most recently completed contact.
func (c *contact) metrics() ContactMetrics { return c.lastMetrics }

func (c *contact) isActive() bool {
	return c.pendingSamples > 0 || c.liveCount > 0
}

func (c *contact) reset() {
	clear(c.pending)
	c.pendingIndex = 0
	c.pendingSamples = 0
	c.contactSamples = 0
	c.mallets = [contactMalletSlots]mallet{}
	c.nextSlot = 0
	c.liveCount = 0
	c.lastMetrics = ContactMetrics{}
}

// trigger starts a strike. strikePointM is the head's current displacement
// under the stick, so a hit landing on a still-moving head starts from where
// the head actually is rather than from zero.
func (c *contact) trigger(velocity01, strikePointM float64) {
	if c.model == ContactPrescribed {
		c.triggerPrescribed(velocity01)

		return
	}

	c.triggerHertzian(velocity01, strikePointM)
}

func (c *contact) triggerPrescribed(velocity01 float64) {
	impulseKgMPerS := c.massKg * c.maxVelocity * velocity01
	sampleCount := contactSampleCount(
		c.sampleRateHz,
		c.hardness01,
		velocity01,
	)

	addContactPulse(
		c.pending,
		c.pendingIndex,
		sampleCount,
		impulseKgMPerS*c.sampleRateHz,
	)
	c.pendingSamples = max(c.pendingSamples, sampleCount)
	c.contactSamples = sampleCount
}

func (c *contact) triggerHertzian(velocity01, strikePointM float64) {
	velocityMPerS := c.maxVelocity * velocity01
	if velocityMPerS <= 0 || c.substeps == 0 {
		return
	}

	slot := &c.mallets[c.nextSlot]
	if slot.active {
		// Every slot is busy, so the oldest one is displaced. That is a stick
		// being taken off the head early rather than a dropped hit: its modal
		// energy stays, and the incoming strike is the one that matters.
		c.liveCount--
	}

	c.nextSlot = (c.nextSlot + 1) % contactMalletSlots
	c.liveCount++

	*slot = mallet{
		active:        true,
		positionM:     strikePointM,
		velocityMPerS: velocityMPerS,
		stiffness:     c.stiffness,
		remaining:     c.windowLength,
		// Before any contact, so the first touch is not read as a re-contact.
		lastTouch:    -1 - c.separationSamples,
		firstLobeEnd: -1,
	}
	c.contactSamples = c.windowLength
}

// nextForce advances the contact by one sample and returns the force on the
// head, given the head's displacement and velocity under the stick at the start
// of that sample.
func (c *contact) nextForce(strikePointM, strikeVelocityMPerS float64) float64 {
	if c.model == ContactPrescribed {
		return c.nextPrescribedForce()
	}

	return c.nextHertzianForce(strikePointM, strikeVelocityMPerS)
}

func (c *contact) nextPrescribedForce() float64 {
	if c.pendingSamples == 0 {
		return 0
	}

	forceN := c.pending[c.pendingIndex]
	c.pending[c.pendingIndex] = 0

	c.pendingIndex++
	if c.pendingIndex == len(c.pending) {
		c.pendingIndex = 0
	}

	c.pendingSamples--

	return forceN
}

// nextHertzianForce integrates every live mallet across the sample and returns
// the mean force they applied over it.
//
// Both sides of the contact move inside the sample. Carrying the head along —
// as a free point mass of headPointMassKg driven by the same force, seeded each
// sample from the true modal state — is not a refinement but a requirement: with
// the head held still or merely extrapolated at constant velocity, the fast
// half of the contact oscillator is missing and the force chatters. That chatter
// looks exactly like the stick separating and being caught again, which is a
// real measured phenomenon, so it is worth saying plainly that here it was an
// artifact: it converged away under substep and sample-rate refinement, and the
// converged contact is a single smooth touch. See docs/physical-contact.md.
//
// The head track is discarded at the end of the sample; the modal bank is
// advanced by the mean force, which is what carries the momentum.
func (c *contact) nextHertzianForce(strikePointM, strikeVelocityMPerS float64) float64 {
	if c.liveCount == 0 {
		return 0
	}

	timeStep := 1 / c.sampleRateHz
	subStep := timeStep / float64(c.substeps)
	total := 0.0

	for index := range c.mallets {
		slot := &c.mallets[index]
		if !slot.active {
			continue
		}

		surfaceM := strikePointM
		surfaceVelocity := strikeVelocityMPerS
		sum := 0.0

		for range c.substeps {
			forceN := contactForceN(
				slot.positionM-surfaceM,
				slot.velocityMPerS-surfaceVelocity,
				slot.stiffness,
				c.exponent,
				c.hysteresis,
			)

			slot.velocityMPerS -= forceN / c.massKg * subStep
			slot.positionM += slot.velocityMPerS * subStep
			surfaceVelocity += forceN * c.inverseHeadMassKg * subStep
			surfaceM += surfaceVelocity * subStep
			sum += forceN
		}

		meanForceN := sum / float64(c.substeps)
		c.recordTouch(slot, meanForceN, timeStep)
		total += meanForceN
	}

	return total
}

// recordTouch advances one mallet's contact bookkeeping and retires it when its
// tracking window closes.
//
// The window is what ends a strike, not separation: the stick separating is
// exactly the event this model exists to represent, and a stick that has bounced
// off may still be caught by the head coming back up. Retiring on separation
// would delete the re-contacts and leave the prescribed model's defect in place
// under a different name.
func (c *contact) recordTouch(slot *mallet, forceN, timeStep float64) {
	slot.peakForceN = max(slot.peakForceN, forceN)
	if forceN > 0 {
		slot.impulseNS += forceN * timeStep
	}

	if forceN > contactSeparationFraction*slot.peakForceN {
		if !slot.touching && slot.elapsed-slot.lastTouch > c.separationSamples {
			slot.touchCount++
			if slot.touchCount > 1 && slot.firstLobeEnd < 0 {
				slot.firstLobeEnd = slot.lastTouch + 1
			}
		}

		slot.touching = true
		slot.lastTouch = slot.elapsed
	} else {
		slot.touching = false
	}

	slot.elapsed++
	slot.remaining--

	if slot.remaining > 0 {
		return
	}

	firstLobe := slot.firstLobeEnd
	if firstLobe < 0 {
		// One touch only, so the first lobe is the whole contact.
		firstLobe = slot.lastTouch + 1
	}

	c.lastMetrics = ContactMetrics{
		FirstLobeSeconds: float64(firstLobe) * timeStep,
		DwellSeconds:     float64(slot.lastTouch+1) * timeStep,
		TouchCount:       slot.touchCount,
		PeakForceN:       slot.peakForceN,
		ImpulseNS:        slot.impulseNS,
	}
	slot.active = false
	c.liveCount--
}

// contactForceN is the Hunt-Crossley form of Hertzian contact: an elastic
// K*delta^alpha with a hysteresis term proportional to it, so the damping
// vanishes with the compression instead of jumping at impact and again at
// separation the way a linear dashpot does.
//
// Tension is not transmitted, so a negative compression — or a hysteresis term
// large enough to pull the total below zero on the way out — means the stick has
// left the head.
func contactForceN(
	compressionM, approachMPerS, stiffness, exponent, hysteresis float64,
) float64 {
	if compressionM <= 0 {
		return 0
	}

	// x*sqrt(x) rather than Pow for the canonical 3/2, which is the default and
	// the measured value: the substep loop evaluates this tens of thousands of
	// times per strike and Pow is an order of magnitude dearer than Sqrt.
	//
	// Written as an either/or rather than as a Sqrt with a Pow override, which is
	// what it was: exponent is fixed for the model's lifetime, so the override
	// form paid for a discarded Sqrt on every substep of every non-default drum.
	var elastic float64
	if exponent == 1.5 {
		elastic = compressionM * math.Sqrt(compressionM)
	} else {
		elastic = math.Pow(compressionM, exponent)
	}

	forceN := stiffness * elastic * (1 + hysteresis*approachMPerS)

	return max(forceN, 0)
}

// hertzStiffnessNPerMAlpha resolves the tip stiffness for the configured
// hardness.
//
// The hardness law is chosen so that HARD keeps doing exactly what it did. A
// Hertzian contact's duration scales as K^(-1/(alpha+1)), so raising K by
// 2^(alpha+1) per unit of hardness reproduces the prescribed model's
// exp2(reference - hardness) duration law term for term — the knob's calibrated
// range survives the change of mechanism, and only the *shape* of the force
// inside that duration is different.
func hertzStiffnessNPerMAlpha(strike Strike) float64 {
	return strike.Contact.StiffnessNPerMAlpha *
		math.Exp2((strike.Hardness01-referenceStickHardness)*
			(strike.Contact.Exponent+1))
}

// reducedMassKg is the two-body mass of stick and head at the strike point.
func reducedMassKg(malletKg, headPointKg float64) float64 {
	if headPointKg <= 0 || math.IsInf(headPointKg, 0) {
		return malletKg
	}

	return malletKg * headPointKg / (malletKg + headPointKg)
}

// hertzSubsteps sizes the mallet integrator against the stiffest moment of the
// hardest admissible strike.
//
// The contact oscillation is sqrt(alpha*K*delta^(alpha-1)/mu), which is largest
// at peak compression, and peak compression follows from the impact energy:
// mu*v^2/2 = K*delta^(alpha+1)/(alpha+1). Sizing there rather than at some
// average is deliberate — the substep count is fixed for the model's lifetime,
// so it has to hold at the worst case a caller can reach.
func hertzSubsteps(
	stiffness, exponent, reducedKg, velocityMPerS, sampleRateHz float64,
) int {
	if stiffness <= 0 || velocityMPerS <= 0 || reducedKg <= 0 {
		return 1
	}

	peakCompressionM := math.Pow(
		(exponent+1)*reducedKg*velocityMPerS*velocityMPerS/(2*stiffness),
		1/(exponent+1),
	)
	angularFrequency := math.Sqrt(
		exponent * stiffness * math.Pow(peakCompressionM, exponent-1) / reducedKg,
	)
	substeps := int(math.Ceil(
		angularFrequency / sampleRateHz / contactSubstepTarget,
	))

	return min(max(substeps, 1), contactMaxSubsteps)
}

// hertzContactSeconds is the closed-form contact time of a mass rebounding off
// a rigid Hertzian spring. It is not what the model produces — the head is
// neither rigid nor still — but it is what the model is calibrated against, and
// the ratio of the two is the honest statement of how much the head contributes
// to the measured contact time.
func hertzContactSeconds(
	stiffness, exponent, reducedKg, velocityMPerS float64,
) float64 {
	if stiffness <= 0 || velocityMPerS <= 0 || reducedKg <= 0 {
		return 0
	}

	peakCompressionM := math.Pow(
		(exponent+1)*reducedKg*velocityMPerS*velocityMPerS/(2*stiffness),
		1/(exponent+1),
	)

	return 2 * hertzQuarterIntegral(exponent) * peakCompressionM / velocityMPerS
}

// hertzQuarterIntegral is the integral of (1 - u^(alpha+1))^(-1/2) over the unit
// interval, in closed form via the beta function. It is the shape factor that
// turns a peak compression into a duration; at alpha = 3/2 it is 1.4717.
func hertzQuarterIntegral(exponent float64) float64 {
	power := exponent + 1

	return math.Gamma(1/power) * math.Sqrt(math.Pi) /
		(power * math.Gamma(1/power+0.5))
}

// contactSampleCount resolves the prescribed model's contact interval in
// samples, which addContactPulse then uses as the force pulse's width. Those are
// two different quantities; see the constants above for why that matters and why
// ContactHertzian rather than a different number here is the fix.
func contactSampleCount(sampleRate, hardness01, velocity01 float64) int {
	stickDuration := quietStickContactSeconds +
		(loudStickContactSeconds-quietStickContactSeconds)*velocity01
	hardnessScale := math.Exp2(referenceStickHardness - hardness01)

	return max(2, int(math.Round(stickDuration*hardnessScale*sampleRate)))
}

func addContactPulse(
	pending []float64,
	start, sampleCount int,
	scale float64,
) {
	// The exact sum of sin(pi*(k+1/2)/N), k=0..N-1, is
	// 1/sin(pi/(2N)). This keeps the prescribed impulse invariant while
	// allowing velocity-dependent contact duration without allocation.
	normalizer := math.Sin(math.Pi / (2 * float64(sampleCount)))

	for index := range sampleCount {
		sample := math.Sin(
			math.Pi*(float64(index)+0.5)/float64(sampleCount),
		) * normalizer
		pendingIndex := (start + index) % len(pending)
		pending[pendingIndex] += scale * sample
	}
}
