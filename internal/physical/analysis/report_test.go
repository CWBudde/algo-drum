package analysis

import (
	"errors"
	"math"
	"testing"

	"github.com/cwbudde/algo-drum/internal/physical"
)

func TestAnalyzeDefaultPhysicalDrum(t *testing.T) {
	t.Parallel()

	options := DefaultOptions()
	options.DurationSeconds = 0.5
	options.FFTSize = 8192
	report, err := Analyze(physical.DefaultPhysicalDrum(), options)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(report.Modes) != physical.QualityStandard.ModeLimit() {
		t.Fatalf("mode metrics = %d, want %d", len(report.Modes), physical.QualityStandard.ModeLimit())
	}
	if report.Waveform.Peak <= 0 || report.Waveform.RMS <= 0 {
		t.Fatalf("invalid waveform metrics: %#v", report.Waveform)
	}
	if report.Decay.RT60 <= 0 {
		t.Fatalf("RT60 = %v, want positive estimate", report.Decay.RT60)
	}
	if report.Spectrum.CentroidHz <= 0 || report.Spectrum.RolloffHz <= 0 {
		t.Fatalf("invalid spectrum metrics: %#v", report.Spectrum)
	}
	if len(report.SpectralPeaks) != options.PeakCount {
		t.Fatalf("spectral peaks = %d, want %d", len(report.SpectralPeaks), options.PeakCount)
	}
	if len(report.PitchTrack) == 0 {
		t.Fatal("pitch track is empty")
	}
	nearestPeakDifference := math.Inf(1)
	for _, peak := range report.SpectralPeaks {
		nearestPeakDifference = min(
			nearestPeakDifference,
			math.Abs(peak.FrequencyHz-report.Modes[0].FrequencyHz),
		)
	}
	if resolution := report.SampleRateHz / float64(options.FFTSize); nearestPeakDifference > resolution {
		t.Fatalf(
			"lowest analytic mode %.3f Hz has no spectral peak within %.3f Hz; nearest differs by %.3f Hz",
			report.Modes[0].FrequencyHz,
			resolution,
			nearestPeakDifference,
		)
	}
	if difference := math.Abs(
		report.Modes[0].T60Seconds -
			math.Log(1000)/report.Modes[0].DecayRatePerSecond,
	); difference > 1e-14 {
		t.Fatalf("first modal T60 differs by %v", difference)
	}
}

func TestCompareSignalsIsGainInvariant(t *testing.T) {
	t.Parallel()

	reference := make([]float64, 4096)
	scaled := make([]float64, len(reference))
	for index := range reference {
		reference[index] = math.Sin(2 * math.Pi * 217 * float64(index) / 48_000)
		scaled[index] = 0.25 * reference[index]
	}

	comparison, err := CompareSignals(reference, scaled)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.WaveformNRMSE > 1e-14 {
		t.Fatalf("waveform NRMSE = %v, want approximately zero", comparison.WaveformNRMSE)
	}
	if math.Abs(comparison.WaveformCorrelation-1) > 1e-14 {
		t.Fatalf("correlation = %v, want one", comparison.WaveformCorrelation)
	}
	if comparison.SpectrumRMSEDB > 1e-12 {
		t.Fatalf("spectrum RMSE = %v dB, want approximately zero", comparison.SpectrumRMSEDB)
	}
}

func TestCompareSignalsDetectsFrequencyChange(t *testing.T) {
	t.Parallel()

	reference := make([]float64, 4096)
	changed := make([]float64, len(reference))
	for index := range reference {
		reference[index] = math.Sin(2 * math.Pi * 217 * float64(index) / 48_000)
		changed[index] = math.Sin(2 * math.Pi * 260 * float64(index) / 48_000)
	}

	comparison, err := CompareSignals(reference, changed)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.WaveformNRMSE < 0.5 {
		t.Fatalf("waveform NRMSE = %v, want clear change", comparison.WaveformNRMSE)
	}
	if comparison.SpectrumRMSEDB < 10 {
		t.Fatalf("spectrum RMSE = %v dB, want clear change", comparison.SpectrumRMSEDB)
	}
}

func TestAnalyzeRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	options := DefaultOptions()
	options.FFTSize = 1
	if _, err := Analyze(physical.DefaultPhysicalDrum(), options); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Analyze() error = %v, want ErrInvalidOptions", err)
	}

	if _, err := CompareSignals(nil, nil); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("CompareSignals() error = %v, want ErrInvalidOptions", err)
	}
}
