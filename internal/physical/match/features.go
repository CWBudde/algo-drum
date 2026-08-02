package match

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/cwbudde/algo-dsp/dsp/filter/biquad"
	"github.com/cwbudde/algo-dsp/dsp/filter/design"
	"github.com/cwbudde/algo-dsp/measure/ir"
	frequencystats "github.com/cwbudde/algo-dsp/stats/frequency"
	timestats "github.com/cwbudde/algo-dsp/stats/time"
)

// ErrInvalidOptions reports feature options that cannot describe an analysis.
var ErrInvalidOptions = errors.New("invalid match options")

// The decibel conversions, folded into single constants.
//
// Both substitutions are identities — 20*log10(x) is (20/ln10)*ln(x), and
// 10^(x/20) is exp(x*ln10/20) — so what changes is only where the rounding
// falls, by well under an ulp of the result. They are worth spelling out
// because the library forms are not one operation each: Go's Log10 is a Frexp,
// a Log and two multiplies, and Pow with a constant base is a general power.
// These run per spectral bin and per trace point, tens of thousands of times
// per extraction.
const (
	// decibelsPerPower converts a natural log of power to decibels: the folded
	// 10/ln(10). decay.go's powerDecibels applies the same constant.
	decibelsPerPower = 10 / math.Ln10
	// decibelsPerAmplitude is the same for an amplitude ratio: 20/ln(10).
	decibelsPerAmplitude = 20 / math.Ln10
	// amplitudePerDecibel inverts decibelsPerAmplitude, for exp() in place of
	// math.Pow(10, dB/20).
	amplitudePerDecibel = math.Ln10 / 20
)

// TimeWindow is one span of the hit, measured from the onset.
type TimeWindow struct {
	Name         string  `json:"name"`
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds"`
}

// Options controls feature extraction. The zero value is not usable; start
// from DefaultOptions.
type Options struct {
	// AnalysisSeconds bounds everything measured here. The default stops
	// before a typical room tail has decided the answer.
	AnalysisSeconds float64 `json:"analysisSeconds"`

	// Partial detection.
	MaxPartials      int     `json:"maxPartials"`
	MinFrequencyHz   float64 `json:"minFrequencyHz"`
	MaxFrequencyHz   float64 `json:"maxFrequencyHz"`
	PartialFloorDB   float64 `json:"partialFloorDB"`   // relative to the strongest peak
	PeakProminenceDB float64 `json:"peakProminenceDB"` // how far a peak must clear its skirt
	MinSeparationHz  float64 `json:"minSeparationHz"`  // rejects one lobe picked twice
	SustainStartSecs float64 `json:"sustainStartSecs"` // partial detection window
	SustainEndSecs   float64 `json:"sustainEndSecs"`
	// A second, earlier detection window. The sustain window cannot see a
	// partial that is over before it closes; this one can. See detectPartials.
	EarlyDetectionStartSecs float64 `json:"earlyDetectionStartSecs"`
	EarlyDetectionEndSecs   float64 `json:"earlyDetectionEndSecs"`
	FFTSize                 int     `json:"fftSize"`

	// Per-partial decay fitting.
	DecayFitStartSeconds float64 `json:"decayFitStartSeconds"`
	DecayFitEndSeconds   float64 `json:"decayFitEndSeconds"`
	DecayFitFloorDB      float64 `json:"decayFitFloorDB"` // stop fitting below this

	// Glide: instantaneous frequency of the fundamental, early versus late.
	GlideEarlySeconds float64 `json:"glideEarlySeconds"`
	// GlideLateSeconds is the *latest* the second probe may sit, not where it
	// necessarily lands: the probe is walked back from here to the last point
	// the tracked partial still supports a reading. See measureGlide.
	GlideLateSeconds float64 `json:"glideLateSeconds"`
	// GlideMinSpanSeconds is the shortest early-to-late span still worth
	// calling a glide. Below it the measurement is refused outright.
	GlideMinSpanSeconds float64 `json:"glideMinSpanSeconds"`
	// GlideFloorDB is how far the tracked partial's baseband envelope may fall
	// below its level at the early probe before the late probe is treated as
	// unsupported. See measureGlide for why this bound is the whole fix.
	GlideFloorDB float64 `json:"glideFloorDB"`
	// GlidePartialWindowDB is how far below the loudest partial the glide may
	// still be read: the lowest partial within this window is the fundamental.
	GlidePartialWindowDB float64 `json:"glidePartialWindowDB"`

	// Windowed spectra.
	Windows        []TimeWindow `json:"windows"`
	WindowFFTSize  int          `json:"windowFFTSize"`
	BandMinHz      float64      `json:"bandMinHz"`
	BandMaxHz      float64      `json:"bandMaxHz"`
	BandsPerOctave int          `json:"bandsPerOctave"`

	// Amplitude envelope.
	EnvelopeFrameSeconds float64 `json:"envelopeFrameSeconds"`
	EnvelopeHopSeconds   float64 `json:"envelopeHopSeconds"`
	EnvelopeFloorDB      float64 `json:"envelopeFloorDB"`

	// Attack balance: the two bands whose ratio names "click" against "body".
	AttackWindowSeconds float64 `json:"attackWindowSeconds"`
	AttackHighMinHz     float64 `json:"attackHighMinHz"`
	AttackHighMaxHz     float64 `json:"attackHighMaxHz"`
	AttackLowMinHz      float64 `json:"attackLowMinHz"`
	AttackLowMaxHz      float64 `json:"attackLowMaxHz"`

	// Diagnostics turns on Features.Waveform and Features.Decay, which are
	// reported by cmd/measure-tom and read by nothing that scores anything.
	//
	// They are off by default because they are not free: between them they walk
	// the whole hit twice more per extraction — a fourth-order Welford update
	// over every sample, and a full Schroeder integration — for numbers
	// match.Distance never looks at. A fit run pays that once per take per
	// candidate, millions of times.
	Diagnostics bool `json:"diagnostics,omitempty"`
}

