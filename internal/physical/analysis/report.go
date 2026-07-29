// Package analysis provides offline calibration and regression measurements
// for the real-time physical drum model.
package analysis

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/algo-dsp/dsp/spectrum"
	"github.com/cwbudde/algo-dsp/dsp/window"
	"github.com/cwbudde/algo-dsp/measure/ir"
	frequencystats "github.com/cwbudde/algo-dsp/stats/frequency"
	timestats "github.com/cwbudde/algo-dsp/stats/time"
	algofft "github.com/cwbudde/algo-fft"
)

const (
	defaultDurationSeconds = 2
	defaultFFTSize         = 16_384
	defaultPitchFrameSize  = 4096
	defaultPitchHopSize    = 1024
	defaultPeakCount       = 12
)

var ErrInvalidOptions = errors.New("invalid physical analysis options")

// Options controls one offline render and its analysis resolution.
type Options struct {
	DurationSeconds float64
	Velocity01      float64
	FFTSize         int
	PitchFrameSize  int
	PitchHopSize    int
	PeakCount       int
}

// DefaultOptions returns settings suitable for committed calibration metrics.
func DefaultOptions() Options {
	return Options{
		DurationSeconds: defaultDurationSeconds,
		Velocity01:      0.8,
		FFTSize:         defaultFFTSize,
		PitchFrameSize:  defaultPitchFrameSize,
		PitchHopSize:    defaultPitchHopSize,
		PeakCount:       defaultPeakCount,
	}
}

// ModeMetric records analytic targets derived directly from a modal pole.
type ModeMetric struct {
	AzimuthalOrder           int     `json:"azimuthalOrder"`
	RadialOrder              int     `json:"radialOrder"`
	Orientation              string  `json:"orientation"`
	FrequencyHz              float64 `json:"frequencyHz"`
	StructuralDecayPerSecond float64 `json:"structuralDecayPerSecond"`
	RadiationDecayPerSecond  float64 `json:"radiationDecayPerSecond"`
	DecayCorrectionPerSecond float64 `json:"decayCorrectionPerSecond"`
	DecayRatePerSecond       float64 `json:"decayRatePerSecond"`
	T60Seconds               float64 `json:"t60Seconds"`
	RadiationWeight          float64 `json:"radiationWeight"`
}

// WaveformMetric is a compact time-domain signature.
type WaveformMetric struct {
	Peak        float64 `json:"peak"`
	RMS         float64 `json:"rms"`
	Energy      float64 `json:"energy"`
	CrestFactor float64 `json:"crestFactor"`
}

// SpectrumMetric is a compact frequency-domain signature.
type SpectrumMetric struct {
	CentroidHz  float64 `json:"centroidHz"`
	RolloffHz   float64 `json:"rolloffHz"`
	BandwidthHz float64 `json:"bandwidthHz"`
	Flatness    float64 `json:"flatness"`
}

// Peak identifies one local spectral maximum.
type Peak struct {
	FrequencyHz float64 `json:"frequencyHz"`
	Magnitude   float64 `json:"magnitude"`
}

// PitchPoint is the strongest modal-frequency track at one analysis frame.
type PitchPoint struct {
	TimeSeconds float64 `json:"timeSeconds"`
	FrequencyHz float64 `json:"frequencyHz"`
	Magnitude   float64 `json:"magnitude"`
}

// Report contains analytic modes plus measured waveform, decay, spectrum, and
// pitch-track metrics for one deterministic synthesized hit.
type Report struct {
	SampleRateHz    float64        `json:"sampleRateHz"`
	SampleCount     int            `json:"sampleCount"`
	Velocity01      float64        `json:"velocity01"`
	StrikeRadius01  float64        `json:"strikeRadius01"`
	PickupRadius01  float64        `json:"pickupRadius01"`
	PickupAngleRad  float64        `json:"pickupAngleRad"`
	PickupDistanceM float64        `json:"pickupDistanceM"`
	Modes           []ModeMetric   `json:"modes,omitempty"`
	Waveform        WaveformMetric `json:"waveform"`
	Decay           ir.Metrics     `json:"decay"`
	Spectrum        SpectrumMetric `json:"spectrum"`
	SpectralPeaks   []Peak         `json:"spectralPeaks"`
	PitchTrack      []PitchPoint   `json:"pitchTrack"`
}

