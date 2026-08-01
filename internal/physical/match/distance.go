package match

import (
	"math"
	"slices"
)

// Terms is the distance broken into the things a listener would name
// separately. Each is reported in its own unit, so a number here can be read
// against a tolerance rather than only against another run.
type Terms struct {
	// PartialFrequency is the trimmed RMS cents error over matched partials.
	// Cents, because the ear hears pitch as a ratio and a 3 Hz error means
	// something different at 118 Hz than at 1180 Hz.
	PartialFrequency float64 `json:"partialFrequencyCents"`
	// PartialLevel is the trimmed RMS dB error of the partials' relative levels
	// — the balance between the fundamental and the modes above it.
	PartialLevel float64 `json:"partialLevelDB"`
	// PartialDecay is the trimmed RMS of |ln(T60_ref / T60_candidate)|. A log
	// ratio, because ringing twice as long and half as long are the same size
	// of mistake.
	//
	// It used to be weighted by the product of the two fits' R², on the reasoning
	// that a partial whose envelope is not an exponential has a meaningless
	// slope. The reasoning is sound and R² does not implement it: measured
	// against subband ESPRIT over the sixteen velocities of the licensed
	// reference, median ring-time disagreement is 40 % at R² >= 0.95 and 55 %
	// below it, which is not a separation. Its replacement candidate,
	// Partial.DecayRangeDB, was measured on the same evidence and does not
	// separate them either — it is not even monotone in the disagreement. So the
	// weighting is gone rather than replaced: an unmeasured confidence is worse
	// than none, because it looks like a guard.
	//
	// What does the job the weighting was meant to do is the trimming, which
	// needs no per-partial confidence estimate to work.
	// docs/physical-objective-validation.md §5c and §5f.
	PartialDecay float64 `json:"partialDecayLogRatio"`
	// SpectralEnvelope is the mean per-window RMS dB error of the
	// fractional-octave shape — what the hit sounds like, band by band, as it
	// evolves.
	SpectralEnvelope float64 `json:"spectralEnvelopeDB"`
	// Envelope is the dB RMS error of the amplitude envelope: the shape of the
	// decay as a whole, independent of which partial carries it.
	Envelope float64 `json:"envelopeDB"`
	// Glide is the absolute cents error of the pitch bend.
	Glide float64 `json:"glideCents"`
	// AttackBalance is the dB error of the click-to-body ratio.
	AttackBalance float64 `json:"attackBalanceDB"`
	// Unmatched is the share of the reference's partial *audibility* that no
	// candidate partial accounts for, where a partial is worth how far it
	// stands above the level at which it would not have been detected at all.
	// It is both a diagnostic and the blend that makes the three terms above
	// pay for what is missing rather than averaging over whatever happened to
	// match.
	//
	// This was energy-weighted, as $10^(dB/10)$, and that was wrong in a way
	// that took a listening test to notice. Energy is dominated by whichever
	// partial is loudest: on this repository's tom reference the 212.78 Hz
	// partial carries 99.4 % of it, so a candidate that reproduced that one
	// partial and nothing else scored an unmatched share of 0.006 — and every
	// partial term, averaging over the single pair that matched, reported an
	// excellent number for a drum with one mode in it. Six of eight terms were
	// effectively scoring one partial.
	//
	// Counting instead would overcorrect: the same reference has a genuine but
	// isolated component 39 dB down that no two-headed drum will produce, and
	// missing it must not cost what missing the loudest partial costs.
	// Weighting by dB above the detection floor is the compromise that keeps
	// both properties — it is monotone in loudness, so the loud partials still
	// dominate, but it is compressed enough that six missing quiet ones cannot
	// be rounded away.
	Unmatched float64 `json:"unmatchedShare"`
	// Spurious is the mirror of Unmatched: the share of the candidate's partial
	// audibility that sits in modes the reference has nothing to put against.
	//
	// Unmatched alone is a one-sided measure. It charges for reference partials
	// the candidate fails to produce, but a candidate partial with no reference
	// counterpart is invisible to every partial term — matchPartials iterates
	// the reference — and reaches the total only through the spectral envelope.
	// Measured on the first fit run under the audibility weighting: the
	// candidate covered all seven reference partials, reported an unmatched
	// share of 0.000, and its second-loudest component was an invented 182 Hz
	// mode 15 dB down that cost it nothing. Making missing partials expensive
	// without making invented ones expensive just moves the degenerate optimum
	// from too few modes to too many.
	//
	// Counted only between the lowest and highest reference partial. Above and
	// below that the reference's own detection is unproven — a room recording's
	// noise floor hides modes a model legitimately has — so a partial out there
	// is charged by the spectral envelope, on evidence, and not by this.
	Spurious float64 `json:"spuriousShare"`
	// Total is the weighted sum.
	Total float64 `json:"total"`
}