// DefaultOptions is the measurement this repository's tom work is calibrated
// against. Every number is a judgement about what a tom is, so each is
// explained where it is not obvious.
func DefaultOptions() Options {
	return Options{
		// Long enough to contain the decay of the drum being measured, which is
		// the only thing that makes a ring time evidence rather than an
		// extrapolation.
		//
		// This was 1.2 s until PLAN N17, on the reasoning that a tom is 40 dB
		// down inside a second and past that a close-microphone recording is
		// mostly room. The first half of that is a statement about a particular
		// drum and it did not survive the reference moving: on
		// reference/tt08x08/lp/hd the fundamental rings 0.686 s and the longest
		// partial 2.5 s, and at the old window the low band's ring times moved
		// 18.9 % when it was widened by a third. Re-measured across window ends
		// — partial-matched by frequency within 0.5 %, median |log2 T60 ratio|
		// below 1 kHz — the movement falls 18.9 % -> 8.1 % -> 4.1 % -> 1.4 % at
		// window ends 0.60, 0.90, 1.20, 1.60, 1.90 s. 1.60 is where it converges;
		// the numbers below are that, plus the room the fit needs on either side.
		//
		// The room-tail concern is real and is now answered where it belongs.
		// It is not the analysis span's job to bound the estimator's credulity
		// by staying too short to see anything: slowestSupportedT60 rejects a
		// decay this window cannot support, whatever its length, and
		// DecayFitFloorDB truncates the trace before a floor can flatten it.
		AnalysisSeconds: 2.0,

		MaxPartials:    16,
		MinFrequencyHz: 60,
		// Above ~3 kHz a 12-inch head has hundreds of unresolved modes; that
		// band is the stochastic attack layer's job and is measured by band
		// energy, not by partial.
		MaxFrequencyHz:   3000,
		PartialFloorDB:   -42,
		PeakProminenceDB: 6,
		MinSeparationHz:  15,
		// Detection runs on the sustain, after the strike transient has gone
		// and while the modes still stand above the noise.
		SustainStartSecs: 0.05,
		SustainEndSecs:   0.85,
		// Starts with the sustain — after the strike transient, whose broadband
		// click would otherwise offer a peak at every frequency — and closes
		// while a partial ringing for a tenth of a second is still standing.
		EarlyDetectionStartSecs: 0.05,
		EarlyDetectionEndSecs:   0.30,
		FFTSize:                 1 << 16,

		// The fit starts after the contact pulse and the glide have settled and
		// runs until the ring times it reports stop moving — 1.60 s, per the
		// convergence measured in AnalysisSeconds above. It ended at 0.60 s
		// before PLAN N17, which was shorter than 30 % of this reference's own
		// partials and rendered the longest of them meaningless.
		DecayFitStartSeconds: 0.05,
		DecayFitEndSeconds:   1.60,
		DecayFitFloorDB:      -45,

		GlideEarlySeconds: 0.030,
		GlideLateSeconds:  0.400,
		// 80 ms from the early probe is past the knee of a tom's bend — the
		// tension transient has a time constant of tens of milliseconds — so a
		// reading this short is still a reading, and anything shorter is not.
		GlideMinSpanSeconds: 0.050,
		// 20 dB, which is a stricter bound than it first looks. What has to
		// hold at the late probe is not that the partial is audible but that
		// it still *dominates* its own passband, and its neighbours decay too.
		// Swept over the model's cavity coupling from the shipped stiffness to
		// a rigid shell, 20 dB gives a reading that moves smoothly with the
		// coupling; at 30 dB the strongly coupled end starts reading the
		// neighbour again and the trend breaks. The reference is unaffected —
		// its fundamental is only 15 dB down at 0.400 s — so this costs
		// nothing where the partial does survive.
		GlideFloorDB: 20,
		// 20 dB: wide enough to reach past a strongly radiating upper mode to
		// the fundamental below it, narrow enough that a shell resonance or a
		// room mode 40 dB down cannot claim the note.
		GlidePartialWindowDB: 20,

		Windows: []TimeWindow{
			{Name: "attack", StartSeconds: 0, EndSeconds: 0.020},
			{Name: "early", StartSeconds: 0.020, EndSeconds: 0.100},
			{Name: "body", StartSeconds: 0.100, EndSeconds: 0.400},
			// The tail window ends where the analysis does. It has to track
			// AnalysisSeconds rather than hold a number of its own: audio
			// inside the analysis span but past the last window is measured by
			// nothing, so the spectral-envelope term would simply stop looking
			// at 1.2 s while every other term went on to 2.0.
			{Name: "tail", StartSeconds: 0.400, EndSeconds: 2.000},
		},
		WindowFFTSize:  1 << 14,
		BandMinHz:      50,
		BandMaxHz:      12500,
		BandsPerOctave: 3,

		EnvelopeFrameSeconds: 0.020,
		EnvelopeHopSeconds:   0.010,
		EnvelopeFloorDB:      -70,

		AttackWindowSeconds: 0.020,
		AttackHighMinHz:     1000,
		AttackHighMaxHz:     8000,
		AttackLowMinHz:      100,
		AttackLowMaxHz:      500,
	}
}

// Partial is one resolved mode of the hit.
type Partial struct {
	FrequencyHz float64 `json:"frequencyHz"`
	// LevelDB is relative to the strongest partial, so it survives any gain.
	LevelDB float64 `json:"levelDB"`
	// T60Seconds comes from a log-linear fit of the partial's own envelope.
	// Zero means the fit did not converge on a decay.
	T60Seconds float64 `json:"t60Seconds"`
	// FitQuality is that fit's R². Partials whose envelope is not an
	// exponential — beating pairs, buried modes — score low and are weighted
	// down rather than discarded, because their frequency is still evidence.
	//
	// It is reported and it is *not* what the decay term trusts. Measured
	// against subband ESPRIT over the sixteen velocities of the licensed
	// reference, R² does not discriminate at all: median ring-time disagreement
	// is 39 % at R² >= 0.95 and 44 % below it. DecayRangeDB is what replaced it,
	// and unlike R² it was measured to separate the two populations before being
	// given the job. docs/physical-objective-validation.md §5c/§5f.
	FitQuality float64 `json:"fitQuality"`
	// DecayRangeDB is how far the partial fell inside the fit window before the
	// fitted noise floor caught it: the dynamic range the ring time was actually
	// read over. It says how much evidence there is for the number, where R²
	// says only how straight a line was drawn through whatever evidence there
	// was — and a slope through a noise floor is perfectly straight.
	//
	// Decay before the window opens is excluded, because it was not observed,
	// and the whole quantity is capped at the trace's own span, because a
	// partial still above the noise when the window closes leaves the floor
	// unconstrained and the model's 10*log10(P0/N) runs away.
	//
	// Measured, on the same evidence R² was measured on: it does not
	// discriminate on this reference either, for the reason given in
	// docs/physical-objective-validation.md §5f. It is reported, and the decay
	// term does not weight by it.
	DecayRangeDB float64 `json:"decayRangeDB"`
}

// WindowFeature is one time window's spectral shape.
type WindowFeature struct {
	TimeWindow
	// BandDB is the fractional-octave band level, mean-removed across the
	// bands present. Removing the mean is what makes it a *shape*: a level
	// difference between reference and candidate cannot show up here.
	BandDB   []float64            `json:"bandDB"`
	Spectrum frequencystats.Stats `json:"spectrum"`
}