// Comparison supplies scale-aware waveform and log-spectrum regression
// metrics. Identical signals produce zero errors and correlation one.
type Comparison struct {
	WaveformNRMSE       float64 `json:"waveformNrmse"`
	WaveformCorrelation float64 `json:"waveformCorrelation"`
	SpectrumRMSEDB      float64 `json:"spectrumRmseDb"`
}

// Analyze renders one hit and returns its deterministic offline report.
func Analyze(config physical.PhysicalDrum, options Options) (Report, error) {
	if err := validateOptions(config, options); err != nil {
		return Report{}, err
	}

	model, err := physical.NewSingleHead(config)
	if err != nil {
		return Report{}, err
	}

	if err := model.Trigger(options.Velocity01); err != nil {
		return Report{}, err
	}

	sampleCount := int(math.Round(options.DurationSeconds * config.SampleRateHz))

	radiated := make([]float64, sampleCount)
	for index := range radiated {
		radiated[index] = model.Tick().Radiated
	}

	magnitude, err := magnitudeSpectrum(radiated, options.FFTSize)
	if err != nil {
		return Report{}, err
	}

	frequency := frequencystats.Calculate(magnitude, config.SampleRateHz)
	time := timestats.Calculate(radiated)

	decay, err := ir.NewAnalyzer(config.SampleRateHz).Analyze(radiated)
	if err != nil {
		return Report{}, err
	}

	firstMode, _ := model.Mode(0)

	pitchTrack, err := trackPitch(
		radiated,
		config.SampleRateHz,
		options.PitchFrameSize,
		options.PitchHopSize,
		firstMode.FrequencyHz,
	)
	if err != nil {
		return Report{}, err
	}

	return Report{
		SampleRateHz:    config.SampleRateHz,
		SampleCount:     sampleCount,
		Velocity01:      options.Velocity01,
		StrikeRadius01:  config.Strike.Radius01,
		PickupRadius01:  config.Pickup.Radius01,
		PickupAngleRad:  config.Pickup.AngleRad,
		PickupDistanceM: config.Pickup.DistanceM,
		Modes:           modeMetrics(model),
		Waveform: WaveformMetric{
			Peak:        time.Peak,
			RMS:         time.RMS,
			Energy:      time.Energy,
			CrestFactor: time.CrestFactor,
		},
		Decay: decay,
		Spectrum: SpectrumMetric{
			CentroidHz:  frequency.Centroid,
			RolloffHz:   frequency.Rolloff,
			BandwidthHz: frequency.Bandwidth,
			Flatness:    frequency.Flatness,
		},
		SpectralPeaks: strongestPeaks(
			magnitude,
			config.SampleRateHz,
			options.FFTSize,
			options.PeakCount,
		),
		PitchTrack: pitchTrack,
	}, nil
}