// Weights converts each term to a common currency. The defaults are set so
// that one "just about audible" error in any term contributes roughly the same
// amount, which is what makes the sum meaningful rather than arbitrary.
type Weights struct {
	PartialFrequency float64 `json:"partialFrequency"`
	PartialLevel     float64 `json:"partialLevel"`
	PartialDecay     float64 `json:"partialDecay"`
	SpectralEnvelope float64 `json:"spectralEnvelope"`
	Envelope         float64 `json:"envelope"`
	Glide            float64 `json:"glide"`
	AttackBalance    float64 `json:"attackBalance"`
	Unmatched        float64 `json:"unmatched"`
	Spurious         float64 `json:"spurious"`

	// MatchToleranceCents bounds how far a candidate partial may sit from a
	// reference partial and still be called the same mode. Scaled by the
	// partial's index, because the high modes of a real drum scatter and
	// insisting they do not would make the low ones unfittable.
	MatchToleranceCents float64 `json:"matchToleranceCents"`
}

// DefaultWeights is the scoring this repository's tom fit uses.
//
// Every weight is the reciprocal of that term's adoption gate, so a candidate
// scoring exactly at its gate contributes exactly 1.0 and the nine terms are
// commensurable. That much was always the intent, and TestWeightsAreReciprocalGates
// makes it structural.
//
// The gates are **measured**, not chosen. Each is the 90th percentile of the
// objective's disagreement with itself, taken over the sixteen velocities of
// reference/tt08x08/lp/hd/v*.wav scored channel-against-channel in both
// directions — 32 scorings. That pair is coincident: peak inter-channel
// correlation at 0 samples of lag on thirteen of the sixteen and 1 sample on the
// other three, at 0.944-0.990. The two signals are two observations of one
// acoustic event, so any disagreement between them is the instrument's own noise
// floor, and a candidate at its gate is indistinguishable from a second
// microphone at the same point in space. cmd/measure-objective performs the
// measurement, through this Distance rather than through a copy of it.
//
// They have now been measured three times, and the third measurement is a
// different *drum*, not a different estimator. The reference set moved from the
// medium-pitch head strikes to the low-pitch ones on 2026-08-01, because that is
// the sound the fit is now aiming at; the floor is a property of the estimator
// **and of what it is pointed at**, so it had to be re-derived rather than
// carried over. All three columns are the p90 of the same 32 scorings, taken
// through this Distance:
//
//	term              mp-hd, defective  mp-hd, repaired  lp-hd (current)  gate now
//	partial frequency        113.0 ¢           76.2 ¢          65.0 ¢       70 ¢
//	partial level             17.85 dB          6.81 dB         6.76 dB      7 dB
//	partial decay              1.262            0.558           0.589        0.6
//	spectral envelope          3.65 dB          3.67 dB         3.24 dB      3.5 dB
//	envelope                   3.81 dB          3.84 dB         1.38 dB      1.5 dB
//	glide                    310.3 ¢          280.1 ¢           2.3 ¢       10 ¢
//	attack balance             1.12 dB          1.13 dB         0.81 dB      0.9 dB
//	unmatched share            0.880            0.250           0.280        0.3
//	spurious share             0.346            0.245           0.293        0.3  (see below)
//
// Every term is at least as reproducible on the low-pitch set as on the medium,
// and two are dramatically more so. **Glide is the headline**: 280.1 ¢ → 2.3 ¢,
// a factor of 120. That is not an estimator change — the estimator is untouched.
// The medium-pitch fundamental died before the late probe, so more than half of
// those pairs were placing two probes on a partial that was no longer there and
// measuring the noise between them; the low-pitch fundamental rings long enough
// that both probes land on signal. A term that was documented here as "still
// broken" turns out to have been broken *by the target*, and on this reference it
// is the single most reproducible thing the objective measures. The envelope term
// improves for the same underlying reason — 2.5 s of file against 1.25 s, so the
// tail being compared is real signal rather than floor.
//
// The consequence to keep in view: the glide gate is now 10 cents. A candidate
// whose pitch bend is 30 cents wrong used to contribute 0.10 and now contributes
// 3.0. Glide has gone from a term the objective could not see to one of the
// terms most able to dominate a total, and no fit has yet been run under it.
//
// Gates are rounded *up* from the measured p90, because a gate is what a
// candidate has to beat and rounding a floor down sets a threshold below the
// floor.
//
// The two middle columns are the estimator history, kept because it is the
// reason the first gate set was wrong and the reason this comment insists the
// floor be re-derived. Both were measured on mp-hd. Running that campaign with
// the trimming in the three partial terms disabled separated the two defects by
// measurement rather than dividing them by assertion:
//
//	term               2026-08-01  repaired only  repaired + trimmed
//	partial frequency     113.0 ¢       112.4 ¢          76.2 ¢
//	partial level          17.85 dB       7.24 dB         6.81 dB
//	partial decay           1.262          0.608           0.558
//	unmatched share         0.880          0.250           0.250
//	spurious share          0.346          0.245           0.245
//
// So the estimator repair is what fixed level, decay, unmatched and spurious —
// those four were measuring the collapsed takes and nothing else — and the
// trimming is what fixed frequency, which the repair did not touch at all. They
// are two different defects and neither substitutes for the other.
//
// Where that leaves the standing findings, after the change of reference:
//
//   - **The spectral envelope is still the term that was always right.** 3.24 dB
//     against a gate of 3.5, and it was 3.67 against 4 on the other drum: the one
//     term neither an estimator defect nor a change of target has moved much.
//     Every conclusion drawn from it stands.
//   - **AttackBalance is still the most reproducible term in the objective**,
//     0.231 dB at the median, and it is still the one that used to carry the
//     smallest weight, 1/6. It now carries 1/0.9.
//   - **"Glide is broken" is withdrawn — it was a statement about the target.**
//     See the headline above. On this reference its median is 0.803 cents, which
//     is the best-behaved term in the whole objective. What survives of the old
//     finding is narrower and still useful: the glide estimator fails silently
//     and completely when the fundamental does not outlive the late probe, so it
//     is a reliable term only on a reference whose fundamental rings, and it must
//     be re-checked rather than assumed on any new one.
//   - **"The partial terms were never gateable" stays withdrawn.** 65 cents and
//     6.8 dB are wide tolerances but they are thresholds a model can be held to.
//     The six rounds of intervention aimed at the old 25-cent and 0.25 gates were
//     still aimed at thresholds nothing could reach; that part stands.
//
// Measured consequence: at these weights the objective's disagreement with itself
// totals **5.92 at the median and 6.68 at p90** on this reference. Read that as
// the floor, not as a score: no fit total below 5.92 is distinguishable from the
// objective's own noise, whatever else it claims. The same distribution scored
// under the previous, mp-hd-derived weights totals 4.68/5.72, which is the
// like-for-like number and is not a regression — tightening a gate raises the
// weight, so identical raw disagreement scores higher.
//
// Which is the whole difficulty with reading totals: **no total recorded before
// this change is comparable to any total after it**, and not even the sign of the
// change is meaningful. Two things moved at once here — the weights and the drum
// — so this boundary is harder than the previous ones, and the pre-2026-08-01
// fits were already incomparable for their own reasons. The readable quantity is
// the per-term contribution, and there the claim the weights make is intact:
// every term's p90 lands at or under its own gate, so nothing contributes more
// than 1.0 at the floor. cmd/measure-objective writes the floor into its own
// report so that a total always arrives beside the floor it should be read
// against.
//
// Spurious used to be a deliberate departure from the reciprocal rule: on mp-hd
// its floor came out at 0.245 against Unmatched's 0.250, which would have made it
// very slightly the heavier of the two, and that direction had already been
// refuted by a fit run — it abandoned the drum and converged on two partials with
// a spurious share of 0.000, because the blend's pressure toward completeness is
// exactly what the spurious weight works against, and outweighing it makes
// emptiness the cheapest bank on offer. So both terms were pinned to Unmatched's
// gate. On lp-hd the order is the other way round (unmatched 0.280, spurious
// 0.293) and both round up to the same 0.3, so the departure is no longer doing
// anything and the equality is now what the measurement says rather than an
// override of it. The refuted direction is still refuted, and the inequality is
// still worth pinning in case a future measurement separates them:
// TestSpuriousDoesNotOutweighCompleteness pins it; it cannot pin the behaviour,
// for the reason given there.
//
// The raw distribution, the method and the commands are in
// docs/physical-objective-validation.md. Re-derive these whenever the estimators
// in features.go change **or the reference set does**. The first version of this
// comment asserted the floor was "a property of the estimator, not of the drum".
// The lp-hd measurement refutes that: same estimator, different drum, and the
// glide floor moved by a factor of 120. It is a property of the pair. This
// repository has twice quoted gates measured through an estimator it had since
// replaced; do not now start quoting gates measured on a drum it no longer aims
// at.
func DefaultWeights() Weights {
	return Weights{
		PartialFrequency:    1.0 / 70,
		PartialLevel:        1.0 / 7,
		PartialDecay:        1.0 / 0.6,
		SpectralEnvelope:    1.0 / 3.5,
		Envelope:            1.0 / 1.5,
		Glide:               1.0 / 10,
		AttackBalance:       1.0 / 0.9,
		Unmatched:           1.0 / 0.3,
		Spurious:            1.0 / 0.3,
		MatchToleranceCents: 120,
	}
}