// Features is everything this package measures about one hit.
type Features struct {
	SampleRateHz float64 `json:"sampleRateHz"`
	// OnsetSample is where the analysis was anchored in the source signal.
	OnsetSample int `json:"onsetSample"`

	Partials   []Partial `json:"partials"`
	GlideCents float64   `json:"glideCents"`
	// GlideMeasured reports whether GlideCents is a reading at all. False means
	// the fundamental did not survive far enough past the strike for two
	// probes to be placed on it, and GlideCents is zero because there is no
	// number, not because the pitch held. Distance treats the two differently.
	GlideMeasured bool            `json:"glideMeasured"`
	Windows       []WindowFeature `json:"windows"`
	EnvelopeDB    []float64       `json:"envelopeDB"`
	AttackBalance float64         `json:"attackBalanceDB"`

	Waveform timestats.Stats `json:"waveform"`
	Decay    ir.Metrics      `json:"decay"`

	// BandCentresHz labels Windows[i].BandDB. Same for every window.
	BandCentresHz []float64 `json:"bandCentresHz"`
}

// Extract measures one hit. samples may contain leading silence; the onset is
// found and everything is measured relative to it.
func Extract(samples []float64, sampleRateHz float64, options Options) (Features, error) {
	if err := options.validate(sampleRateHz); err != nil {
		return Features{}, err
	}

	if len(samples) == 0 {
		return Features{}, fmt.Errorf("%w: empty signal", ErrInvalidReference)
	}

	work := acquireExtractScratch()
	defer releaseExtractScratch(work)

	hit, onset, err := onsetAlignedHitInto(work, samples, sampleRateHz, options)
	if err != nil {
		return Features{}, err
	}

	features := Features{
		SampleRateHz: sampleRateHz,
		OnsetSample:  onset,
	}

	// Reporting only, and skipped unless asked for. See Options.Diagnostics.
	if options.Diagnostics {
		features.Waveform = timestats.Calculate(hit)

		if metrics, err := ir.NewAnalyzer(sampleRateHz).Analyze(hit); err == nil {
			features.Decay = metrics
		}
	}

	partials, err := detectPartials(work, hit, sampleRateHz, options)
	if err != nil {
		return Features{}, err
	}

	features.Partials = measureDecays(work, hit, sampleRateHz, options, partials)

	// The glide belongs to the fundamental — see glidePartial for why it is
	// neither simply the lowest partial nor simply the loudest.
	if index := glidePartial(features.Partials, options.GlidePartialWindowDB); index >= 0 {
		features.GlideCents, features.GlideMeasured = measureGlide(work, hit, sampleRateHz, options,
			features.Partials[index].FrequencyHz,
			glideCutoffHz(features.Partials, index))
	}

	centres, edges := cachedFractionalOctaveBands(options.BandMinHz, options.BandMaxHz,
		options.BandsPerOctave, sampleRateHz)
	features.BandCentresHz = centres

	features.Windows, err = measureWindows(work, hit, sampleRateHz, options, edges)
	if err != nil {
		return Features{}, err
	}

	features.EnvelopeDB = measureEnvelope(hit, sampleRateHz, options)

	features.AttackBalance, err = measureAttackBalance(work, hit, sampleRateHz, options)
	if err != nil {
		return Features{}, err
	}

	return features, nil
}

// onsetAlignedHit finds the strike, trims the analysis span to it and
// peak-normalizes, returning the span and where it started in the source.
//
// Shared by both estimators on purpose. Everything they disagree about should
// be the estimate; where the hit begins and how loud it was scaled to are not
// estimates, and a second copy of this would eventually make them one.
func onsetAlignedHit(samples []float64, sampleRateHz float64, options Options) ([]float64, int, error) {
	return onsetAlignedHitInto(nil, samples, sampleRateHz, options)
}

// onsetAlignedHitInto is onsetAlignedHit writing the normalized span into work's
// buffer. A nil scratch allocates, which is what the high-resolution path and
// the tests want; Extract passes its own, because at 2 s and 44.1 kHz this one
// array was the largest single allocation left in an extraction.
func onsetAlignedHitInto(work *extractScratch, samples []float64, sampleRateHz float64, options Options) ([]float64, int, error) {
	if len(samples) == 0 {
		return nil, 0, fmt.Errorf("%w: empty signal", ErrInvalidReference)
	}

	onset, err := ir.NewAnalyzer(sampleRateHz).FindImpulseStart(samples)
	if err != nil || onset < 0 || onset >= len(samples) {
		onset = 0
	}

	span := samples[onset:]
	if limit := int(options.AnalysisSeconds * sampleRateHz); limit > 0 && limit < len(span) {
		span = span[:limit]
	}

	// Peak-normalize once, here, so gain invariance is a property of the
	// measurement rather than something every term has to remember.
	if work == nil {
		return normalizePeak(span), onset, nil
	}

	work.hit = growFloats(work.hit, len(span))

	return normalizePeakInto(work.hit, span), onset, nil
}

func (o Options) validate(sampleRateHz float64) error {
	switch {
	case !(sampleRateHz > 0) || math.IsInf(sampleRateHz, 0):
		return fmt.Errorf("%w: sample rate %v", ErrInvalidOptions, sampleRateHz)
	case o.AnalysisSeconds <= 0:
		return fmt.Errorf("%w: analysis seconds %v", ErrInvalidOptions, o.AnalysisSeconds)
	case o.MaxPartials <= 0:
		return fmt.Errorf("%w: max partials %d", ErrInvalidOptions, o.MaxPartials)
	case !(o.MinFrequencyHz > 0) || o.MaxFrequencyHz <= o.MinFrequencyHz:
		return fmt.Errorf("%w: partial band %v..%v Hz",
			ErrInvalidOptions, o.MinFrequencyHz, o.MaxFrequencyHz)
	case o.FFTSize < 64 || o.FFTSize&(o.FFTSize-1) != 0:
		return fmt.Errorf("%w: fft size %d", ErrInvalidOptions, o.FFTSize)
	case o.WindowFFTSize < 64 || o.WindowFFTSize&(o.WindowFFTSize-1) != 0:
		return fmt.Errorf("%w: window fft size %d", ErrInvalidOptions, o.WindowFFTSize)
	case o.DecayFitEndSeconds <= o.DecayFitStartSeconds:
		return fmt.Errorf("%w: decay fit span %v..%v s",
			ErrInvalidOptions, o.DecayFitStartSeconds, o.DecayFitEndSeconds)
	case o.BandsPerOctave <= 0 || !(o.BandMaxHz > o.BandMinHz) || !(o.BandMinHz > 0):
		return fmt.Errorf("%w: band layout %v..%v Hz at 1/%d octave",
			ErrInvalidOptions, o.BandMinHz, o.BandMaxHz, o.BandsPerOctave)
	case o.GlideEarlySeconds < 0 || o.GlideLateSeconds <= o.GlideEarlySeconds:
		return fmt.Errorf("%w: glide probes at %v..%v s",
			ErrInvalidOptions, o.GlideEarlySeconds, o.GlideLateSeconds)
	case o.GlideMinSpanSeconds <= 0 ||
		o.GlideEarlySeconds+o.GlideMinSpanSeconds > o.GlideLateSeconds:
		return fmt.Errorf("%w: glide minimum span %v s does not fit in %v..%v s",
			ErrInvalidOptions, o.GlideMinSpanSeconds, o.GlideEarlySeconds, o.GlideLateSeconds)
	case o.GlideFloorDB <= 0:
		return fmt.Errorf("%w: glide floor %v dB", ErrInvalidOptions, o.GlideFloorDB)
	case o.GlidePartialWindowDB <= 0:
		return fmt.Errorf("%w: glide partial window %v dB", ErrInvalidOptions, o.GlidePartialWindowDB)
	case o.EnvelopeFrameSeconds <= 0 || o.EnvelopeHopSeconds <= 0:
		return fmt.Errorf("%w: envelope frame %v hop %v",
			ErrInvalidOptions, o.EnvelopeFrameSeconds, o.EnvelopeHopSeconds)
	case len(o.Windows) == 0:
		return fmt.Errorf("%w: no time windows", ErrInvalidOptions)
	}

	return nil
}