// CompareSignals compares equal-rate signals using a least-squares gain fit,
// normalized waveform error, correlation, and log-magnitude spectral error.
func CompareSignals(reference, candidate []float64) (Comparison, error) {
	if len(reference) < 2 || len(candidate) < 2 {
		return Comparison{}, fmt.Errorf("%w: signals need at least two samples", ErrInvalidOptions)
	}

	sampleCount := min(len(reference), len(candidate))
	reference = reference[:sampleCount]
	candidate = candidate[:sampleCount]

	var referenceEnergy, candidateEnergy, cross float64

	for index, referenceSample := range reference {
		candidateSample := candidate[index]
		referenceEnergy += referenceSample * referenceSample
		candidateEnergy += candidateSample * candidateSample
		cross += referenceSample * candidateSample
	}

	if referenceEnergy == 0 || candidateEnergy == 0 {
		return Comparison{}, fmt.Errorf("%w: signals must contain energy", ErrInvalidOptions)
	}

	gain := cross / candidateEnergy
	errorEnergy := 0.0

	for index, referenceSample := range reference {
		difference := referenceSample - gain*candidate[index]
		errorEnergy += difference * difference
	}

	fftSize := floorPowerOfTwo(min(sampleCount, defaultFFTSize))

	referenceMagnitude, err := magnitudeSpectrum(reference, fftSize)
	if err != nil {
		return Comparison{}, err
	}

	candidateMagnitude, err := magnitudeSpectrum(candidate, fftSize)
	if err != nil {
		return Comparison{}, err
	}

	spectrumError := 0.0

	for index := range referenceMagnitude {
		referenceDB := 20 * math.Log10(max(referenceMagnitude[index], 1e-15))
		candidateDB := 20 * math.Log10(max(candidateMagnitude[index]*math.Abs(gain), 1e-15))
		difference := referenceDB - candidateDB
		spectrumError += difference * difference
	}

	return Comparison{
		WaveformNRMSE:       math.Sqrt(errorEnergy / referenceEnergy),
		WaveformCorrelation: cross / math.Sqrt(referenceEnergy*candidateEnergy),
		SpectrumRMSEDB:      math.Sqrt(spectrumError / float64(len(referenceMagnitude))),
	}, nil
}

func validateOptions(config physical.PhysicalDrum, options Options) error {
	if err := config.Validate(); err != nil {
		return err
	}

	if !finite(options.DurationSeconds) ||
		options.DurationSeconds <= 0 ||
		options.DurationSeconds > 30 {
		return fmt.Errorf("%w: duration %v outside (0,30]", ErrInvalidOptions, options.DurationSeconds)
	}

	if !finite(options.Velocity01) ||
		options.Velocity01 < 0 ||
		options.Velocity01 > 1 {
		return fmt.Errorf("%w: velocity %v outside [0,1]", ErrInvalidOptions, options.Velocity01)
	}

	sampleCount := int(math.Round(options.DurationSeconds * config.SampleRateHz))
	if options.FFTSize < 2 || options.FFTSize > sampleCount {
		return fmt.Errorf("%w: FFT size %d outside [2,%d]", ErrInvalidOptions, options.FFTSize, sampleCount)
	}

	if options.PitchFrameSize < 2 || options.PitchFrameSize > sampleCount {
		return fmt.Errorf(
			"%w: pitch frame size %d outside [2,%d]",
			ErrInvalidOptions,
			options.PitchFrameSize,
			sampleCount,
		)
	}

	if options.PitchHopSize < 1 || options.PitchHopSize > options.PitchFrameSize {
		return fmt.Errorf("%w: pitch hop %d invalid", ErrInvalidOptions, options.PitchHopSize)
	}

	if options.PeakCount < 1 || options.PeakCount > 100 {
		return fmt.Errorf("%w: peak count %d outside [1,100]", ErrInvalidOptions, options.PeakCount)
	}

	return nil
}

func modeMetrics(model *physical.SingleHead) []ModeMetric {
	metrics := make([]ModeMetric, 0, model.ModeCount())
	for index := range model.ModeCount() {
		mode, _ := model.Mode(index)

		t60 := math.Inf(1)
		if mode.DecayRatePerSecond > 0 {
			t60 = math.Log(1000) / mode.DecayRatePerSecond
		}

		metrics = append(metrics, ModeMetric{
			AzimuthalOrder:           mode.AzimuthalOrder,
			RadialOrder:              mode.RadialOrder,
			Orientation:              mode.Orientation.String(),
			FrequencyHz:              mode.FrequencyHz,
			StructuralDecayPerSecond: mode.StructuralDecayPerSecond,
			RadiationDecayPerSecond:  mode.RadiationDecayPerSecond,
			DecayCorrectionPerSecond: mode.DecayCorrectionPerSecond,
			DecayRatePerSecond:       mode.DecayRatePerSecond,
			T60Seconds:               t60,
			RadiationWeight:          mode.RadiationWeight,
		})
	}

	return metrics
}