// AdoptionGates is DefaultWeights stated the other way round: the value of each
// term at which a candidate stops being distinguishable from a second observation
// of the reference. weight = 1/gate is the definition, so this is derived rather
// than a second source of truth, and it exists so that a report can print the
// gate beside the score without every caller doing the division.
func AdoptionGates() Weights {
	weights := DefaultWeights()

	return Weights{
		PartialFrequency:    1 / weights.PartialFrequency,
		PartialLevel:        1 / weights.PartialLevel,
		PartialDecay:        1 / weights.PartialDecay,
		SpectralEnvelope:    1 / weights.SpectralEnvelope,
		Envelope:            1 / weights.Envelope,
		Glide:               1 / weights.Glide,
		AttackBalance:       1 / weights.AttackBalance,
		Unmatched:           1 / weights.Unmatched,
		Spurious:            1 / weights.Spurious,
		MatchToleranceCents: weights.MatchToleranceCents,
	}
}

// Distance scores a candidate against a reference.
//
// Deliberately absent: any sample-aligned waveform comparison. Against a room
// recording of a different physical drum, waveform error measures the phase
// relationship between two signals that were never meant to share one — it is
// large for candidates that sound identical and small for candidates that do
// not. analysis.CompareSignals keeps that job, for regression between two
// renders of the same model.
func Distance(reference, candidate Features, weights Weights) Terms {
	var terms Terms

	pairs, unmatched, spurious := matchPartials(reference.Partials, candidate.Partials, weights.MatchToleranceCents)

	// Each partial term is blended against a fixed penalty in proportion to
	// the reference energy no candidate partial accounts for.
	//
	// This is not decoration; without it the objective has a degenerate
	// optimum. An error computed only over matched pairs is zero when there
	// are no pairs, so a candidate that produces one partial in the wrong
	// place scores better on three of the eight terms than any real drum can
	// — and the search finds it. Measured: a one-partial candidate reached
	// 11.2 against the shipped default's 39.2, while sounding like nothing at
	// all. Blending makes a partial that is missing cost what a partial that
	// is present but wrong costs, so there is nothing to gain by not having it.
	terms.PartialFrequency = blend(partialFrequencyError(pairs), unmatchedFrequencyCents, unmatched)
	terms.PartialLevel = blend(partialLevelError(pairs), unmatchedLevelDB, unmatched)
	terms.PartialDecay = blend(partialDecayError(pairs), unmatchedDecayLogRatio, unmatched)
	terms.Unmatched = unmatched
	// Not blended into the partial terms above: those pairs really did match,
	// and an invented mode elsewhere does not make a matched one less matched.
	// It is its own failure and it is reported as one.
	terms.Spurious = spurious
	terms.SpectralEnvelope = spectralEnvelopeError(reference.Windows, candidate.Windows)
	terms.Envelope = envelopeError(reference.EnvelopeDB, candidate.EnvelopeDB)
	terms.Glide = glideError(reference, candidate)
	terms.AttackBalance = math.Abs(reference.AttackBalance - candidate.AttackBalance)

	terms.Total = weights.PartialFrequency*terms.PartialFrequency +
		weights.PartialLevel*terms.PartialLevel +
		weights.PartialDecay*terms.PartialDecay +
		weights.SpectralEnvelope*terms.SpectralEnvelope +
		weights.Envelope*terms.Envelope +
		weights.Glide*terms.Glide +
		weights.AttackBalance*terms.AttackBalance +
		weights.Unmatched*terms.Unmatched +
		weights.Spurious*terms.Spurious

	return terms
}