func normalizePeak(samples []float64) []float64 {
	return normalizePeakInto(make([]float64, len(samples)), samples)
}

// normalizePeakInto writes the normalized copy into out, which must be exactly
// len(samples). Its previous contents are overwritten in full, including the
// silent case.
func normalizePeakInto(out, samples []float64) []float64 {
	peak := 0.0
	for _, sample := range samples {
		if magnitude := math.Abs(sample); magnitude > peak {
			peak = magnitude
		}
	}

	if peak == 0 {
		clear(out)

		return out
	}

	// One reciprocal and a multiply per sample rather than a divide per sample.
	// Not exactly the same bits — 1/peak rounds once more than the quotient
	// does — but this is a peak normalization ahead of a dB-domain measurement,
	// and half an ulp of scale is common to every sample and so invisible to
	// every term downstream.
	scale := 1 / peak

	out = out[:len(samples)]
	for i, sample := range samples {
		out[i] = sample * scale
	}

	return out
}

// spectralPeak is one prominent interpolated maximum of one transform.
type spectralPeak struct {
	frequency float64
	magnitude float64
}

// spectralPeaks returns the prominent peaks of one segment, strongest first.
func spectralPeaks(work *extractScratch, segment []float64, sampleRateHz float64, options Options) ([]spectralPeak, error) {
	magnitude, err := work.spectrum(segment, options.FFTSize)
	if err != nil {
		return nil, err
	}

	binHz := sampleRateHz / float64(options.FFTSize)
	lowBin := max(1, int(options.MinFrequencyHz/binHz))
	highBin := min(len(magnitude)-2, int(options.MaxFrequencyHz/binHz))

	var peaks []spectralPeak

	for bin := lowBin; bin <= highBin; bin++ {
		left, centre, right := magnitude[bin-1], magnitude[bin], magnitude[bin+1]
		if centre <= left || centre < right || centre <= 0 {
			continue
		}

		if prominenceDB(magnitude, bin) < options.PeakProminenceDB {
			continue
		}

		offset := parabolicOffset(left, centre, right)
		peaks = append(peaks, spectralPeak{
			frequency: (float64(bin) + offset) * binHz,
			magnitude: centre,
		})
	}

	slices.SortFunc(peaks, func(a, b spectralPeak) int {
		switch {
		case a.magnitude > b.magnitude:
			return -1
		case a.magnitude < b.magnitude:
			return 1
		default:
			return 0
		}
	})

	return peaks, nil
}

// detectPartials picks interpolated spectral peaks from two views of the hit.
//
// The sustain transform is the selective one and does most of the work. It is
// also blind in a specific way: it spans 800 ms, so a partial that rings for a
// tenth of that stands roughly 90 dB lower in it than a partial of the same
// strike amplitude that rings throughout. Both detection guards — the relative
// floor and the count — rank on that magnitude, so a loud, short-ringing partial
// was discarded before anything measured it. A synthetic partial at half the
// fundamental's amplitude, ringing 120 ms, was not detected at all;
// TestShortPartialsDoNotOutrankLongOnes is that case.
//
// So a second, earlier and shorter window is read as well, and each window
// admits candidates relative to *its own* strongest peak. A short partial
// competes against the other short-lived content rather than against the
// fundamental's whole ring, which is the comparison it can win. The early window
// starts after the strike transient — a broadband click would otherwise supply a
// peak at every frequency — and is coarser in resolution, which costs a few
// cents on the candidates only it finds; measureDecays heterodynes each partial
// at its own frequency afterwards regardless.
func detectPartials(work *extractScratch, hit []float64, sampleRateHz float64, options Options) ([]Partial, error) {
	sustainStart := clampIndex(int(options.SustainStartSecs*sampleRateHz), len(hit))

	sustainEnd := clampIndex(int(options.SustainEndSecs*sampleRateHz), len(hit))
	if sustainEnd-sustainStart < 64 {
		sustainStart, sustainEnd = 0, len(hit)
	}

	sustainPeaks, err := spectralPeaks(work, hit[sustainStart:sustainEnd], sampleRateHz, options)
	if err != nil {
		return nil, err
	}

	earlyStart := clampIndex(int(options.EarlyDetectionStartSecs*sampleRateHz), len(hit))

	earlyEnd := clampIndex(int(options.EarlyDetectionEndSecs*sampleRateHz), len(hit))
	if earlyEnd-earlyStart < 64 {
		earlyStart, earlyEnd = sustainStart, sustainEnd
	}

	earlyPeaks, err := spectralPeaks(work, hit[earlyStart:earlyEnd], sampleRateHz, options)
	if err != nil {
		return nil, err
	}

	// Detection is deliberately looser than the final floor, in both level and
	// count: the job here is only to not lose anything that will pass then.
	const (
		detectionHeadroomDB = 20
		detectionSurplus    = 2
	)

	var kept []Partial

	admit := func(peaks []spectralPeak) {
		if len(peaks) == 0 {
			return
		}

		floor := peaks[0].magnitude * math.Exp((options.PartialFloorDB-detectionHeadroomDB)*amplitudePerDecibel)
		budget := len(kept) + options.MaxPartials*detectionSurplus

		for _, peak := range peaks {
			if len(kept) >= budget || peak.magnitude < floor {
				break
			}

			// One spectral lobe spans several bins, and the two windows see the
			// same partials; without this the strongest partial would be
			// "detected" three times and crowd out the rest.
			if slices.ContainsFunc(kept, func(known Partial) bool {
				return math.Abs(known.FrequencyHz-peak.frequency) < options.MinSeparationHz
			}) {
				continue
			}

			kept = append(kept, Partial{FrequencyHz: peak.frequency})
		}
	}

	// Sustain first, so where both windows see a partial the better-resolved
	// frequency is the one kept.
	admit(sustainPeaks)
	admit(earlyPeaks)

	slices.SortFunc(kept, func(a, b Partial) int {
		switch {
		case a.FrequencyHz < b.FrequencyHz:
			return -1
		case a.FrequencyHz > b.FrequencyHz:
			return 1
		default:
			return 0
		}
	})

	return kept, nil
}

