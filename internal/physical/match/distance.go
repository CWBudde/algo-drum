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
	// Unmatched is the share of the reference's partial *energy* that no
	// candidate partial accounts for. It is mostly a diagnostic — the three
	// terms above already carry the cost of what is missing — but it keeps a
	// small weight of its own, because a drum with the right partials and one
	// gone is worse than the terms alone say.
	//
	// Energy-weighted rather than counted, because a missing partial matters
	// exactly as much as it is loud. The reference tom has a genuine, isolated
	// component at 87 Hz that is 39 dB down; counting would make failing to
	// reproduce it as expensive as losing the fundamental, and a floor tuned to
	// exclude it would also throw away the 500–700 Hz cluster that does matter.
	Unmatched float64 `json:"unmatchedEnergyFraction"`
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

	pairs, unmatched := matchPartials(reference.Partials, candidate.Partials, weights.MatchToleranceCents)

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
		weights.Unmatched*terms.Unmatched

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
func matchPartials(reference, candidate []Partial, toleranceCents float64) (pairs []pair, unmatchedEnergy float64) {
	if len(reference) == 0 {
		return nil, 0
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
		energy := math.Pow(10, reference[index].LevelDB/10)

		total += energy

		if !used {
			missing += energy
		}
	}

	if total <= 0 {
		return pairs, 0
	}

	return pairs, missing / total
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