func magnitudeSpectrum(samples []float64, fftSize int) ([]float64, error) {
	if fftSize < 2 || len(samples) < fftSize {
		return nil, fmt.Errorf("%w: FFT size %d exceeds %d samples", ErrInvalidOptions, fftSize, len(samples))
	}

	input := append([]float64(nil), samples[:fftSize]...)

	coefficients := window.Generate(window.TypeHann, fftSize, window.WithPeriodic())
	for index := range input {
		input[index] *= coefficients[index]
	}

	plan, err := algofft.NewPlanReal64(fftSize)
	if err != nil {
		return nil, fmt.Errorf("create real FFT: %w", err)
	}

	bins := make([]complex128, plan.SpectrumLen())
	if err := plan.Forward(bins, input); err != nil {
		return nil, fmt.Errorf("real FFT: %w", err)
	}

	return spectrum.Magnitude(bins), nil
}

func strongestPeaks(magnitude []float64, sampleRate float64, fftSize, count int) []Peak {
	peaks := make([]Peak, 0, count*2)

	for index := 1; index+1 < len(magnitude); index++ {
		if magnitude[index] <= magnitude[index-1] ||
			magnitude[index] < magnitude[index+1] {
			continue
		}

		offset := parabolicOffset(
			magnitude[index-1],
			magnitude[index],
			magnitude[index+1],
		)
		peaks = append(peaks, Peak{
			FrequencyHz: (float64(index) + offset) * sampleRate / float64(fftSize),
			Magnitude:   magnitude[index],
		})
	}

	sort.Slice(peaks, func(left, right int) bool {
		return peaks[left].Magnitude > peaks[right].Magnitude
	})

	if len(peaks) > count {
		peaks = peaks[:count]
	}

	return peaks
}

func trackPitch(
	samples []float64,
	sampleRate float64,
	frameSize, hopSize int,
	targetFrequencyHz float64,
) ([]PitchPoint, error) {
	track := make([]PitchPoint, 0, 1+(len(samples)-frameSize)/hopSize)
	minBin := max(1, int(math.Floor(targetFrequencyHz*0.70*float64(frameSize)/sampleRate)))
	maxBin := min(
		frameSize/2-1,
		int(math.Ceil(targetFrequencyHz*1.30*float64(frameSize)/sampleRate)),
	)

	for start := 0; start+frameSize <= len(samples); start += hopSize {
		magnitude, err := magnitudeSpectrum(samples[start:start+frameSize], frameSize)
		if err != nil {
			return nil, err
		}

		peakBin := minBin
		for index := minBin + 1; index <= maxBin; index++ {
			if magnitude[index] > magnitude[peakBin] {
				peakBin = index
			}
		}

		offset := parabolicOffset(
			magnitude[peakBin-1],
			magnitude[peakBin],
			magnitude[peakBin+1],
		)
		track = append(track, PitchPoint{
			TimeSeconds: (float64(start) + float64(frameSize)/2) / sampleRate,
			FrequencyHz: (float64(peakBin) + offset) * sampleRate / float64(frameSize),
			Magnitude:   magnitude[peakBin],
		})
	}

	return track, nil
}

func parabolicOffset(left, center, right float64) float64 {
	left = math.Log(max(left, 1e-30))
	center = math.Log(max(center, 1e-30))
	right = math.Log(max(right, 1e-30))

	denominator := left - 2*center + right
	if denominator == 0 {
		return 0
	}

	return 0.5 * (left - right) / denominator
}

func floorPowerOfTwo(value int) int {
	result := 1
	for result <= value/2 {
		result *= 2
	}

	return result
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
