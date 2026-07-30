package match

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/cwbudde/algo-dsp/dsp/filter/biquad"
	"github.com/cwbudde/algo-dsp/dsp/filter/design"
	"github.com/cwbudde/algo-dsp/dsp/spectrum"
	"github.com/cwbudde/algo-dsp/dsp/window"
	"github.com/cwbudde/algo-dsp/measure/ir"
	frequencystats "github.com/cwbudde/algo-dsp/stats/frequency"
	timestats "github.com/cwbudde/algo-dsp/stats/time"
	algofft "github.com/cwbudde/algo-fft"
)

// ErrInvalidOptions reports feature options that cannot describe an analysis.
var ErrInvalidOptions = errors.New("invalid match options")

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
	FFTSize          int     `json:"fftSize"`

	// Per-partial decay fitting.
	DecayFitStartSeconds float64 `json:"decayFitStartSeconds"`
	DecayFitEndSeconds   float64 `json:"decayFitEndSeconds"`
	DecayFitFloorDB      float64 `json:"decayFitFloorDB"` // stop fitting below this

	// Glide: instantaneous frequency of the lowest partial, early versus late.
	GlideEarlySeconds float64 `json:"glideEarlySeconds"`
	GlideLateSeconds  float64 `json:"glideLateSeconds"`

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
}

