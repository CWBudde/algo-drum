//go:build purego

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
)

// report is the part of cmd/fit-physical's output the figures need. The report
// carries both feature sets in full, which is what lets every figure here be
// reproduced from a committed artefact rather than from the recording — that
// recording is not redistributable, so a figure pipeline that needed it would
// not be reproducible by anyone else.
type report struct {
	Baseline struct {
		Terms terms `json:"terms"`
	} `json:"baseline"`
	Best struct {
		Terms    terms    `json:"terms"`
		Params   []param  `json:"params"`
		Features features `json:"features"`
	} `json:"best"`
	Target features `json:"target"`
}

type terms struct {
	PartialFrequency float64 `json:"partialFrequencyCents"`
	PartialLevel     float64 `json:"partialLevelDB"`
	PartialDecay     float64 `json:"partialDecayLogRatio"`
	SpectralEnvelope float64 `json:"spectralEnvelopeDB"`
	Envelope         float64 `json:"envelopeDB"`
	Glide            float64 `json:"glideCents"`
	AttackBalance    float64 `json:"attackBalanceDB"`
	Unmatched        float64 `json:"unmatchedShare"`
	Spurious         float64 `json:"spuriousShare"`
	Total            float64 `json:"total"`
}

type param struct {
	Label      string  `json:"label"`
	Normalized float64 `json:"normalized"`
	Value      float64 `json:"value"`
}

type features struct {
	Partials      []partial `json:"partials"`
	Windows       []window  `json:"windows"`
	BandCentresHz []float64 `json:"bandCentresHz"`
}

type partial struct {
	FrequencyHz float64 `json:"frequencyHz"`
	LevelDB     float64 `json:"levelDB"`
	T60Seconds  float64 `json:"t60Seconds"`
	FitQuality  float64 `json:"fitQuality"`
}

type window struct {
	Name         string    `json:"name"`
	StartSeconds float64   `json:"startSeconds"`
	EndSeconds   float64   `json:"endSeconds"`
	BandDB       []float64 `json:"bandDB"`
}

func loadReport(path string) (*report, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var parsed report
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	if len(parsed.Target.Partials) == 0 {
		return nil, fmt.Errorf("%s carries no reference partials — is it a -report-only run?", path)
	}

	return &parsed, nil
}

// matchToleranceCents mirrors match.DefaultWeights().MatchToleranceCents, and
// the widening below mirrors matchPartials. The figures have to reproduce the
// matching rather than approximate it, or they would illustrate a pairing the
// distance never made.
const matchToleranceCents = 120

type pairing struct {
	links      [][2]int
	matchedRef map[int]bool
	matchedCan map[int]bool
}

func matchPartials(reference, candidate []partial) pairing {
	type link struct {
		cents    float64
		ref, can int
	}

	var links []link

	for refIndex, ref := range reference {
		tolerance := matchToleranceCents * (1 + 0.15*float64(refIndex))

		for canIndex, can := range candidate {
			if ref.FrequencyHz <= 0 || can.FrequencyHz <= 0 {
				continue
			}

			cents := math.Abs(1200 * math.Log2(can.FrequencyHz/ref.FrequencyHz))
			if cents <= tolerance {
				links = append(links, link{cents: cents, ref: refIndex, can: canIndex})
			}
		}
	}

	sort.Slice(links, func(i, j int) bool { return links[i].cents < links[j].cents })

	result := pairing{matchedRef: map[int]bool{}, matchedCan: map[int]bool{}}

	for _, candidateLink := range links {
		if result.matchedRef[candidateLink.ref] || result.matchedCan[candidateLink.can] {
			continue
		}

		result.matchedRef[candidateLink.ref] = true
		result.matchedCan[candidateLink.can] = true
		result.links = append(result.links, [2]int{candidateLink.ref, candidateLink.can})
	}

	return result
}

func (p *pairing) split(count int, matched map[int]bool) (in, out []int) {
	for index := range count {
		if matched[index] {
			in = append(in, index)
		} else {
			out = append(out, index)
		}
	}

	return in, out
}

func referenceSpan(partials []partial) (low, high float64) {
	low, high = math.Inf(1), math.Inf(-1)

	for _, p := range partials {
		if p.FrequencyHz <= 0 {
			continue
		}

		low, high = math.Min(low, p.FrequencyHz), math.Max(high, p.FrequencyHz)
	}

	return low, high
}

// errNoBands guards a report written before band centres were recorded.
var errNoBands = errors.New("report has no band centres")
