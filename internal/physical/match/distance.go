package match

import (
	"math"
	"slices"
)

// Terms is the distance broken into the things a listener would name
// separately. Each is reported in its own unit, so a number here can be read
// against a tolerance rather than only against another run.
type Terms struct {
	// PartialFrequency is the RMS cents error over matched partials. Cents,
	// because the ear hears pitch as a ratio and a 3 Hz error means something
	// different at 118 Hz than at 1180 Hz.
	PartialFrequency float64 `json:"partialFrequencyCents"`
	// PartialLevel is the RMS dB error of the partials' relative levels — the
	// balance between the fundamental and the modes above it.
	PartialLevel float64 `json:"partialLevelDB"`
	// PartialDecay is the RMS of |ln(T60_ref / T60_candidate)|, weighted by
	// each partial's fit quality. A log ratio, because ringing twice as long
	// and half as long are the same size of mistake.
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
// The scale is set by pitch: 25 cents is the point where a tuning error stops
// being a nuance, so PartialFrequency carries 1/25 and every other weight is
// the reciprocal of that term's own "clearly wrong" threshold — 3 dB of
// partial balance, a ln-ratio of 0.35 (a factor of 1.4 in ring time), 4 dB of
// spectral shape, 3 dB of envelope, 40 cents of glide, 6 dB of attack balance.
// Unmatched is small because the partial terms already absorb what is missing;
// what is left is a mild preference for completeness.
//
// Spurious carries the same weight as Unmatched, and the symmetry is load-
// bearing rather than tidy. It was first set larger, at 1/0.2, on the reasoning
// that nothing else in the sum absorbs an invented partial while a missing one
// is charged twice — once directly and once through the blend. A fit run
// refuted that inside fourteen minutes: it abandoned the drum and converged on
// two partials with a spurious share of 0.000, because the blend's pressure
// toward completeness is exactly what the spurious weight works against, and
// outweighing it makes emptiness the cheapest bank on offer. Measured on that
// run's own candidates the two degenerate extremes came out within 0.12 of each
// other, so the search simply drifted between them.
//
// At equal weights the extra asymmetry the argument wanted is still there — it
// just comes from the blend, where it belongs, rather than from the weight.
// TestSpuriousDoesNotOutweighCompleteness pins the inequality; it cannot pin the
// behaviour, for the reason given there.
func DefaultWeights() Weights {
	return Weights{
		PartialFrequency:    1.0 / 25,
		PartialLevel:        1.0 / 3,
		PartialDecay:        1.0 / 0.35,
		SpectralEnvelope:    1.0 / 4,
		Envelope:            1.0 / 3,
		Glide:               1.0 / 40,
		AttackBalance:       1.0 / 6,
		Unmatched:           2.0,
		Spurious:            2.0,
		MatchToleranceCents: 120,
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
	terms.Glide = math.Abs(reference.GlideCents - candidate.GlideCents)
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

func partialFrequencyError(pairs []pair) float64 {
	if len(pairs) == 0 {
		return 0
	}

	sum := 0.0
	for _, matched := range pairs {
		sum += matched.cents * matched.cents
	}

	return math.Sqrt(sum / float64(len(pairs)))
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

	sum := 0.0

	for _, matched := range pairs {
		delta := (matched.reference.LevelDB - refOffset) - (matched.candidate.LevelDB - candOffset)
		sum += delta * delta
	}

	return math.Sqrt(sum / float64(len(pairs)))
}

func partialDecayError(pairs []pair) float64 {
	var sum, weight float64

	for _, matched := range pairs {
		if matched.reference.T60Seconds <= 0 || matched.candidate.T60Seconds <= 0 {
			continue
		}
		// A partial whose envelope is not an exponential — a beating pair, a
		// mode buried under its neighbour — has a meaningless slope. Weighting
		// by the product of the two fits' R² lets its frequency still count
		// while its decay does not.
		confidence := matched.reference.FitQuality * matched.candidate.FitQuality
		if confidence <= 0 {
			continue
		}

		ratio := math.Log(matched.reference.T60Seconds / matched.candidate.T60Seconds)
		sum += confidence * ratio * ratio
		weight += confidence
	}

	if weight == 0 {
		return 0
	}

	return math.Sqrt(sum / weight)
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