// unreadableGlideCents is what a candidate pays when the reference's pitch bend
// could be measured and the candidate's could not — its fundamental did not
// survive far enough past the strike to place two probes on. See
// match.measureGlide.
//
// One "clearly wrong" glide, and no more. It has to be nonzero: a candidate
// that cannot be measured must not score better on this term than one that can
// and is merely wrong, or the cheapest way to satisfy the objective is to
// produce a drum with no sustain. It must not be large, because a fundamental
// that dies early is already charged by the decay and envelope terms, and
// charging it again here would be double-counting the same fault.
const unreadableGlideCents = 40

// glideError scores the pitch bend, allowing for either side not having one.
//
// A reference with no reading zeroes the term rather than scoring against a
// number that was never measured: there is nothing to compare to, and a
// fabricated zero would silently assert that the reference does not bend.
func glideError(reference, candidate Features) float64 {
	switch {
	case !reference.GlideMeasured:
		return 0
	case !candidate.GlideMeasured:
		return unreadableGlideCents
	default:
		return math.Abs(reference.GlideCents - candidate.GlideCents)
	}
}

// What a reference partial costs when the candidate has nothing to put where
// it was. Each is the error of a partial that is present but as wrong as this
// measure is willing to describe: at the far edge of the matching tolerance,
// four times the "clearly wrong" level error, and a factor of three in ring
// time.
const (
	unmatchedFrequencyCents = 120
	unmatchedLevelDB        = 12
	unmatchedDecayLogRatio  = 1.0986 // ln 3

	// partialAudibilityFloorDB is the level, relative to the strongest partial,
	// at which a partial stops counting for anything — because it is the level
	// at which the detector stops finding it. It must track
	// Options.PartialFloorDB, and TestAudibilityFloorTracksTheDetectionFloor
	// fails if the two drift apart.
	partialAudibilityFloorDB = -42
)

