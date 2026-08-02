package physical

import "math"

// ReleaseBoundSeconds is how long after a strike the model stops being an
// active voice, whatever its loss law says.
//
// It exists because the loss law is reachable. `PhysicalTomConfig` multiplies
// d1, d2 and every mode-decay correction by DAMP x D.TILT, so D.TILT at its
// lower stop deletes the whole frequency-dependent half of the law and leaves
// only the flat d0 and radiation terms: measured over the shipped knob ranges,
// D.TILT alone takes the release from 1.5 s to over 21 s, and the DAMP-min /
// D.TILT-0 / DEC-max corner to over 65 s. A note held for a minute after the
// player struck it is not a long decay, it is a stuck voice.
//
// The number is a product statement and has to be one. A deadline derived from
// the generated bank would scale *with* the pathology — at D.TILT 0 the bank's
// own slowest mode is the problem — so it would agree with whatever the knobs
// did. 8 s is 5.2x the shipped voice's own 1.527 s release, which is what makes
// the bound provably inert on everything that ships; `just check-physical-reference`
// staying clean is the standing proof of that, since the analysis renders
// 1.2-2 s windows.
//
// It is exported so the product-side sweep in internal/drum asserts against
// this constant rather than against a second copy of the number.
const ReleaseBoundSeconds = 8.0

// releaseFadeSeconds is the raised-cosine ramp run out at the end of the bound.
//
// It is not decoration. physicalTom.Tick hard-returns 0 the moment IsActive
// goes false, so a bound that fired on a still-ringing voice would step the
// output to zero in one sample and click. Five milliseconds is long enough to
// be inaudible as a discontinuity and short enough that the voice is gone
// within a sample or two of the deadline.
const releaseFadeSeconds = 0.005

// releaseBound counts samples since the last strike and turns the count into a
// radiated-output gain and an is-it-over predicate.
//
// It gates the radiated output and IsActive, and nothing else. Tick still runs
// the full state update past the deadline, so every conservation and passivity
// test reads the same stored energy it read before this existed, and a
// fixed-length Render is never truncated.
type releaseBound struct {
	boundSamples int
	fadeSamples  int
	sample       int
}

func newReleaseBound(sampleRateHz float64) releaseBound {
	return releaseBound{
		boundSamples: int(ReleaseBoundSeconds * sampleRateHz),
		// At least one sample, so a fade always exists to ramp through even at
		// an absurdly low sample rate.
		fadeSamples: max(1, int(releaseFadeSeconds*sampleRateHz)),
	}
}

// restart re-arms the deadline. A strike restarts it rather than extending it,
// which is what makes a roll behave: every hit buys the voice another
// ReleaseBoundSeconds, and the bound only bites once the player stops.
func (r *releaseBound) restart() { r.sample = 0 }

// advance returns the gain for this sample and moves the counter on. It must be
// called exactly once per Tick.
//
// The ramp sits *inside* the deadline rather than after it, so
// ReleaseBoundSeconds is the whole claim — the voice is gone by then, not gone
// shortly after then. A bound a fade-length longer than its own constant is the
// kind of small lie that survives for years.
func (r *releaseBound) advance() float64 {
	sample := r.sample
	r.sample++

	fadeFrom := r.boundSamples - r.fadeSamples
	if sample < fadeFrom {
		return 1
	}

	if sample >= r.boundSamples {
		return 0
	}

	// The +1 puts the exact zero on the last sample before the deadline rather
	// than on the first sample after it. That end is the one the click argument
	// is about — physicalTom.Tick stops calling this the moment IsActive goes
	// false — so it is the end that gets to be exact. The cost is that the ramp
	// starts a step below unity instead of at it, some 87 dB down on a 5 ms
	// fade at 48 kHz.
	return 0.5 * (1 + math.Cos(math.Pi*float64(sample-fadeFrom+1)/float64(r.fadeSamples)))
}

// expired reports whether the deadline has passed. It is deliberately false for
// the whole ramp: going inactive mid-fade would reintroduce the click the fade
// exists to remove.
func (r *releaseBound) expired() bool {
	return r.sample >= r.boundSamples
}
