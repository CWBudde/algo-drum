package main

import (
	"fmt"
	"math"

	"github.com/cwbudde/algo-drum/internal/physical/match"
	"github.com/cwbudde/algo-dsp/measure/ir"
)

// Report is the derived table set. This — not the audio — is what a measurement
// session commits, so it carries the options it was produced with and every
// health number a later reader would need to decide whether to trust it.
type Report struct {
	Options       match.Options  `json:"options"`
	Base          BaseRule       `json:"base"`
	Takes         []Take         `json:"takes"`
	Repeatability *Repeatability `json:"repeatability,omitempty"`
	Doublet       *Doublet       `json:"doublet,omitempty"`
}

// BaseRule records how the partial the ratios are taken against was chosen. It
// is in the report rather than only in the flags because every ratio in every
// table depends on it, and a reader who disagrees needs to be able to see the
// rule before re-deriving them.
type BaseRule struct {
	WindowDB   float64 `json:"windowDB"`
	ForcedHz   float64 `json:"forcedHz,omitempty"`
	Tolerance  float64 `json:"forcedToleranceRatio,omitempty"`
	Autoselect bool    `json:"autoselect"`
}

// Take is one recorded hit, or one file of them.
type Take struct {
	Path     string  `json:"path"`
	Channel  string  `json:"channel"`
	Format   Format  `json:"format"`
	Health   Health  `json:"health"`
	Onset    float64 `json:"onsetSeconds"`
	BaseHz   float64 `json:"baseFrequencyHz"`
	Partials []Row   `json:"partials"`

	// GlideCents is the fundamental's early-to-late frequency change — the
	// lowest partial standing within match.Options.GlidePartialWindowDB of the
	// loudest, not the loudest one. On a real drum it is the Berger
	// nonlinearity, so it belongs beside the tension coefficients rather than
	// beside the mode table.
	GlideCents float64 `json:"glideCents"`
	// GlideMeasured says whether GlideCents is a reading at all. False means
	// the fundamental had decayed too far for a second probe to be placed on
	// it, and GlideCents is zero for want of a number rather than because the
	// pitch held. The two are worth telling apart here more than anywhere: a
	// take of a real drum, quietly struck or heavily damped, is exactly where a
	// dead fundamental is likely, and a flat glide reported from one would be
	// read straight into the tension coefficients.
	GlideMeasured   bool    `json:"glideMeasured"`
	AttackBalanceDB float64 `json:"attackBalanceDB"`

	Decay ir.Metrics `json:"decay"`
}

// Format records what was decoded, because a table read a year later has to be
// able to say what rate and depth produced it.
type Format struct {
	SampleRateHz float64 `json:"sampleRateHz"`
	Channels     int     `json:"channels"`
	BitDepth     int     `json:"bitDepth"`
	Frames       int     `json:"frames"`
	// ChannelDelaySamples and ChannelCorrelation describe a stereo pair before
	// it was reduced; see match.Reference. A large delay at high correlation
	// means two microphones at different distances, and the mono sum was
	// aligned rather than combed.
	ChannelDelaySamples int     `json:"channelDelaySamples,omitempty"`
	ChannelCorrelation  float64 `json:"channelCorrelation,omitempty"`
}

// Health is the "is this recording worth anything" block, printed before the
// partial table on purpose. Every field here corresponds to a failure mode
// listed in docs/physical-measurement-protocol.md.
type Health struct {
	PeakAmplitude   float64 `json:"peakAmplitude"`
	ClippedSamples  int     `json:"clippedSamples"`
	DCOffset        float64 `json:"dcOffset"`
	PreOnsetSeconds float64 `json:"preOnsetSeconds"`
	// PreOnsetFloorDB is the pre-onset RMS relative to the take's peak, and is
	// absent when there is too little pre-roll to measure one. It bounds every
	// T60 in the take: a decay cannot be followed below it.
	PreOnsetFloorDB *float64 `json:"preOnsetFloorDB,omitempty"`
	// AnalyzedSeconds is how much signal the onset left, against the requested
	// analysis span.
	AnalyzedSeconds float64  `json:"analyzedSeconds"`
	Warnings        []string `json:"warnings,omitempty"`
}