// blend mixes a measured error with the penalty for what could not be
// measured, in the mean-square domain the errors are already RMS values in.
func blend(measured, penalty, missingShare float64) float64 {
	share := min(max(missingShare, 0), 1)

	return math.Sqrt((1-share)*measured*measured + share*penalty*penalty)
}

// pair is one reference partial and the candidate partial identified with it.
type pair struct {
	reference Partial
	candidate Partial
	cents     float64
}

// matchPartials identifies candidate partials with reference partials.
//
// Greedy by closeness rather than in order, so one badly placed candidate
// cannot cascade a mis-identification through the whole series. Each candidate
// is claimed at most once, so a candidate cannot explain two reference modes.
func matchPartials(reference, candidate []Partial, toleranceCents float64) (pairs []pair, unmatchedShare, spuriousShare float64) {
	if len(reference) == 0 {
		return nil, 0, 0
	}

	type link struct {
		ref, cand int
		cents     float64
	}

	var links []link

	for refIndex, refPartial := range reference {
		// The tolerance widens with the mode index: real two-headed drums
		// scatter around the ideal Bessel series by ±20 % in both directions
		// (Richardson, Toulson & Nunn, JASA 131(1) 2012), so demanding the
		// tenth partial land as tightly as the first would make the whole
		// series unmatchable rather than the fit precise.
		tolerance := toleranceCents * (1 + 0.15*float64(refIndex))

		for candIndex, candPartial := range candidate {
			if refPartial.FrequencyHz <= 0 || candPartial.FrequencyHz <= 0 {
				continue
			}

			cents := math.Abs(1200 * math.Log2(candPartial.FrequencyHz/refPartial.FrequencyHz))
			if cents <= tolerance {
				links = append(links, link{ref: refIndex, cand: candIndex, cents: cents})
			}
		}
	}

	slices.SortFunc(links, func(a, b link) int {
		switch {
		case a.cents < b.cents:
			return -1
		case a.cents > b.cents:
			return 1
		default:
			return 0
		}
	})

	usedRef := make([]bool, len(reference))
	usedCand := make([]bool, len(candidate))

	for _, candidateLink := range links {
		if usedRef[candidateLink.ref] || usedCand[candidateLink.cand] {
			continue
		}

		usedRef[candidateLink.ref], usedCand[candidateLink.cand] = true, true

		pairs = append(pairs, pair{
			reference: reference[candidateLink.ref],
			candidate: candidate[candidateLink.cand],
			cents:     candidateLink.cents,
		})
	}

	var missing, total float64

	for index, used := range usedRef {
		weight := partialAudibility(reference[index].LevelDB)

		total += weight

		if !used {
			missing += weight
		}
	}

	if total > 0 {
		unmatchedShare = missing / total
	}

	return pairs, unmatchedShare, spuriousAudibilityShare(reference, candidate, usedCand)
}