// prominenceDB is how far a peak stands above the higher of the two valleys
// separating it from a taller peak — the standard topographic definition.
//
// A bare local-maximum test accepts every ripple on the skirt of a strong
// partial. On the reference tom that produced a phantom mode at 87 Hz, 39 dB
// down on the shoulder of the fundamental, which a fit would then have to
// invent a matching mode for.
func prominenceDB(magnitude []float64, peak int) float64 {
	walk := func(step int) float64 {
		valley := magnitude[peak]

		for bin := peak + step; bin > 0 && bin < len(magnitude)-1; bin += step {
			if magnitude[bin] > magnitude[peak] {
				break
			}

			valley = min(valley, magnitude[bin])
		}

		return valley
	}

	valley := max(walk(-1), walk(1))
	if valley <= 0 {
		return math.Inf(1)
	}

	return decibelsPerAmplitude * math.Log(magnitude[peak]/valley)
}

// parabolicOffset interpolates a peak's sub-bin position from its neighbours,
// in the log domain because a windowed magnitude peak is closer to a parabola
// in dB than in linear amplitude.
func parabolicOffset(left, centre, right float64) float64 {
	const floor = 1e-30

	logLeft := math.Log(max(left, floor))
	logCentre := math.Log(max(centre, floor))
	logRight := math.Log(max(right, floor))

	denominator := logLeft - 2*logCentre + logRight
	if denominator == 0 {
		return 0
	}

	offset := 0.5 * (logLeft - logRight) / denominator
	if math.Abs(offset) > 0.5 || math.IsNaN(offset) {
		return 0
	}

	return offset
}

// phasorAnchorInterval is how many samples the heterodyne phasor advances by
// recurrence before it is re-derived from math.Sincos.
//
// Rotating a unit phasor by a constant is one complex multiply per sample
// instead of a sine and a cosine, and it was the single most expensive thing
// left in a fit — the old form also handed Sincos an argument that grew to tens
// of thousands of radians, which forces the slow exact argument reduction. But
// the rotor's modulus is not exactly one in floating point, so a free-running
// phasor's amplitude creeps and its phase shears, compounding over the tens of
// thousands of samples a partial is measured across.
//
// Re-anchoring bounds that: the error inside a block grows like the block
// length times the rounding unit and is then discarded, so it never
// accumulates across the signal. At 512 the worst case is a few parts in
// 10^13 — far below the reference's own precision — while 511 of every 512
// Sincos calls still disappear.
//
// A variable only so that the accuracy test can set it to 1, which re-derives
// every sample and so reproduces the exact per-sample Sincos form this
// replaced. Nothing outside tests writes it.
var phasorAnchorInterval = 512

// heterodyne shifts one partial to DC and low-passes it, returning the complex
// baseband envelope.
//
// This, rather than a bandpass filter bank, because the selectivity is then set
// by one cutoff in hertz that can be chosen from the *measured* spacing to the
// nearest neighbouring partial — a bandpass would have to express the same
// thing as a Q that changes meaning with centre frequency.
// sections chooses selectivity against settling time: 2 gives a fourth-order
// Butterworth (24 dB down an octave past the cutoff), 1 a second-order one
// whose group delay is short enough to read a pitch 30 ms after the strike.
func heterodyne(hit []float64, sampleRateHz, frequencyHz, cutoffHz float64, sections int) (inPhase, quadrature []float64) {
	inPhase = make([]float64, len(hit))
	quadrature = make([]float64, len(hit))

	heterodyneInto(inPhase, quadrature, hit, sampleRateHz, frequencyHz, cutoffHz, sections)

	return inPhase, quadrature
}

// The two cascades heterodyne offers, at package scope so that the choice
// between them costs no allocation. A slice literal here ran once per detected
// partial per extraction.
var (
	fourthOrderResonances = []float64{0.5411961, 1.3065630}
	secondOrderResonances = []float64{math.Sqrt2 / 2}
)

// heterodyneInto is heterodyne writing into buffers the caller owns. Both must
// be exactly len(hit); their previous contents are overwritten in full.
func heterodyneInto(inPhase, quadrature, hit []float64, sampleRateHz, frequencyHz, cutoffHz float64, sections int) {
	resonances := fourthOrderResonances
	if sections == 1 {
		resonances = secondOrderResonances
	}

	step := -2 * math.Pi * frequencyHz / sampleRateHz
	stepSine, stepCosine := math.Sincos(step)

	// The phasor advances by one complex multiply per sample rather than a
	// Sincos per sample, and is re-derived exactly every phasorAnchorInterval
	// samples. See phasorAnchorInterval for why it cannot simply run free.
	for start := 0; start < len(hit); start += phasorAnchorInterval {
		end := min(start+phasorAnchorInterval, len(hit))
		sine, cosine := math.Sincos(step * float64(start))

		// Blocked through subslices of one length so the two writes carry no
		// bounds check; the arithmetic is unchanged.
		source := hit[start:end]
		cosineOut, sineOut := inPhase[start:end], quadrature[start:end]

		for n, sample := range source {
			cosineOut[n], sineOut[n] = sample*cosine, sample*sine

			cosine, sine = cosine*stepCosine-sine*stepSine,
				sine*stepCosine+cosine*stepSine
		}
	}

	// Zero phase, by filtering forwards and then backwards. The filter is real
	// and symmetric in ±frequency, so applying it identically to the two
	// quadratures is exactly zero-phase filtering of the complex baseband.
	//
	// It matters twice: a causal filter's group delay would bias the pitch
	// probe that reads the glide, and its settling transient would sit right
	// on top of the first tens of milliseconds a decay is fitted over.
	zeroPhaseLowpassPair(inPhase, quadrature, resonances, cutoffHz, sampleRateHz)
}