// Row is one partial, with the two derived quantities the model wants.
type Row struct {
	FrequencyHz float64 `json:"frequencyHz"`
	// LevelDB is relative to the take's strongest partial, so it is a balance
	// and survives any gain setting.
	LevelDB     float64 `json:"levelDB"`
	RatioToBase float64 `json:"ratioToBase"`
	T60Seconds  float64 `json:"t60Seconds"`
	// DampingRatioPercent is zeta = gamma/omega with gamma = ln(1000)/T60 — the
	// quantity Head.Loss1MPerSecond is zeta*c of, and the one the literature
	// reports. Zero when the decay fit did not converge.
	DampingRatioPercent float64 `json:"dampingRatioPercent"`
	FitQuality          float64 `json:"fitQuality"`
}

// Repeatability is the scatter across takes at one nominal dynamic. It sizes
// the per-trigger jitter PLAN item S7 proposes and no config field yet carries.
//
// It is only meaningful when the takes really are repeats: the tool emits it
// whenever more than one file is measured without -doublet, and the caller is
// responsible for not reading it off a strike-position series.
type Repeatability struct {
	Takes           int     `json:"takes"`
	MeanBaseHz      float64 `json:"meanBaseFrequencyHz"`
	BaseSDCents     float64 `json:"baseFrequencySDCents"`
	BaseSpreadCents float64 `json:"baseFrequencySpreadCents"`
	MeanBaseT60     float64 `json:"meanBaseT60Seconds"`
	BaseT60SDPct    float64 `json:"baseT60SDPercent"`
	// PeakSpreadDB is how far apart the takes' raw peak amplitudes are. A base
	// frequency spread that tracks this one is the tension nonlinearity working
	// as designed, not jitter.
	PeakSpreadDB       float64 `json:"peakSpreadDB"`
	AttackBalanceSDDB  float64 `json:"attackBalanceSDDB"`
	MissingBaseT60Take int     `json:"takesWithoutBaseT60,omitempty"`
}

// Doublet is Fischer's protocol applied to a tom: the (0,1) with the resonant
// head off, and the pair it splits into once the head is back on at unchanged
// batter tuning. It is the direct measurement of what Cavity.StiffnessScale is
// fitted to.
type Doublet struct {
	SingleHeadPath string `json:"singleHeadPath"`
	DoubleHeadPath string `json:"doubleHeadPath"`

	SingleHz float64 `json:"singleHeadHz"`
	LowerHz  float64 `json:"lowerBranchHz"`
	UpperHz  float64 `json:"upperBranchHz"`

	LowerLevelDB float64 `json:"lowerBranchLevelDB"`
	UpperLevelDB float64 `json:"upperBranchLevelDB"`

	// SplitRatio is upper/lower — the separation the model's fit targets, and
	// the quantity eigenvalue interlacing says is the one a fit can move.
	SplitRatio float64 `json:"splitRatio"`
	// UpperOverSingle is the ratio Fischer published as 215/186 = 1.16, and
	// LowerOverSingle is how far interlacing let the audible branch move.
	// Reporting both is what disambiguates the two readings of that number.
	UpperOverSingle float64 `json:"upperOverSingle"`
	LowerOverSingle float64 `json:"lowerOverSingle"`

	// SearchMinRatio and SearchMaxRatio are the window the upper branch was
	// picked from, echoed so the choice is visible. Candidates lists every
	// partial above the lower branch, including those outside the window, so a
	// reader can overrule it.
	SearchMinRatio float64  `json:"searchMinRatio"`
	SearchMaxRatio float64  `json:"searchMaxRatio"`
	Candidates     []Row    `json:"candidates"`
	Warnings       []string `json:"warnings,omitempty"`
}

const (
	// clipThreshold is where a 16- or 24-bit sample counts as pinned. Slightly
	// under 1 because a converter's full scale lands a hair below it and an
	// exactly-1.0 test would miss a clipped file.
	clipThreshold = 0.9995
	// minPreRollSeconds is how much silence a take needs before the onset for
	// its noise floor to mean anything.
	minPreRollSeconds = 0.05
	// quietFloorDB is how far below the peak the noise floor must sit for the
	// decay fit's own -45 dB floor to be reachable in the drum rather than in
	// the room.
	quietFloorDB = -45
	// dcOffsetLimit is where a DC offset stops being a rounding artefact and
	// starts being a broken input stage.
	dcOffsetLimit = 1e-3
	// weakFitQuality is the R² below which a decay is not a single exponential
	// — a beating pair, or a partial that fell into the noise.
	weakFitQuality = 0.9
)