// spuriousAudibilityShare is Terms.Spurious: of the candidate's audibility in
// the band the reference demonstrably resolves, how much sits in modes no
// reference partial claimed.
func spuriousAudibilityShare(reference, candidate []Partial, usedCand []bool) float64 {
	low, high := math.Inf(1), math.Inf(-1)

	for _, refPartial := range reference {
		if refPartial.FrequencyHz <= 0 {
			continue
		}

		low, high = min(low, refPartial.FrequencyHz), max(high, refPartial.FrequencyHz)
	}

	if low > high {
		return 0
	}

	var invented, total float64

	for index, used := range usedCand {
		// Outside the reference's own span there is no evidence either way, so
		// the partial is left to the spectral envelope rather than charged here.
		if candidate[index].FrequencyHz < low || candidate[index].FrequencyHz > high {
			continue
		}

		weight := partialAudibility(candidate[index].LevelDB)

		total += weight

		if !used {
			invented += weight
		}
	}

	if total <= 0 {
		return 0
	}

	return invented / total
}

// partialAudibility is what one reference partial is worth: how far it stands,
// in dB, above the level at which the detector would not have found it. Levels
// are relative to the strongest partial, so this is zero at the floor and grows
// with prominence, and it is the compressed alternative to the power weighting
// described on Terms.Unmatched.
func partialAudibility(levelDB float64) float64 {
	return max(0, levelDB-partialAudibilityFloorDB)
}

// retainedShare is how much of the matched set the three partial terms are
// aggregated over. The worst fifth is dropped.
//
// This is not a tolerance for a bad model. It is a measured property of the
// estimator that produces both sides of the comparison. Across matched partials
// the *median* disagreement between two microphones at the same point in space
// is about one cent and about 0.1 in log-T60 — excellent — with a handful of
// catastrophic mis-assignments, and plain RMS converts those few into the
// headline number: one swapped partial in sixteen at 40 dB and 1.4 s of
// disagreement moves the level term by more than every correctly matched
// partial in the table put together. Six rounds of intervention were aimed at
// thresholds that were mostly reporting the estimator's own tail.
//
// A fifth is three partials out of the sixteen this repository extracts, which
// is the observed size of that tail rather than a round number: on the licensed
// reference the merge defect alone accounts for 15 to 24 of 153 to 164 matched
// pairs, and a merged partial is precisely a pair whose decay is a beat rather
// than an exponential.
//
// What this does not do is let a candidate off: a partial dropped here is
// dropped from a term it would have dominated, not from the objective. Anything
// the candidate fails to produce at all is charged by Unmatched, anything it
// invents by Spurious, and the whole spectrum by SpectralEnvelope — none of
// which is trimmed, and all of which a model exploiting the trim would have to
// pay. The trimming is on the *matched* set, where the failure being trimmed
// away is a measurement failure rather than a modelling one.
const retainedShare = 0.8