// DefaultOptions is the measurement this repository's tom work is calibrated
// against. Every number is a judgement about what a tom is, so each is
// explained where it is not obvious.
func DefaultOptions() Options {
	return Options{
		// A tom is 40 dB down inside a second. Past that a close-microphone
		// recording is mostly room, which the model does not have and should
		// not be fitted to.
		AnalysisSeconds: 1.2,

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
		FFTSize:          1 << 16,

		// The fit starts after the contact pulse and the glide have settled and
		// stops where a room tail would start to dominate a real recording.
		DecayFitStartSeconds: 0.05,
		DecayFitEndSeconds:   0.60,
		DecayFitFloorDB:      -45,

		GlideEarlySeconds: 0.030,
		GlideLateSeconds:  0.400,

		Windows: []TimeWindow{
			{Name: "attack", StartSeconds: 0, EndSeconds: 0.020},
			{Name: "early", StartSeconds: 0.020, EndSeconds: 0.100},
			{Name: "body", StartSeconds: 0.100, EndSeconds: 0.400},
			{Name: "tail", StartSeconds: 0.400, EndSeconds: 1.200},
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
	FitQuality float64 `json:"fitQuality"`
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

	Partials      []Partial       `json:"partials"`
	GlideCents    float64         `json:"glideCents"`
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

	analyzer := ir.NewAnalyzer(sampleRateHz)

	onset, err := analyzer.FindImpulseStart(samples)
	if err != nil || onset < 0 || onset >= len(samples) {
		onset = 0
	}

	span := samples[onset:]
	if limit := int(options.AnalysisSeconds * sampleRateHz); limit > 0 && limit < len(span) {
		span = span[:limit]
	}

	// Peak-normalize once, here, so gain invariance is a property of the
	// measurement rather than something every term has to remember.
	hit := normalizePeak(span)

	features := Features{
		SampleRateHz: sampleRateHz,
		OnsetSample:  onset,
		Waveform:     timestats.Calculate(hit),
	}

	if metrics, err := analyzer.Analyze(hit); err == nil {
		features.Decay = metrics
	}

	partials, magnitudes, sustain, err := detectPartials(hit, sampleRateHz, options)
	if err != nil {
		return Features{}, err
	}

	features.Partials = measureDecays(hit, sampleRateHz, options, partials, magnitudes, sustain)

	// The glide is read off the loudest partial, not the lowest: on a
	// two-headed drum the lowest peak in the band may be a shell resonance or
	// a room mode 40 dB down, and the bend belongs to the mode that carries
	// the note.
	if index := loudestPartial(features.Partials); index >= 0 {
		features.GlideCents = measureGlide(hit, sampleRateHz, options,
			features.Partials[index].FrequencyHz,
			glideCutoffHz(features.Partials, index))
	}

	centres, edges := fractionalOctaveBands(options.BandMinHz, options.BandMaxHz,
		options.BandsPerOctave, sampleRateHz)
	features.BandCentresHz = centres

	features.Windows, err = measureWindows(hit, sampleRateHz, options, edges)
	if err != nil {
		return Features{}, err
	}

	features.EnvelopeDB = measureEnvelope(hit, sampleRateHz, options)

	features.AttackBalance, err = measureAttackBalance(hit, sampleRateHz, options)
	if err != nil {
		return Features{}, err
	}

	return features, nil
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
	case o.EnvelopeFrameSeconds <= 0 || o.EnvelopeHopSeconds <= 0:
		return fmt.Errorf("%w: envelope frame %v hop %v",
			ErrInvalidOptions, o.EnvelopeFrameSeconds, o.EnvelopeHopSeconds)
	case len(o.Windows) == 0:
		return fmt.Errorf("%w: no time windows", ErrInvalidOptions)
	}

	return nil
}

func normalizePeak(samples []float64) []float64 {
	peak := 0.0
	for _, sample := range samples {
		if magnitude := math.Abs(sample); magnitude > peak {
			peak = magnitude
		}
	}

	out := make([]float64, len(samples))
	if peak == 0 {
		return out
	}

	for i, sample := range samples {
		out[i] = sample / peak
	}

	return out
}

// magnitudeSpectrum is a Hann-windowed, zero-padded real FFT magnitude.
func magnitudeSpectrum(segment []float64, fftSize int) ([]float64, error) {
	if len(segment) == 0 {
		return nil, fmt.Errorf("%w: empty segment", ErrInvalidOptions)
	}

	size := min(len(segment), fftSize)
	coefficients := window.Generate(window.TypeHann, size, window.WithPeriodic())

	input := make([]float64, fftSize)
	for i := range size {
		input[i] = segment[i] * coefficients[i]
	}

	plan, err := algofft.NewPlanReal64(fftSize)
	if err != nil {
		return nil, err
	}

	bins := make([]complex128, plan.SpectrumLen())
	if err := plan.Forward(bins, input); err != nil {
		return nil, err
	}

	return spectrum.Magnitude(bins), nil
}

// detectPartials picks interpolated spectral peaks from the sustain.
func detectPartials(hit []float64, sampleRateHz float64, options Options) ([]Partial, []float64, sustainWindow, error) {
	start := clampIndex(int(options.SustainStartSecs*sampleRateHz), len(hit))

	end := clampIndex(int(options.SustainEndSecs*sampleRateHz), len(hit))
	if end-start < 64 {
		start, end = 0, len(hit)
	}

	sustain := sustainWindow{
		startSample:  start,
		length:       min(end-start, options.FFTSize),
		sampleRateHz: sampleRateHz,
	}

	magnitude, err := magnitudeSpectrum(hit[start:end], options.FFTSize)
	if err != nil {
		return nil, nil, sustain, err
	}

	binHz := sampleRateHz / float64(options.FFTSize)
	lowBin := max(1, int(options.MinFrequencyHz/binHz))
	highBin := min(len(magnitude)-2, int(options.MaxFrequencyHz/binHz))

	type candidate struct {
		frequency float64
		magnitude float64
	}

	var candidates []candidate

	for bin := lowBin; bin <= highBin; bin++ {
		left, centre, right := magnitude[bin-1], magnitude[bin], magnitude[bin+1]
		if centre <= left || centre < right || centre <= 0 {
			continue
		}

		if prominenceDB(magnitude, bin) < options.PeakProminenceDB {
			continue
		}

		offset := parabolicOffset(left, centre, right)
		candidates = append(candidates, candidate{
			frequency: (float64(bin) + offset) * binHz,
			magnitude: centre,
		})
	}

	if len(candidates) == 0 {
		return nil, nil, sustain, nil
	}

	slices.SortFunc(candidates, func(a, b candidate) int {
		switch {
		case a.magnitude > b.magnitude:
			return -1
		case a.magnitude < b.magnitude:
			return 1
		default:
			return 0
		}
	})

	// Detection is deliberately looser than the final floor, in both level and
	// count. Its window spans the whole sustain, so a partial that rings for
	// 200 ms is reported far quieter than one that rings for two seconds at the
	// same strike level. measureDecays corrects that once each partial's decay
	// rate is known; here the job is only to not lose anything that will pass
	// then.
	const (
		detectionHeadroomDB = 20
		detectionSurplus    = 2
	)

	floor := candidates[0].magnitude * math.Pow(10, (options.PartialFloorDB-detectionHeadroomDB)/20)
	limit := options.MaxPartials * detectionSurplus

	var (
		kept       []Partial
		magnitudes []float64
	)

	for _, peak := range candidates {
		if len(kept) >= limit || peak.magnitude < floor {
			break
		}

		// One spectral lobe spans several bins; without this the strongest
		// partial would be "detected" three times and crowd out the rest.
		if slices.ContainsFunc(kept, func(known Partial) bool {
			return math.Abs(known.FrequencyHz-peak.frequency) < options.MinSeparationHz
		}) {
			continue
		}

		kept = append(kept, Partial{FrequencyHz: peak.frequency})
		magnitudes = append(magnitudes, peak.magnitude)
	}

	order := make([]int, len(kept))
	for i := range order {
		order[i] = i
	}

	slices.SortFunc(order, func(a, b int) int {
		switch {
		case kept[a].FrequencyHz < kept[b].FrequencyHz:
			return -1
		case kept[a].FrequencyHz > kept[b].FrequencyHz:
			return 1
		default:
			return 0
		}
	})

	sortedPartials := make([]Partial, len(order))
	sortedMagnitudes := make([]float64, len(order))

	for i, from := range order {
		sortedPartials[i], sortedMagnitudes[i] = kept[from], magnitudes[from]
	}

	return sortedPartials, sortedMagnitudes, sustain, nil
}

// sustainWindow records the segment detectPartials measured, so the decay
// correction below can undo exactly the attenuation that window applied.
type sustainWindow struct {
	startSample  int
	length       int
	sampleRateHz float64
}

// decayAttenuation is how much a Hann-windowed transform of this segment
// under-reports the amplitude a partial had at the strike, for a partial
// decaying at the given rate.
//
// A window spanning 50 to 850 ms sees almost all of a partial that rings for
// two seconds and almost none of one that rings for two hundred milliseconds,
// so without this correction the detection magnitudes are a measure of decay
// as much as of level. The integral is exact for an isolated exponential,
// which is what a resolved partial is.
func (w sustainWindow) decayAttenuation(decayPerSecond float64) float64 {
	if w.length <= 0 {
		return 0
	}

	coefficients := window.Generate(window.TypeHann, w.length, window.WithPeriodic())

	sum := 0.0
	for n, coefficient := range coefficients {
		sum += coefficient * math.Exp(-decayPerSecond*float64(n)/w.sampleRateHz)
	}

	return math.Exp(-decayPerSecond*float64(w.startSample)/w.sampleRateHz) * sum
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

	return 20 * math.Log10(magnitude[peak]/valley)
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
	resonances := []float64{0.5411961, 1.3065630}
	if sections == 1 {
		resonances = []float64{math.Sqrt2 / 2}
	}

	inPhase = make([]float64, len(hit))
	quadrature = make([]float64, len(hit))

	step := -2 * math.Pi * frequencyHz / sampleRateHz

	for n, sample := range hit {
		sine, cosine := math.Sincos(step * float64(n))
		inPhase[n], quadrature[n] = sample*cosine, sample*sine
	}

	// Zero phase, by filtering forwards and then backwards. The filter is real
	// and symmetric in ±frequency, so applying it identically to the two
	// quadratures is exactly zero-phase filtering of the complex baseband.
	//
	// It matters twice: a causal filter's group delay would bias the pitch
	// probe that reads the glide, and its settling transient would sit right
	// on top of the first tens of milliseconds a decay is fitted over.
	zeroPhaseLowpass(inPhase, resonances, cutoffHz, sampleRateHz)
	zeroPhaseLowpass(quadrature, resonances, cutoffHz, sampleRateHz)

	return inPhase, quadrature
}

func zeroPhaseLowpass(signal, resonances []float64, cutoffHz, sampleRateHz float64) {
	run := func() {
		sections := make([]biquad.Section, len(resonances))
		for k, resonance := range resonances {
			sections[k] = biquad.Section{Coefficients: design.Lowpass(cutoffHz, resonance, sampleRateHz)}
		}

		for n, sample := range signal {
			for k := range sections {
				sample = sections[k].ProcessSample(sample)
			}

			signal[n] = sample
		}
	}

	run()
	slices.Reverse(signal)
	run()
	slices.Reverse(signal)
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

// glideCutoffHz picks the baseband width the pitch probe sees.
//
// Wide enough to follow a bend of a semitone or more, but narrow enough to
// exclude the neighbouring partial: on this reference the coupled (0,1) pair
// sits 21.6 Hz apart, and a cutoff that admits both reads their beat as a
// 96-cent bend where the true one is about 70.
func glideCutoffHz(partials []Partial, index int) float64 {
	return clampFloat(0.45*neighbourSpacing(partials, index), 10, 60)
}

// measureDecays fits each partial's own decay, and corrects its level for it.
//
// Two measurements, from two different views of the same partial, because
// neither view can do both jobs.
//
// The *rate* comes from the time domain: the signal is heterodyned to
// baseband and low-passed, and the log of the resulting envelope is fitted
// with a straight line.
//
// The *level* comes from the detection spectrum, divided by the attenuation
// that spectrum's own window applied to a partial decaying at the fitted rate.
// It cannot come from the envelope: resolving a pair 10 Hz apart needs a
// filter whose impulse response is longer than 150 ms, and the strike
// transient smeared through one of those put this reference's 212 Hz partial
// 32 dB too loud — above the fundamental. A transform over the sustain is far
// more selective for the same time span, and once the decay rate is known the
// window's effect on the amplitude is an integral, not a guess.
func measureDecays(hit []float64, sampleRateHz float64, options Options, partials []Partial, magnitudes []float64, sustain sustainWindow) []Partial {
	levelLinear := make([]float64, len(partials))

	for index := range partials {
		// Half the distance to the nearest neighbour, bounded: too narrow and
		// the filter's own ring outlasts the partial, too wide and the
		// neighbour's decay is measured instead of this one's.
		cutoff := clampFloat(0.5*neighbourSpacing(partials, index), 10, 40)

		inPhase, quadrature := heterodyne(hit, sampleRateHz, partials[index].FrequencyHz, cutoff, 2)

		start := clampIndex(int(options.DecayFitStartSeconds*sampleRateHz), len(hit))

		end := clampIndex(int(options.DecayFitEndSeconds*sampleRateHz), len(hit))
		if end-start < 16 {
			continue
		}

		peak := 0.0

		for sample := start; sample < end; sample++ {
			if magnitude := math.Hypot(inPhase[sample], quadrature[sample]); magnitude > peak {
				peak = magnitude
			}
		}

		if peak <= 0 {
			continue
		}

		times := make([]float64, 0, end-start)
		trace := make([]float64, 0, end-start)
		floor := peak * math.Pow(10, options.DecayFitFloorDB/20)

		for sample := start; sample < end; sample++ {
			magnitude := math.Hypot(inPhase[sample], quadrature[sample])
			if magnitude < floor {
				// Below the fit floor the trace is noise or a neighbour, and
				// including it would flatten every slope towards zero.
				break
			}

			times = append(times, float64(sample)/sampleRateHz)
			trace = append(trace, 20*math.Log10(magnitude))
		}

		if len(times) < 16 {
			continue
		}

		slope, _, fitQuality := linearFit(times, trace)
		if slope >= 0 {
			// No decay to speak of: the trace is noise, or a neighbour bleeding
			// through. Neither its level nor its ring time means anything, so
			// the partial is dropped rather than reported with one of them made
			// up.
			continue
		}

		t60 := -60 / slope

		attenuation := sustain.decayAttenuation(math.Log(1000) / t60)
		if attenuation <= 0 {
			continue
		}

		levelLinear[index] = magnitudes[index] / attenuation
		partials[index].T60Seconds = t60
		partials[index].FitQuality = fitQuality
	}

	strongest := 0.0
	for _, level := range levelLinear {
		strongest = max(strongest, level)
	}

	if strongest <= 0 {
		return nil
	}

	kept := partials[:0]

	for index := range partials {
		if levelLinear[index] <= 0 {
			continue
		}

		levelDB := 20 * math.Log10(levelLinear[index]/strongest)
		if levelDB < options.PartialFloorDB {
			continue
		}

		partials[index].LevelDB = levelDB
		kept = append(kept, partials[index])
	}

	if len(kept) > options.MaxPartials {
		byLevel := slices.Clone(kept)
		slices.SortFunc(byLevel, func(a, b Partial) int {
			switch {
			case a.LevelDB > b.LevelDB:
				return -1
			case a.LevelDB < b.LevelDB:
				return 1
			default:
				return 0
			}
		})

		kept = byLevel[:options.MaxPartials]
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
	}

	return kept
}

// linearFit returns the least-squares slope of levels against times, the level
// that line reaches at time zero, and the fit's R².
func linearFit(times, levels []float64) (slope, intercept, rSquared float64) {
	count := float64(len(times))

	var sumTimes, sumLevels float64

	for i := range times {
		sumTimes += times[i]
		sumLevels += levels[i]
	}

	meanTime, meanLevel := sumTimes/count, sumLevels/count

	var covariance, varianceTime, varianceLevel float64

	for i := range times {
		deltaTime, deltaLevel := times[i]-meanTime, levels[i]-meanLevel
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

// measureGlide reports how far the lowest partial falls, in cents, between the
// early and late probes.
//
// Every published tom analysis treats the downward glide as the characteristic
// feature, and in this model it is the one observable that NLIN moves and
// nothing else does.
func measureGlide(hit []float64, sampleRateHz float64, options Options, frequencyHz, cutoffHz float64) float64 {
	inPhase, quadrature := heterodyne(hit, sampleRateHz, frequencyHz, cutoffHz, 1)

	// The heterodyne put the steady partial at DC, so the residual phase slope
	// *is* the deviation from frequencyHz.
	deviation := func(atSeconds float64) (float64, bool) {
		centre := int(atSeconds * sampleRateHz)
		half := int(0.005 * sampleRateHz)

		start, end := centre-half, centre+half
		if start < 1 || end >= len(hit) {
			return 0, false
		}

		var (
			sum   float64
			count int
		)

		for n := start; n < end; n++ {
			previous := math.Atan2(quadrature[n-1], inPhase[n-1])
			current := math.Atan2(quadrature[n], inPhase[n])
			delta := math.Mod(current-previous+3*math.Pi, 2*math.Pi) - math.Pi
			sum += delta
			count++
		}

		if count == 0 {
			return 0, false
		}

		return sum / float64(count) * sampleRateHz / (2 * math.Pi), true
	}

	early, okEarly := deviation(options.GlideEarlySeconds)

	late, okLate := deviation(options.GlideLateSeconds)
	if !okEarly || !okLate {
		return 0
	}

	earlyHz, lateHz := frequencyHz+early, frequencyHz+late
	if earlyHz <= 0 || lateHz <= 0 {
		return 0
	}

	// Positive means the pitch fell, which is the direction a drum bends.
	return 1200 * math.Log2(earlyHz/lateHz)
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

	for band, edge := range edges {
		lowBin := max(0, int(math.Ceil(edge[0]/binHz)))
		highBin := min(len(magnitude)-1, int(math.Floor(edge[1]/binHz)))

		power := 0.0
		for bin := lowBin; bin <= highBin; bin++ {
			power += magnitude[bin] * magnitude[bin]
		}

		levels[band] = 10 * math.Log10(power+1e-30)
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

func measureWindows(hit []float64, sampleRateHz float64, options Options, edges [][2]float64) ([]WindowFeature, error) {
	features := make([]WindowFeature, 0, len(options.Windows))

	for _, span := range options.Windows {
		start := clampIndex(int(span.StartSeconds*sampleRateHz), len(hit))

		end := clampIndex(int(span.EndSeconds*sampleRateHz), len(hit))
		if end-start < 16 {
			continue
		}

		magnitude, err := magnitudeSpectrum(hit[start:end], options.WindowFFTSize)
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

	var levels []float64

	peak := 0.0

	for start := 0; start+frame <= len(hit); start += hop {
		rms := timestats.Calculate(hit[start : start+frame]).RMS

		levels = append(levels, rms)

		if rms > peak {
			peak = rms
		}
	}

	if peak <= 0 {
		return nil
	}

	for i, rms := range levels {
		levels[i] = max(20*math.Log10(rms/peak+1e-30), options.EnvelopeFloorDB)
	}

	return levels
}

// measureAttackBalance is the click-to-body ratio of the strike, in dB.
//
// It names in one number what the hybrid attack layer exists to supply, and it
// is the term ATK.L and ATK.T move most directly.
func measureAttackBalance(hit []float64, sampleRateHz float64, options Options) (float64, error) {
	end := clampIndex(int(options.AttackWindowSeconds*sampleRateHz), len(hit))
	if end < 16 {
		return 0, nil
	}

	magnitude, err := magnitudeSpectrum(hit[:end], options.WindowFFTSize)
	if err != nil {
		return 0, err
	}

	binHz := sampleRateHz / float64(options.WindowFFTSize)

	bandPower := func(lowHz, highHz float64) float64 {
		lowBin := max(0, int(math.Ceil(lowHz/binHz)))
		highBin := min(len(magnitude)-1, int(math.Floor(highHz/binHz)))

		power := 0.0
		for bin := lowBin; bin <= highBin; bin++ {
			power += magnitude[bin] * magnitude[bin]
		}

		return power
	}

	high := bandPower(options.AttackHighMinHz, options.AttackHighMaxHz)
	low := bandPower(options.AttackLowMinHz, options.AttackLowMaxHz)

	return 10 * math.Log10((high+1e-30)/(low+1e-30)), nil
}

func clampIndex(value, length int) int {
	return min(max(value, 0), length)
}

func clampFloat(value, low, high float64) float64 {
	return min(max(value, low), high)
}