// measureTake loads one file and reduces it to the tables the protocol asks
// for. base selects which partial the ratios are taken against.
func measureTake(path string, channel match.Channel, options match.Options, base BaseRule) (Take, error) {
	reference, err := match.LoadReference(path, channel)
	if err != nil {
		return Take{}, fmt.Errorf("%s: %w", path, err)
	}

	features, err := match.Extract(reference.Samples, reference.SampleRateHz, options)
	if err != nil {
		return Take{}, fmt.Errorf("%s: %w", path, err)
	}

	take := Take{
		Path:    path,
		Channel: string(channel),
		Format: Format{
			SampleRateHz:        reference.SampleRateHz,
			Channels:            reference.Channels,
			BitDepth:            reference.BitDepth,
			Frames:              len(reference.Samples),
			ChannelDelaySamples: reference.ChannelDelaySamples,
			ChannelCorrelation:  reference.ChannelCorrelation,
		},
		Onset:           float64(features.OnsetSample) / reference.SampleRateHz,
		GlideCents:      features.GlideCents,
		GlideMeasured:   features.GlideMeasured,
		AttackBalanceDB: features.AttackBalance,
		Decay:           features.Decay,
	}

	index := baseIndex(features.Partials, base)
	if index >= 0 {
		take.BaseHz = features.Partials[index].FrequencyHz
	}

	take.Partials = rows(features.Partials, take.BaseHz)
	take.Health = takeHealth(reference, features, options)

	if base.ForcedHz > 0 && index < 0 {
		take.Health.Warnings = append(take.Health.Warnings, fmt.Sprintf(
			"no partial within %.0f %% of the requested base %.1f Hz: this take has no ratios",
			100*base.Tolerance, base.ForcedHz,
		))
	}

	if index >= 0 && features.Partials[index].FitQuality < weakFitQuality {
		take.Health.Warnings = append(take.Health.Warnings, fmt.Sprintf(
			"base partial decay fit R²=%.2f: its envelope is not a single exponential, so its T60 is not usable",
			features.Partials[index].FitQuality,
		))
	}

	return take, nil
}

// baseIndex picks the partial the ratios are taken against: the lowest one that
// is still within WindowDB of the strongest, or the one nearest ForcedHz.
//
// Not simply the lowest, because the lowest detected peak on a real recording
// may be a room mode or a shell rattle tens of dB down — features.go makes the
// same argument about reading the glide off the lowest partial. Not simply the
// loudest either, because on an off-centre strike that is often the (1,1) and
// the ratios would then be taken against the wrong mode.
//
// The window has to be wide, and how wide is a measurement rather than a taste:
// S5 measured the model's fundamental 9.78 dB below the strongest partial at
// the shipped 0.30 R strike, and the shipped voice's own render — reproducible
// with `go run ./cmd/render-physical && go run ./cmd/measure-tom` — puts it
// 27 dB down once the attack layer is included. A 20 dB window would therefore
// take the ratios against a mode two octaves up on the model's own output. The
// default is 30, which still sits 12 dB clear of the -42 dB detection floor.
//
// When even that is wrong for a particular drum, -base-hz overrules it, which
// is the honest escape hatch: naming the fundamental is a judgement the person
// who recorded the take is better placed to make than a rule is.
func baseIndex(partials []match.Partial, base BaseRule) int {
	if base.ForcedHz > 0 {
		nearest, distance := -1, math.Inf(1)

		for index, partial := range partials {
			if offset := math.Abs(partial.FrequencyHz - base.ForcedHz); offset < distance {
				nearest, distance = index, offset
			}
		}

		if nearest >= 0 && distance <= base.Tolerance*base.ForcedHz {
			return nearest
		}

		return -1
	}

	for index, partial := range partials {
		if partial.LevelDB >= -base.WindowDB {
			return index
		}
	}

	if len(partials) > 0 {
		return 0
	}

	return -1
}

func rows(partials []match.Partial, baseHz float64) []Row {
	out := make([]Row, 0, len(partials))

	for _, partial := range partials {
		row := Row{
			FrequencyHz: partial.FrequencyHz,
			LevelDB:     partial.LevelDB,
			T60Seconds:  partial.T60Seconds,
			FitQuality:  partial.FitQuality,
		}

		if baseHz > 0 {
			row.RatioToBase = partial.FrequencyHz / baseHz
		}

		row.DampingRatioPercent = dampingRatioPercent(partial.FrequencyHz, partial.T60Seconds)

		out = append(out, row)
	}

	return out
}

// dampingRatioPercent converts a ring time into the fraction of critical
// damping, which is what the loss law and the literature are both written in.
func dampingRatioPercent(frequencyHz, t60Seconds float64) float64 {
	if frequencyHz <= 0 || t60Seconds <= 0 {
		return 0
	}

	decayRate := math.Log(1000) / t60Seconds

	return 100 * decayRate / (2 * math.Pi * frequencyHz)
}