// trimmedRootMeanSquare is the RMS over the smallest retainedShare of its
// inputs, which are squared errors.
//
// At least one value is always retained, so a short table degrades to its own
// best member rather than to zero.
func trimmedRootMeanSquare(squares []float64) float64 {
	if len(squares) == 0 {
		return 0
	}

	ordered := slices.Clone(squares)
	slices.Sort(ordered)

	// Truncated rather than rounded, which is what makes a small table safe.
	// int() discards nothing below five values, so a four-partial comparison is
	// a plain RMS: with four partials there is no basis for calling any one of
	// them the estimator's tail, and discarding one would hide a real
	// single-partial error instead of an artifact.
	// TestDistanceIsolatesWhatChanged/rebalancing is exactly that case.
	retained := len(ordered) - int((1-retainedShare)*float64(len(ordered)))

	sum := 0.0
	for _, square := range ordered[:retained] {
		sum += square
	}

	return math.Sqrt(sum / float64(retained))
}

func partialFrequencyError(pairs []pair) float64 {
	squares := make([]float64, 0, len(pairs))
	for _, matched := range pairs {
		squares = append(squares, matched.cents*matched.cents)
	}

	return trimmedRootMeanSquare(squares)
}

func partialLevelError(pairs []pair) float64 {
	if len(pairs) == 0 {
		return 0
	}

	// Both sides are already referred to their own strongest partial, but that
	// need not be the *same* partial, so re-referring to the matched set keeps
	// this a comparison of balance rather than of which mode won.
	refOffset, candOffset := math.Inf(-1), math.Inf(-1)

	for _, matched := range pairs {
		refOffset = max(refOffset, matched.reference.LevelDB)
		candOffset = max(candOffset, matched.candidate.LevelDB)
	}

	squares := make([]float64, 0, len(pairs))

	for _, matched := range pairs {
		delta := (matched.reference.LevelDB - refOffset) - (matched.candidate.LevelDB - candOffset)
		squares = append(squares, delta*delta)
	}

	return trimmedRootMeanSquare(squares)
}

func partialDecayError(pairs []pair) float64 {
	squares := make([]float64, 0, len(pairs))

	for _, matched := range pairs {
		// A partial with no fitted decay on either side contributes nothing
		// rather than a made-up ratio. What is missing is charged by Unmatched,
		// which is where a missing thing belongs.
		if matched.reference.T60Seconds <= 0 || matched.candidate.T60Seconds <= 0 {
			continue
		}

		ratio := math.Log(matched.reference.T60Seconds / matched.candidate.T60Seconds)
		squares = append(squares, ratio*ratio)
	}

	return trimmedRootMeanSquare(squares)
}

func spectralEnvelopeError(reference, candidate []WindowFeature) float64 {
	if len(reference) == 0 || len(candidate) == 0 {
		return 0
	}

	var (
		sum     float64
		counted int
	)

	for _, refWindow := range reference {
		index := slices.IndexFunc(candidate, func(w WindowFeature) bool {
			return w.Name == refWindow.Name
		})
		if index < 0 {
			continue
		}

		sum += rmsDifference(refWindow.BandDB, candidate[index].BandDB)
		counted++
	}

	if counted == 0 {
		return 0
	}

	return sum / float64(counted)
}

func envelopeError(reference, candidate []float64) float64 {
	return rmsDifference(reference, candidate)
}

// rmsDifference compares the overlapping prefix of two dB vectors.
func rmsDifference(first, second []float64) float64 {
	overlap := min(len(first), len(second))
	if overlap == 0 {
		return 0
	}

	sum := 0.0

	for i := range overlap {
		delta := first[i] - second[i]
		sum += delta * delta
	}

	return math.Sqrt(sum / float64(overlap))
}