// zeroPhaseLowpassPair filters two signals of equal length through the same
// cascade, forwards and then backwards, in place.
//
// Both quadratures in one loop rather than a call each, because a biquad is a
// serial recurrence: every sample waits on the state the previous sample left
// behind, so a lone cascade spends much of its time waiting on a multiply it
// cannot start yet. The two signals are entirely independent of each other, and
// interleaving them fills those gaps — identical arithmetic on each signal,
// sample for sample.
//
// This filtering was the most expensive thing in a fit, so the shape of the
// loop below is measured rather than assumed; the pairing is worth about 1.35×.
// TestZeroPhaseLowpassPairMatchesPerSignalReference holds it to the plain
// per-signal form bit for bit, which is the whole licence for writing it this
// way.
func zeroPhaseLowpassPair(first, second, resonances []float64, cutoffHz, sampleRateHz float64) {
	// Backed by a stack array rather than a heap slice: the cascade is two
	// sections and this ran once per partial per extraction. append still grows
	// correctly if a caller ever asks for a longer one.
	var storage [4]biquad.Coefficients

	coefficients := storage[:0]
	for _, resonance := range resonances {
		coefficients = append(coefficients, design.Lowpass(cutoffHz, resonance, sampleRateHz))
	}

	// One section over the whole signal before the next, rather than one sample
	// through the whole cascade before the next: a section is a pure function of
	// its input stream, so the two orders produce the same numbers sample for
	// sample, but this one lets the coefficients and the two-word state live in
	// registers for the length of a pass instead of being reloaded per sample.
	//
	// The backward pass walks from the end rather than reversing the signal,
	// filtering, and reversing it back — four sweeps of the pair that bought
	// nothing.
	//
	// The recurrence is spelled out rather than calling biquad.Section because
	// neither of those two things is expressible through it: it processes one
	// signal, forwards. The form below is its ProcessSample, unchanged.
	// One section over the whole signal before the next, rather than one sample
	// through the whole cascade before the next: a section is a pure function of
	// its input stream, so the two orders produce the same numbers sample for
	// sample, but this one lets the coefficients and the two-word state live in
	// registers for the length of a pass instead of being reloaded per sample.
	//
	// The backward pass walks from the end rather than reversing the signal,
	// filtering, and reversing it back — four sweeps of the pair that bought
	// nothing.
	//
	// The recurrence is spelled out rather than calling biquad.Section because
	// neither of those two things is expressible through it: it processes one
	// signal, forwards. The form below is its ProcessSample, unchanged.
	//
	// The strided `index != to` form costs four bounds checks a sample, because
	// nothing can be proved about an index moving by a variable stride, and
	// splitting it into separate forward and backward loops removes all four —
	// confirmed in the SSA. It was tried, and it is not written that way,
	// because it does not measure faster: 25 interleaved pairs of runs put the
	// split form at +3.3 % on the minimum and -7.8 % on the median, which is
	// noise on either side of nothing. A biquad is a serial recurrence and every
	// sample stalls on the multiply the previous one has not finished; the
	// bounds checks were being issued into that stall and cost nothing to
	// remove. Two copies of the body for no measured gain is a worse file.
	run := func(from, to, stride int) {
		for _, section := range coefficients {
			var firstD0, firstD1, secondD0, secondD1 float64

			for index := from; index != to; index += stride {
				sample := first[index]
				filtered := section.B0*sample + firstD0
				firstD0 = section.B1*sample - section.A1*filtered + firstD1
				firstD1 = section.B2*sample - section.A2*filtered
				first[index] = filtered

				sample = second[index]
				filtered = section.B0*sample + secondD0
				secondD0 = section.B1*sample - section.A1*filtered + secondD1
				secondD1 = section.B2*sample - section.A2*filtered
				second[index] = filtered
			}
		}
	}

	run(0, len(first), 1)
	run(len(first)-1, -1, -1)
}

// neighbourSpacing is the distance to the closest other detected partial, which
// sets how selective that partial's envelope filter has to be.
func neighbourSpacing(partials []Partial, index int) float64 {
	spacing := math.Inf(1)

	for other := range partials {
		if other == index {
			continue
		}

		if distance := math.Abs(partials[other].FrequencyHz - partials[index].FrequencyHz); distance < spacing {
			spacing = distance
		}
	}

	return spacing
}

func loudestPartial(partials []Partial) int {
	loudest := -1

	for index := range partials {
		if loudest < 0 || partials[index].LevelDB > partials[loudest].LevelDB {
			loudest = index
		}
	}

	return loudest
}

// glidePartial picks the partial the pitch bend is read off: the lowest one
// standing within windowDB of the loudest.
//
// Neither of the two obvious choices works. The *lowest* partial outright is
// wrong because the lowest peak in the band may be a shell resonance or a room
// mode 40 dB down, and the bend belongs to the mode that carries the note.
//
// The *loudest* partial outright is what this used to be, and it is wrong for a
// reason that took a sweep to see. On this repository's tom reference the
// loudest partial is the 212.7 Hz mode, which is 0.16 s of T60 and gone by the
// time the late probe fires; the 118.05 Hz fundamental beneath it is 7.7 dB
// quieter and rings for 1.5 s. The measurement was being taken on whichever
// mode happened to peak highest rather than on the one that still exists to be
// measured, and it read the noise floor the loud mode decayed into.
//
// Lowest-within-a-window keeps the guard against a 40 dB-down room mode while
// preferring the fundamental, which is both what "the pitch of the note" means
// and, on a drum, the longest-lived thing in the recording.
func glidePartial(partials []Partial, windowDB float64) int {
	loudest := loudestPartial(partials)
	if loudest < 0 {
		return -1
	}

	// Partials are ordered by frequency, so the first one inside the window is
	// the lowest one inside it.
	for index := range partials {
		if partials[index].LevelDB >= partials[loudest].LevelDB-windowDB {
			return index
		}
	}

	return loudest
}

// glideCutoffHz picks the baseband width the pitch probe sees.
//
// Wide enough to follow a bend of a semitone or more, but narrow enough to
// exclude the neighbouring partial: on this reference the coupled (0,1) pair
// sits 21.6 Hz apart, and a cutoff that admits both reads their beat as a
// 96-cent bend where the true one is about 70.
func glideCutoffHz(partials []Partial, index int) float64 {
	return clampFloat(0.45*neighbourSpacing(partials, index), 10, 60)
}

// linearFit returns the least-squares slope of levels against times, the level
// that line reaches at time zero, and the fit's R².
func linearFit(times, levels []float64) (slope, intercept, rSquared float64) {
	// Reslicing to a common length once drops the bounds check from both
	// sweeps, which measureDecays runs over tens of thousands of trace points
	// per partial.
	length := min(len(times), len(levels))
	times, levels = times[:length], levels[:length]

	count := float64(length)

	var sumTimes, sumLevels float64

	for i, elapsed := range times {
		sumTimes += elapsed
		sumLevels += levels[i]
	}

	meanTime, meanLevel := sumTimes/count, sumLevels/count

	var covariance, varianceTime, varianceLevel float64

	for i, elapsed := range times {
		deltaTime, deltaLevel := elapsed-meanTime, levels[i]-meanLevel
		covariance += deltaTime * deltaLevel
		varianceTime += deltaTime * deltaTime
		varianceLevel += deltaLevel * deltaLevel
	}

	if varianceTime == 0 {
		return 0, 0, 0
	}

	slope = covariance / varianceTime
	intercept = meanLevel - slope*meanTime

	if varianceLevel > 0 {
		rSquared = covariance * covariance / (varianceTime * varianceLevel)
	}

	return slope, intercept, rSquared
}

