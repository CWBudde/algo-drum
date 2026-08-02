package match

// The glide probe's memo tables, split out of features.go — which is the file
// that decides what to measure, where this one is only about not measuring the
// same samples forty times over.

import "math"

// glideTerms memoizes the two per-sample quantities a probe window averages:
// the wrapped phase step and the magnitude. Both are properties of the sample
// alone and not of the window, so measureGlide's late-probe walk re-derives
// them for every probe it takes, over windows that overlap by a factor of
// forty. On a candidate render that is 304 probes of 1,764 samples — 536 k
// math.Atan2 and as many math.Sqrt per extraction. Memoized it is 1,920 samples
// for the first probe plus 48 for each step of the walk: 17 k of each.
//
// measureGlide carries the counts and what they cost, including why a recording
// never walks at all and is therefore not slowed down by this table.
//
// The cache holds one contiguous range because the walk is contiguous: it
// starts at the latest probe and steps down by a millisecond, so each probe
// asks for 48 samples below what is already filled. A request that does not
// touch the filled range — the early probe, which sits far below the late
// window — refills from scratch rather than bridging the gap, because bridging
// it would compute the 16,000 samples in between that nobody reads.
//
// It is bit-exact, which is what lets it land without regenerating a fixture:
// each window is still summed in index order over the same values, and only the
// arctangent and the square root stop being recomputed. Extract's glide on
// three renders is identical to the last bit against the direct form.
type glideTerms struct {
	phaseStep []float64
	magnitude []float64
	// filled is the half-open sample range currently valid, in the index space
	// of inPhase. Empty when low >= high.
	low, high int
}

// reset drops the filled range without dropping the buffers, which the scratch
// pool keeps sized from the last hit.
func (terms *glideTerms) reset(work *extractScratch, length int) {
	terms.phaseStep = growFloats(work.glidePhaseStep, length)
	terms.magnitude = growFloats(work.glideMagnitude, length)
	work.glidePhaseStep, work.glideMagnitude = terms.phaseStep, terms.magnitude
	terms.low, terms.high = 0, 0
}

// ensure makes [start, end) valid, computing only what is missing.
func (terms *glideTerms) ensure(inPhase, quadrature []float64, start, end int) {
	if terms.low >= terms.high || start > terms.high || end < terms.low {
		terms.low, terms.high = start, start
	}

	if start < terms.low {
		terms.compute(inPhase, quadrature, start, terms.low)
		terms.low = start
	}

	if end > terms.high {
		terms.compute(inPhase, quadrature, terms.high, end)
		terms.high = end
	}
}

// compute fills [start, end) of both tables.
//
// The per-sample phase step is read off z[n] * conj(z[n-1]) rather than by
// differencing two absolute phases.
//
//	z[n] conj(z[n-1]) = (i[n]i[n-1] + q[n]q[n-1]) + j(q[n]i[n-1] - i[n]q[n-1])
//
// so its argument *is* phi[n] - phi[n-1], already in (-pi, pi] — which is what
// the explicit +3pi / Mod / -pi dance was reconstructing by hand.
//
// Three things follow. One atan2 a sample instead of two, and the two were the
// same call: previous at n is current at n-1, recomputed. No math.Mod, which
// was costing more than the arctangent it was correcting. And better
// conditioning for the small steps this actually measures — a glide is a
// fraction of a radian a sample, and taking it as the difference of two angles
// near +/-pi cancels most of the significand, whereas the cross and dot
// products carry it directly.
//
// The four streams the body reads are taken as subslices of one length —
// current and one-sample-delayed, in phase and quadrature — which is what lets
// the compiler drop the bounds checks.
func (terms *glideTerms) compute(inPhase, quadrature []float64, start, end int) {
	if start >= end {
		return
	}

	currentInPhase, currentQuadrature := inPhase[start:end], quadrature[start:end]
	delayedInPhase, delayedQuadrature := inPhase[start-1:end-1], quadrature[start-1:end-1]
	phaseStep, magnitude := terms.phaseStep[start:end], terms.magnitude[start:end]

	for index, nowInPhase := range currentInPhase {
		nowQuadrature := currentQuadrature[index]
		wasInPhase, wasQuadrature := delayedInPhase[index], delayedQuadrature[index]

		cross := nowQuadrature*wasInPhase - nowInPhase*wasQuadrature
		dot := nowInPhase*wasInPhase + nowQuadrature*wasQuadrature

		phaseStep[index] = math.Atan2(cross, dot)
		// math.Hypot rather than this was guarding against an overflow that
		// cannot happen: these are peak-normalized samples through a lowpass, so
		// neither square can leave the range of a float64. Hypot costs a branch
		// tree and a division per call, and this loop is the hottest caller in
		// the package.
		magnitude[index] = math.Sqrt(nowInPhase*nowInPhase + nowQuadrature*nowQuadrature)
	}
}