func takeHealth(reference match.Reference, features match.Features, options match.Options) Health {
	samples := reference.Samples
	rate := reference.SampleRateHz

	health := Health{
		PreOnsetSeconds: float64(features.OnsetSample) / rate,
		AnalyzedSeconds: float64(len(samples)-features.OnsetSample) / rate,
	}

	var sum float64

	for _, sample := range samples {
		magnitude := math.Abs(sample)

		if magnitude > health.PeakAmplitude {
			health.PeakAmplitude = magnitude
		}

		if magnitude >= clipThreshold {
			health.ClippedSamples++
		}

		sum += sample
	}

	if len(samples) > 0 {
		health.DCOffset = sum / float64(len(samples))
	}

	if health.PreOnsetSeconds >= minPreRollSeconds && health.PeakAmplitude > 0 {
		var energy float64

		for _, sample := range samples[:features.OnsetSample] {
			energy += sample * sample
		}

		rms := math.Sqrt(energy / float64(features.OnsetSample))
		if rms > 0 {
			floor := 20 * math.Log10(rms/health.PeakAmplitude)
			health.PreOnsetFloorDB = &floor
		}
	}

	health.Warnings = healthWarnings(health, rate, options)

	return health
}

func healthWarnings(health Health, sampleRateHz float64, options match.Options) []string {
	var warnings []string

	if health.ClippedSamples > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"%d samples at full scale: the attack is clipped and manufactures partials at every frequency",
			health.ClippedSamples,
		))
	}

	if health.PreOnsetSeconds < minPreRollSeconds {
		warnings = append(warnings, fmt.Sprintf(
			"only %.0f ms before the onset: the noise floor cannot be measured and the strike may be truncated",
			1000*health.PreOnsetSeconds,
		))
	}

	if health.PreOnsetFloorDB != nil && *health.PreOnsetFloorDB > quietFloorDB {
		warnings = append(warnings, fmt.Sprintf(
			"noise floor %.1f dB below peak: decays fitted past that level are reading the room, not the drum",
			*health.PreOnsetFloorDB,
		))
	}

	if math.Abs(health.DCOffset) > dcOffsetLimit {
		warnings = append(warnings, fmt.Sprintf(
			"DC offset %.4f: the input stage is not centred", health.DCOffset,
		))
	}

	if health.AnalyzedSeconds < options.AnalysisSeconds {
		warnings = append(warnings, fmt.Sprintf(
			"only %.2f s after the onset against an analysis span of %.2f s: the tail is missing",
			health.AnalyzedSeconds, options.AnalysisSeconds,
		))
	}

	if sampleRateHz < 44100 {
		warnings = append(warnings, fmt.Sprintf(
			"%.0f Hz sample rate: below the band the attack layer is fitted over", sampleRateHz,
		))
	}

	return warnings
}

// measureRepeatability summarizes the scatter across takes at one dynamic.
func measureRepeatability(takes []Take) *Repeatability {
	if len(takes) < 2 {
		return nil
	}

	var (
		frequencies []float64
		t60s        []float64
		balances    []float64
		peaks       []float64
		missing     int
	)

	for _, take := range takes {
		peaks = append(peaks, take.Health.PeakAmplitude)
		balances = append(balances, take.AttackBalanceDB)

		if take.BaseHz <= 0 {
			missing++

			continue
		}

		frequencies = append(frequencies, take.BaseHz)

		if t60 := baseT60(take); t60 > 0 {
			t60s = append(t60s, t60)
		} else {
			missing++
		}
	}

	if len(frequencies) < 2 {
		return nil
	}

	meanHz := mean(frequencies)

	// In cents rather than hertz, because that is the unit the spread has to be
	// judged in: a jitter amount is a fraction of a frequency, not an offset.
	cents := make([]float64, len(frequencies))
	for index, frequency := range frequencies {
		cents[index] = 1200 * math.Log2(frequency/meanHz)
	}

	repeat := &Repeatability{
		Takes:              len(takes),
		MeanBaseHz:         meanHz,
		BaseSDCents:        standardDeviation(cents),
		BaseSpreadCents:    spread(cents),
		AttackBalanceSDDB:  standardDeviation(balances),
		MissingBaseT60Take: missing,
	}

	if len(t60s) >= 2 {
		repeat.MeanBaseT60 = mean(t60s)
		if repeat.MeanBaseT60 > 0 {
			repeat.BaseT60SDPct = 100 * standardDeviation(t60s) / repeat.MeanBaseT60
		}
	}

	repeat.PeakSpreadDB = peakSpreadDB(peaks)

	return repeat
}