// The half-widths of the two pitch probes. They differ, and the asymmetry is
// the point.
//
// A residual neighbour leaking past the probe filter does not bias the phase
// slope so much as make it swing, at the beat rate between the two. Averaging
// over the window suppresses that swing in proportion to the window's length,
// so a wide probe is a far more accurate one — but only where there is nothing
// to smear.
//
// At the early probe there is: the bend is steepest there, and a wide window
// would average the pitch across the very thing being measured. It stays at
// ±5 ms, a couple of periods of a tom's fundamental.
//
// At the late probe there is not. The bend has settled by construction —
// GlideMinSpanSeconds exists to guarantee it — so widening costs nothing and
// buys a factor of four against exactly the interference that produced the
// readings this measure was rebuilt to stop producing.
const (
	glideEarlyHalfSeconds = 0.005
	glideLateHalfSeconds  = 0.020
)

// glideProbe is one reading of the heterodyned partial: how far its
// instantaneous frequency sits from the carrier, and how much of it is left.
type glideProbe struct {
	deviationHz float64
	amplitude   float64
}

// probeGlide averages the baseband phase increment and magnitude over one
// window. The heterodyne put the steady partial at DC, so the residual phase
// slope *is* the deviation from the carrier.
func probeGlide(inPhase, quadrature []float64, sampleRateHz, atSeconds, halfSeconds float64) (glideProbe, bool) {
	centre := int(atSeconds * sampleRateHz)
	half := int(halfSeconds * sampleRateHz)

	start, end := centre-half, centre+half
	if start < 1 || end >= len(inPhase) {
		return glideProbe{}, false
	}

	var (
		phase, magnitude float64
		count            int
	)

	// The per-sample phase step is read off z[n] * conj(z[n-1]) rather than by
	// differencing two absolute phases.
	//
	//	z[n] conj(z[n-1]) = (i[n]i[n-1] + q[n]q[n-1]) + j(q[n]i[n-1] - i[n]q[n-1])
	//
	// so its argument *is* phi[n] - phi[n-1], already in (-pi, pi] — which is what
	// the explicit +3pi / Mod / -pi dance was reconstructing by hand.
	//
	// Three things follow. One atan2 a sample instead of two, and the two were
	// the same call: previous at n is current at n-1, recomputed. No math.Mod,
	// which was costing more than the arctangent it was correcting. And better
	// conditioning for the small steps this actually measures — a glide is a
	// fraction of a radian a sample, and taking it as the difference of two
	// angles near +/-pi cancels most of the significand, whereas the cross and
	// dot products carry it directly.
	// The four streams the body reads are taken as subslices of one length —
	// current and one-sample-delayed, in phase and quadrature — which is what
	// lets the compiler drop the bounds checks on a loop the late-probe walk
	// runs several hundred thousand times per extraction.
	currentInPhase, currentQuadrature := inPhase[start:end], quadrature[start:end]
	delayedInPhase, delayedQuadrature := inPhase[start-1:end-1], quadrature[start-1:end-1]

	for n, nowInPhase := range currentInPhase {
		nowQuadrature := currentQuadrature[n]
		wasInPhase, wasQuadrature := delayedInPhase[n], delayedQuadrature[n]

		cross := nowQuadrature*wasInPhase - nowInPhase*wasQuadrature
		dot := nowInPhase*wasInPhase + nowQuadrature*wasQuadrature

		phase += math.Atan2(cross, dot)
		// math.Hypot rather than this was guarding against an overflow that
		// cannot happen: these are peak-normalized samples through a lowpass, so
		// neither square can leave the range of a float64. Hypot costs a branch
		// tree and a division per call, and this loop is the hottest caller in
		// the package.
		magnitude += math.Sqrt(nowInPhase*nowInPhase + nowQuadrature*nowQuadrature)
		count++
	}

	if count == 0 {
		return glideProbe{}, false
	}

	return glideProbe{
		deviationHz: phase / float64(count) * sampleRateHz / (2 * math.Pi),
		amplitude:   magnitude / float64(count),
	}, true
}

// measureGlide reports how far the fundamental falls, in cents, between the
// early probe and the latest late probe the partial still supports. The second
// return is false when it supports none, and then there is no reading at all.
//
// Every published tom analysis treats the downward glide as the characteristic
// feature, and in this model it is the one observable that NLIN moves and
// nothing else does.
//
// # Why the late probe moves
//
// An instantaneous-frequency reading is only a reading while the partial being
// tracked still dominates its own passband. Once it has decayed away, what is
// left inside the probe filter is whatever leaked in from the neighbouring
// partials and the noise floor, and the phase slope then reports *their* offset
// from the carrier — confidently, and with no outward sign that anything is
// wrong.
//
// This is not a corner case; it was the normal case. With the late probe nailed
// to 0.400 s:
//
//   - On the model's own renders the (0,1) fundamental has a T60 of 0.21 s, so
//     by 0.400 s it is 105 dB below its early level. The probe read the nearest
//     long-lived neighbour instead. As cavity coupling separates the doublet
//     that neighbour moves further from the carrier, and the reported "glide"
//     grew with it — −13 cents at the shipped stiffness, −717 cents at 0.30,
//     −625 cents at a rigid cavity. Those are not downglides; they are the
//     offset to a different mode.
//   - On the tom reference the same thing happened to the 212.7 Hz mode the
//     measurement used to track (see glidePartial), whose 0.16 s T60 puts it in
//     the room's noise floor well before 0.400 s.
//
// So the probe is walked back from GlideLateSeconds to the last point at which
// the partial is still within GlideFloorDB of its early level, and the reading
// is refused if that point is not at least GlideMinSpanSeconds after the early
// probe. Both probes must also land inside the filter's own passband: a
// deviation larger than the cutoff cannot be this partial, because the filter
// that produced the signal would have removed it.
//
// A short honest span beats a long dishonest one. The bend is an exponential
// settling with a time constant of tens of milliseconds, so a reading taken at
// 0.10 s has already seen nearly all of it, while a reading taken at 0.400 s on
// a dead partial has seen none of it.
func measureGlide(work *extractScratch, hit []float64, sampleRateHz float64, options Options, frequencyHz, cutoffHz float64) (float64, bool) {
	inPhase, quadrature := work.basebandPair(len(hit))
	heterodyneInto(inPhase, quadrature, hit, sampleRateHz, frequencyHz, cutoffHz, 1)

	early, ok := probeGlide(inPhase, quadrature, sampleRateHz,
		options.GlideEarlySeconds, glideEarlyHalfSeconds)
	if !ok || math.Abs(early.deviationHz) > cutoffHz {
		return 0, false
	}

	floor := early.amplitude * math.Exp(-options.GlideFloorDB*amplitudePerDecibel)
	earliestLate := options.GlideEarlySeconds + options.GlideMinSpanSeconds

	// A millisecond, in whole samples. Fine enough that the late probe moves
	// smoothly rather than in audible steps as a candidate's decay changes
	// under the fit, and coarse enough that the walk costs nothing beside the
	// heterodyne above.
	step := math.Round(0.001*sampleRateHz) / sampleRateHz

	for at := options.GlideLateSeconds; at >= earliestLate; at -= step {
		late, ok := probeGlide(inPhase, quadrature, sampleRateHz, at, glideLateHalfSeconds)
		if !ok || late.amplitude < floor || math.Abs(late.deviationHz) > cutoffHz {
			continue
		}

		earlyHz, lateHz := frequencyHz+early.deviationHz, frequencyHz+late.deviationHz
		if earlyHz <= 0 || lateHz <= 0 {
			return 0, false
		}

		// Positive means the pitch fell, which is the direction a drum bends.
		return 1200 * math.Log2(earlyHz/lateHz), true
	}

	return 0, false
}

// fractionalOctaveBands returns band centres and their edges, clipped to
// Nyquist.
func fractionalOctaveBands(minHz, maxHz float64, perOctave int, sampleRateHz float64) (centres []float64, edges [][2]float64) {
	ratio := math.Pow(2, 1/float64(perOctave))
	half := math.Sqrt(ratio)
	nyquist := sampleRateHz / 2

	for centre := minHz; centre <= maxHz; centre *= ratio {
		low, high := centre/half, centre*half
		if high > nyquist {
			break
		}

		centres = append(centres, centre)
		edges = append(edges, [2]float64{low, high})
	}

	return centres, edges
}

// bandLevelsDB reduces a magnitude spectrum to fractional-octave band levels,
// then removes their mean so what remains is a spectral *shape*.
func bandLevelsDB(magnitude []float64, binHz float64, edges [][2]float64) []float64 {
	levels := make([]float64, len(edges))

	var (
		sum     float64
		counted int
	)

	scale := 1 / binHz

	for band, edge := range edges {
		lowBin := max(0, int(math.Ceil(edge[0]*scale)))
		highBin := min(len(magnitude)-1, int(math.Floor(edge[1]*scale)))

		power := 0.0

		if lowBin <= highBin {
			for _, bin := range magnitude[lowBin : highBin+1] {
				power += bin * bin
			}
		}

		levels[band] = decibelsPerPower * math.Log(power+1e-30)
		sum += levels[band]
		counted++
	}

	if counted > 0 {
		mean := sum / float64(counted)
		for band := range levels {
			levels[band] -= mean
		}
	}

	return levels
}

func measureWindows(work *extractScratch, hit []float64, sampleRateHz float64, options Options, edges [][2]float64) ([]WindowFeature, error) {
	features := make([]WindowFeature, 0, len(options.Windows))

	for _, span := range options.Windows {
		start := clampIndex(int(span.StartSeconds*sampleRateHz), len(hit))

		end := clampIndex(int(span.EndSeconds*sampleRateHz), len(hit))
		if end-start < 16 {
			continue
		}

		magnitude, err := work.spectrum(hit[start:end], options.WindowFFTSize)
		if err != nil {
			return nil, err
		}

		binHz := sampleRateHz / float64(options.WindowFFTSize)

		features = append(features, WindowFeature{
			TimeWindow: span,
			BandDB:     bandLevelsDB(magnitude, binHz, edges),
			Spectrum:   frequencystats.Calculate(magnitude, sampleRateHz),
		})
	}

	if len(features) == 0 {
		return nil, fmt.Errorf("%w: every window fell outside the signal", ErrInvalidOptions)
	}

	return features, nil
}

// measureEnvelope is the frame RMS in dB, referred to the loudest frame.
func measureEnvelope(hit []float64, sampleRateHz float64, options Options) []float64 {
	frame := max(1, int(options.EnvelopeFrameSeconds*sampleRateHz))
	hop := max(1, int(options.EnvelopeHopSeconds*sampleRateHz))

	if len(hit) < frame {
		return nil
	}

	// The frame count is closed form, so the slice is sized once instead of
	// being grown a hundred times.
	levels := make([]float64, 0, (len(hit)-frame)/hop+1)
	peak := 0.0

	for start := 0; start+frame <= len(hit); start += hop {
		// timestats.Calculate is a fourth-order Welford update over every
		// sample of every frame — skewness, kurtosis, zero crossings — and one
		// field of it is read. The RMS it reports is exactly sqrt(sum(x^2)/n)
		// accumulated in this order, so this is the same number.
		sumSquares := 0.0
		for _, sample := range hit[start : start+frame] {
			sumSquares += sample * sample
		}

		rms := math.Sqrt(sumSquares / float64(frame))

		levels = append(levels, rms)

		if rms > peak {
			peak = rms
		}
	}

	if peak <= 0 {
		return nil
	}

	scale := 1 / peak
	for i, rms := range levels {
		levels[i] = max(decibelsPerAmplitude*math.Log(rms*scale+1e-30), options.EnvelopeFloorDB)
	}

	return levels
}

// measureAttackBalance is the click-to-body ratio of the strike, in dB.
//
// It names in one number what the hybrid attack layer exists to supply, and it
// is the term ATK.L and ATK.T move most directly.
func measureAttackBalance(work *extractScratch, hit []float64, sampleRateHz float64, options Options) (float64, error) {
	end := clampIndex(int(options.AttackWindowSeconds*sampleRateHz), len(hit))
	if end < 16 {
		return 0, nil
	}

	magnitude, err := work.spectrum(hit[:end], options.WindowFFTSize)
	if err != nil {
		return 0, err
	}

	binHz := sampleRateHz / float64(options.WindowFFTSize)

	scale := 1 / binHz

	bandPower := func(lowHz, highHz float64) float64 {
		lowBin := max(0, int(math.Ceil(lowHz*scale)))
		highBin := min(len(magnitude)-1, int(math.Floor(highHz*scale)))

		power := 0.0
		if lowBin > highBin {
			return power
		}

		for _, bin := range magnitude[lowBin : highBin+1] {
			power += bin * bin
		}

		return power
	}

	high := bandPower(options.AttackHighMinHz, options.AttackHighMaxHz)
	low := bandPower(options.AttackLowMinHz, options.AttackLowMaxHz)

	return decibelsPerPower * math.Log((high+1e-30)/(low+1e-30)), nil
}

func clampIndex(value, length int) int {
	return min(max(value, 0), length)
}

func clampFloat(value, low, high float64) float64 {
	return min(max(value, low), high)
}