func baseT60(take Take) float64 {
	for _, row := range take.Partials {
		if row.FrequencyHz == take.BaseHz {
			return row.T60Seconds
		}
	}

	return 0
}

func peakSpreadDB(peaks []float64) float64 {
	lowest, highest := math.Inf(1), 0.0

	for _, peak := range peaks {
		if peak <= 0 {
			continue
		}

		lowest = min(lowest, peak)
		highest = max(highest, peak)
	}

	if !(lowest > 0) || highest <= 0 {
		return 0
	}

	return 20 * math.Log10(highest/lowest)
}

// measureDoublet compares a resonant-head-off take against a resonant-head-on
// one. single is the take without the resonant head.
func measureDoublet(single, double Take, minRatio, maxRatio float64) *Doublet {
	if single.BaseHz <= 0 || double.BaseHz <= 0 {
		return nil
	}

	doublet := &Doublet{
		SingleHeadPath: single.Path,
		DoubleHeadPath: double.Path,
		SingleHz:       single.BaseHz,
		LowerHz:        double.BaseHz,
		SearchMinRatio: minRatio,
		SearchMaxRatio: maxRatio,
	}

	for _, row := range double.Partials {
		if row.FrequencyHz > double.BaseHz {
			doublet.Candidates = append(doublet.Candidates, row)
		}

		if row.FrequencyHz == double.BaseHz {
			doublet.LowerLevelDB = row.LevelDB
		}
	}

	// The strongest candidate inside the window, not the nearest: the stiffened
	// branch is a genuine partial of the coupled system and stands well above
	// the skirt of anything else in that span.
	best := -1

	for index, row := range doublet.Candidates {
		ratio := row.FrequencyHz / double.BaseHz
		if ratio < minRatio || ratio > maxRatio {
			continue
		}

		if best < 0 || row.LevelDB > doublet.Candidates[best].LevelDB {
			best = index
		}
	}

	if best < 0 {
		doublet.Warnings = append(doublet.Warnings, fmt.Sprintf(
			"no partial between %.2f× and %.2f× the lower branch: "+
				"either the coupling is far stronger than the fitted model's "+
				"(the rigid formula predicts about 1.9×) or the strike was not central",
			minRatio, maxRatio,
		))

		return doublet
	}

	doublet.UpperHz = doublet.Candidates[best].FrequencyHz
	doublet.UpperLevelDB = doublet.Candidates[best].LevelDB
	doublet.SplitRatio = doublet.UpperHz / doublet.LowerHz
	doublet.UpperOverSingle = doublet.UpperHz / doublet.SingleHz
	doublet.LowerOverSingle = doublet.LowerHz / doublet.SingleHz

	doublet.Warnings = append(doublet.Warnings, doubletWarnings(doublet)...)

	return doublet
}

func doubletWarnings(doublet *Doublet) []string {
	var warnings []string

	// Eigenvalue interlacing pins the lower branch between the two heads'
	// uncoupled (0,1) frequencies, so it cannot fall below the single-head one
	// by any amount the coupling can produce. If it has, the batter tuning
	// moved between the takes and the split ratio is void.
	if drift := 1200 * math.Log2(doublet.LowerHz/doublet.SingleHz); drift < -5 {
		warnings = append(warnings, fmt.Sprintf(
			"the lower branch is %.0f cents *below* the single-head fundamental, "+
				"which coupling cannot do: the batter tuning moved between the takes",
			-drift,
		))
	}

	if doublet.SplitRatio < 1.10 || doublet.SplitRatio > 1.20 {
		warnings = append(warnings, fmt.Sprintf(
			"split ratio %.3f is outside the 1.10-1.20 band Fischer's snare and the "+
				"shipped Cavity.StiffnessScale=0.083 both sit in — a finding if the "+
				"capture is clean, so check the strike was central and the tuning held",
			doublet.SplitRatio,
		))
	}

	return warnings
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sum float64
	for _, value := range values {
		sum += value
	}

	return sum / float64(len(values))
}

func standardDeviation(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	average := mean(values)

	var sum float64
	for _, value := range values {
		sum += (value - average) * (value - average)
	}

	// Sample standard deviation: these are a handful of takes drawn from the
	// player, not the population.
	return math.Sqrt(sum / float64(len(values)-1))
}

func spread(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	lowest, highest := values[0], values[0]

	for _, value := range values {
		lowest = min(lowest, value)
		highest = max(highest, value)
	}

	return highest - lowest
}
