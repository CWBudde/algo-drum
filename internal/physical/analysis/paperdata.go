package analysis

import (
	"fmt"
	"math"

	"github.com/cwbudde/algo-drum/internal/physical"
)

// PaperDataVersion is the schema emitted by GeneratePaperData.
const PaperDataVersion = 1

// paperResponsePoints is the resolution of the cavity sweep. The doublet the
// figure exists to show is a few hertz wide at its narrowest, and a log grid of
// this length resolves it to a few cents across the plotted span while keeping
// the committed artefact small enough to read in a diff.
const paperResponsePoints = 900

const (
	paperResponseLowHz  = 60
	paperResponseHighHz = 2000
)

// paperCavityScales are the stiffness scales the cavity figure compares: the
// uncoupled limit, the shipped fitted value, and the rigid-enclosure ceiling.
var paperCavityScales = [...]float64{0, 0.083, 1}

// paperAttackBandRatios mirrors attackBandRatios, which is unexported. It is
// duplicated rather than exported on purpose: what the figure asserts is that
// the layer's releases follow from the head's loss law, and re-deriving them
// here from the public law is the check, not a shortcut around it.
var paperAttackBandRatios = [...]float64{0.4, 1, 2.5}

// PaperData is the committed artefact the paper's model figures are drawn from.
//
// It is derived, not rendered: every number here follows from a configuration by
// closed form — the modal bank from the dispersion relation and the loss law,
// the cavity curves from the continuous-time reference solve. Nothing in it
// depends on an audio render, a recording, or a random seed, which is what lets
// the paper's model figures be reproduced from the repository alone.
type PaperData struct {
	SchemaVersion  int                `json:"schemaVersion"`
	Provenance     string             `json:"provenance"`
	Conditions     string             `json:"conditions"`
	SampleRateHz   float64            `json:"sampleRateHz"`
	Quality        string             `json:"quality"`
	Heads          []PaperHead        `json:"heads"`
	Modes          []PaperMode        `json:"modes"`
	Tiers          []PaperTier        `json:"tiers"`
	AttackBands    []PaperAttackBand  `json:"attackBands"`
	CavityResponse PaperCavityCurves  `json:"cavityResponse"`
	Cavity         PaperCavitySummary `json:"cavity"`
}

// PaperHead is one membrane's constitutive data, so a reader of the artefact can
// re-derive any mode in it without the configuration beside them.
type PaperHead struct {
	Name                   string  `json:"name"`
	AxisymmetricOnly       bool    `json:"axisymmetricOnly"`
	RadiusM                float64 `json:"radiusM"`
	SurfaceDensityKgPerM2  float64 `json:"surfaceDensityKgPerM2"`
	TensionNPerM           float64 `json:"tensionNPerM"`
	BendingStiffnessNM     float64 `json:"bendingStiffnessNM"`
	WaveSpeedMPerS         float64 `json:"waveSpeedMPerS"`
	Loss0PerSecond         float64 `json:"loss0PerSecond"`
	Loss1MPerSecond        float64 `json:"loss1MPerSecond"`
	Loss2M2PerSecond       float64 `json:"loss2M2PerSecond"`
	RadiationLossPerSecond float64 `json:"radiationLossPerSecond"`
	ModeCount              int     `json:"modeCount"`
}

// PaperMode is one retained oscillator of the whole instrument. It extends
// ModeMetric with the head it belongs to, the wavenumber the loss law is
// evaluated at, the damping ratio that law is shaped to hold constant, and the
// swept area the cavity couples through — none of which can be recovered from
// the metric alone.
type PaperMode struct {
	Head           string  `json:"head"`
	WavenumberPerM float64 `json:"wavenumberPerM"`
	DampingRatio   float64 `json:"dampingRatio"`
	SweptAreaM2    float64 `json:"sweptAreaM2"`

	// FarFieldRadiationWeight is the same mode's weight with the near-field
	// term switched off, which is what a distant microphone hears. Carried
	// beside the shipped weight because the difference between the two is the
	// entire justification for the near-field term: in the far field a head this
	// size is nearly a monopole below its top retained mode, so without it the
	// output is very nearly the axisymmetric modes alone.
	FarFieldRadiationWeight float64 `json:"farFieldRadiationWeight"`

	ModeMetric
}

// PaperTier records what one quality budget buys at this tuning. The paper's
// search chapter reports the same quantity for a bank tuned two octaves down,
// where the same budget reaches a very different ceiling; the two are only
// readable together if both name their tuning.
type PaperTier struct {
	Quality       string  `json:"quality"`
	SlotBudget    int     `json:"slotBudget"`
	BatterModes   int     `json:"batterModes"`
	ResonantModes int     `json:"resonantModes"`
	TopModeHz     float64 `json:"topModeHz"`
}

// PaperAttackBand is one band of the stochastic layer, with the release the
// head's own loss law gives it. These are derived quantities, not settings.
type PaperAttackBand struct {
	CentreHz           float64 `json:"centreHz"`
	WavenumberPerM     float64 `json:"wavenumberPerM"`
	DecayRatePerSecond float64 `json:"decayRatePerSecond"`
	T60Seconds         float64 `json:"t60Seconds"`
}

// PaperCavityCurves is the continuous-time radiated response at each compared
// cavity stiffness, on one shared frequency grid.
type PaperCavityCurves struct {
	FrequencyHz []float64          `json:"frequencyHz"`
	Curves      []PaperCavityCurve `json:"curves"`
}

// PaperCavityCurve is one stiffness scale's magnitude response, in dB relative
// to that curve's own peak. Relative, because the absolute level carries an
// output gain fitted against a different question, and three absolute curves
// would invite the figure to be read as a loudness comparison.
type PaperCavityCurve struct {
	StiffnessScale float64   `json:"stiffnessScale"`
	MagnitudeDB    []float64 `json:"magnitudeDb"`
	LowerBranchHz  float64   `json:"lowerBranchHz"`
	UpperBranchHz  float64   `json:"upperBranchHz"`
}

// PaperCavitySummary carries the lumped constants the cavity section quotes.
type PaperCavitySummary struct {
	DepthM                 float64 `json:"depthM"`
	VolumeM3               float64 `json:"volumeM3"`
	StiffnessScale         float64 `json:"stiffnessScale"`
	BulkStiffnessPaPerM3   float64 `json:"bulkStiffnessPaPerM3"`
	RigidStiffnessPaPerM3  float64 `json:"rigidStiffnessPaPerM3"`
	LossPerSecond          float64 `json:"lossPerSecond"`
	FundamentalSweptAreaM2 float64 `json:"fundamentalSweptAreaM2"`
}

// GeneratePaperData derives the artefact behind the paper's model figures.
func GeneratePaperData(config physical.PhysicalDrum) (PaperData, error) {
	if err := config.Validate(); err != nil {
		return PaperData{}, err
	}

	modes, batterCount, err := paperModes(config)
	if err != nil {
		return PaperData{}, err
	}

	tiers, err := paperTiers(config)
	if err != nil {
		return PaperData{}, err
	}

	response, err := paperCavityResponse(config)
	if err != nil {
		return PaperData{}, err
	}

	volume := math.Pi * config.Batter.RadiusM * config.Batter.RadiusM * config.Cavity.DepthM
	rigid := config.Cavity.AirDensityKgPerM3 *
		config.Cavity.SoundSpeedMPerS * config.Cavity.SoundSpeedMPerS / volume

	return PaperData{
		SchemaVersion: PaperDataVersion,
		Provenance: "Derived from DefaultPhysicalDrum by " +
			"go run ./cmd/analyze-physical -paper-data docs/paper/model-data.json",
		Conditions: "Closed-form modal bank and continuous-time cavity solve; " +
			"no render, no recording, no random seed",
		SampleRateHz: config.SampleRateHz,
		Quality:      string(config.Quality),
		Heads: []PaperHead{
			paperHead("batter", config.Batter, batterCount),
			paperHead("resonant", config.Resonant, len(modes)-batterCount),
		},
		Modes:          modes,
		Tiers:          tiers,
		AttackBands:    paperAttackBands(config),
		CavityResponse: response,
		Cavity: PaperCavitySummary{
			DepthM:                 config.Cavity.DepthM,
			VolumeM3:               volume,
			StiffnessScale:         config.Cavity.StiffnessScale,
			BulkStiffnessPaPerM3:   config.Cavity.StiffnessScale * rigid,
			RigidStiffnessPaPerM3:  rigid,
			LossPerSecond:          config.Cavity.LossPerSecond,
			FundamentalSweptAreaM2: fundamentalSweptArea(modes),
		},
	}, nil
}

func paperHead(name string, head physical.Head, modeCount int) PaperHead {
	return PaperHead{
		Name:                   name,
		AxisymmetricOnly:       head.AxisymmetricOnly,
		RadiusM:                head.RadiusM,
		SurfaceDensityKgPerM2:  head.SurfaceDensityKgPerM2,
		TensionNPerM:           head.TensionNPerM,
		BendingStiffnessNM:     head.BendingStiffnessNM,
		WaveSpeedMPerS:         physical.WaveSpeedMPerS(head),
		Loss0PerSecond:         head.Loss0PerSecond,
		Loss1MPerSecond:        head.Loss1MPerSecond,
		Loss2M2PerSecond:       head.Loss2M2PerSecond,
		RadiationLossPerSecond: head.RadiationLossPerSecond,
		ModeCount:              modeCount,
	}
}

// paperModes returns the whole instrument's bank and the index at which the
// resonant head starts. GenerateDrumModes concatenates the two heads in the
// order NewDoubleHead assembles them and GenerateModes returns the batter alone,
// so the split follows from the two lengths rather than from a second selection.
func paperModes(config physical.PhysicalDrum) ([]PaperMode, int, error) {
	modes, err := physical.GenerateDrumModes(config)
	if err != nil {
		return nil, 0, err
	}

	batter, err := physical.GenerateModes(config)
	if err != nil {
		return nil, 0, err
	}

	// Mode selection depends on the heads, the quality budget and the sample
	// rate, and on nothing about the microphone — so the far-field bank retains
	// exactly the same modes in the same order and can be paired by index.
	farField := config
	farField.Pickup.NearFieldScale = 0

	distant, err := physical.GenerateDrumModes(farField)
	if err != nil {
		return nil, 0, err
	}

	if len(distant) != len(modes) {
		return nil, 0, fmt.Errorf(
			"far-field bank has %d modes against %d: mode selection is not "+
				"independent of the microphone",
			len(distant),
			len(modes),
		)
	}

	batterCount := len(batter)
	paper := make([]PaperMode, 0, len(modes))

	for index, mode := range modes {
		head := "batter"
		if index >= batterCount {
			head = "resonant"
		}

		t60 := math.Inf(1)
		if mode.DecayRatePerSecond > 0 {
			t60 = math.Log(1000) / mode.DecayRatePerSecond
		}

		dampingRatio := 0.0
		if mode.AngularFrequency > 0 {
			dampingRatio = mode.DecayRatePerSecond / mode.AngularFrequency
		}

		paper = append(paper, PaperMode{
			Head:                    head,
			WavenumberPerM:          mode.WavenumberPerM,
			DampingRatio:            dampingRatio,
			SweptAreaM2:             mode.SweptAreaM2,
			FarFieldRadiationWeight: distant[index].RadiationWeight,
			ModeMetric: ModeMetric{
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
			},
		})
	}

	return paper, batterCount, nil
}

func paperTiers(config physical.PhysicalDrum) ([]PaperTier, error) {
	qualities := []physical.Quality{
		physical.QualityDraft,
		physical.QualityStandard,
		physical.QualityHigh,
	}

	tiers := make([]PaperTier, 0, len(qualities))

	for _, quality := range qualities {
		probe := config
		probe.Quality = quality

		modes, err := physical.GenerateDrumModes(probe)
		if err != nil {
			return nil, err
		}

		batter, err := physical.GenerateModes(probe)
		if err != nil {
			return nil, err
		}

		top := 0.0
		for _, mode := range modes {
			top = max(top, mode.FrequencyHz)
		}

		tiers = append(tiers, PaperTier{
			Quality:       string(quality),
			SlotBudget:    quality.ModeLimit(),
			BatterModes:   len(batter),
			ResonantModes: len(modes) - len(batter),
			TopModeHz:     top,
		})
	}

	return tiers, nil
}

// paperAttackBands reproduces newAttackLayer's derivation from the public loss
// law, so a reader can check in three lines that the layer's releases are an
// extrapolation of the mode series rather than free parameters beside it.
func paperAttackBands(config physical.PhysicalDrum) []PaperAttackBand {
	speed := physical.WaveSpeedMPerS(config.Batter)
	bands := make([]PaperAttackBand, 0, len(paperAttackBandRatios))

	for _, ratio := range paperAttackBandRatios {
		centreHz := min(config.Attack.CentreHz*ratio, config.SampleRateHz*0.45)
		wavenumber := 2 * math.Pi * centreHz / speed
		rate := physical.ModalDecayRatePerSecond(config.Batter, wavenumber)

		t60 := math.Inf(1)
		if rate > 0 {
			t60 = math.Log(1000) * config.Attack.DecayScale / rate
		}

		bands = append(bands, PaperAttackBand{
			CentreHz:           centreHz,
			WavenumberPerM:     wavenumber,
			DecayRatePerSecond: rate,
			T60Seconds:         t60,
		})
	}

	return bands
}

func paperCavityResponse(config physical.PhysicalDrum) (PaperCavityCurves, error) {
	grid := make([]float64, paperResponsePoints)
	step := math.Log(paperResponseHighHz/paperResponseLowHz) /
		float64(paperResponsePoints-1)

	for index := range grid {
		grid[index] = paperResponseLowHz * math.Exp(step*float64(index))
	}

	uncoupledLower, uncoupledUpper, err := uncoupledFundamentals(config)
	if err != nil {
		return PaperCavityCurves{}, err
	}

	curves := make([]PaperCavityCurve, 0, len(paperCavityScales))

	for _, scale := range paperCavityScales {
		probe := config
		probe.Cavity.StiffnessScale = scale

		model, err := physical.NewDoubleHead(probe)
		if err != nil {
			return PaperCavityCurves{}, fmt.Errorf("cavity scale %v: %w", scale, err)
		}

		magnitude := make([]float64, len(grid))
		pressure := make([]float64, len(grid))

		for index, frequency := range grid {
			response, err := model.ReferenceFrequencyResponse(frequency)
			if err != nil {
				return PaperCavityCurves{}, fmt.Errorf(
					"cavity scale %v at %.3f Hz: %w",
					scale,
					frequency,
					err,
				)
			}

			magnitude[index] = math.Hypot(
				real(response.RawRadiated),
				imag(response.RawRadiated),
			)
			pressure[index] = math.Hypot(
				real(response.CavityPressurePa),
				imag(response.CavityPressurePa),
			)
		}

		// The branches are read off the cavity pressure, not off the radiated
		// magnitude. Pressure is driven through the swept area, which is exactly
		// zero for every m > 0 mode, so its peaks are the axisymmetric branches
		// and nothing else; the radiated curve also contains the (1,1) pair,
		// which sits between the two branches at the shipped stiffness and above
		// the upper one at the rigid ceiling, so peak-picking it would report a
		// different mode at each scale.
		lower, upper := uncoupledLower, uncoupledUpper
		if scale > 0 {
			lower, upper = lowestTwoPeaks(grid, pressure)
		}

		curves = append(curves, PaperCavityCurve{
			StiffnessScale: scale,
			MagnitudeDB:    relativeDB(magnitude),
			LowerBranchHz:  lower,
			UpperBranchHz:  upper,
		})
	}

	return PaperCavityCurves{FrequencyHz: grid, Curves: curves}, nil
}

// uncoupledFundamentals returns the two heads' (0,1) frequencies with the air
// spring absent. At zero stiffness there is no pressure to read a branch off —
// the heads are two independent membranes — so the comparison the figure is for
// needs them stated rather than detected.
func uncoupledFundamentals(config physical.PhysicalDrum) (float64, float64, error) {
	modes, err := physical.GenerateDrumModes(config)
	if err != nil {
		return 0, 0, err
	}

	fundamentals := make([]float64, 0, 2)

	for _, mode := range modes {
		if mode.AzimuthalOrder == 0 && mode.RadialOrder == 1 {
			fundamentals = append(fundamentals, mode.FrequencyHz)
		}
	}

	switch len(fundamentals) {
	case 0:
		return 0, 0, nil
	case 1:
		return fundamentals[0], 0, nil
	default:
		return min(fundamentals[0], fundamentals[1]),
			max(fundamentals[0], fundamentals[1]),
			nil
	}
}

// lowestTwoPeaks reports the two lowest local maxima of a curve.
//
// The grid is logarithmic and coarse enough that a bare bin index would place a
// branch some tens of cents from where it is, which is a quantity this paper
// reads in cents elsewhere. The same log-domain parabolic interpolation the
// spectral-peak detector uses recovers the sub-bin position, evaluated in log
// frequency because that is the axis the grid is uniform on.
func lowestTwoPeaks(frequencyHz, magnitude []float64) (float64, float64) {
	peaks := make([]float64, 0, 2)

	for index := 1; index+1 < len(magnitude); index++ {
		if magnitude[index] <= magnitude[index-1] ||
			magnitude[index] <= magnitude[index+1] {
			continue
		}

		offset := parabolicOffset(
			magnitude[index-1],
			magnitude[index],
			magnitude[index+1],
		)
		step := math.Log(frequencyHz[index+1] / frequencyHz[index])
		peaks = append(peaks, frequencyHz[index]*math.Exp(offset*step))

		if len(peaks) == 2 {
			break
		}
	}

	switch len(peaks) {
	case 0:
		return 0, 0
	case 1:
		return peaks[0], 0
	default:
		return peaks[0], peaks[1]
	}
}

func relativeDB(magnitude []float64) []float64 {
	peak := 0.0
	for _, value := range magnitude {
		peak = max(peak, value)
	}

	if peak == 0 {
		peak = 1
	}

	decibels := make([]float64, len(magnitude))
	for index, value := range magnitude {
		decibels[index] = 20 * math.Log10(max(value/peak, 1e-12))
	}

	return decibels
}

// fundamentalSweptArea returns the (0,1) coupling coefficient of whichever head
// comes first, which is the batter: the number the cavity section quotes when it
// says how much air one mode displaces.
func fundamentalSweptArea(modes []PaperMode) float64 {
	for _, mode := range modes {
		if mode.AzimuthalOrder == 0 && mode.RadialOrder == 1 {
			return mode.SweptAreaM2
		}
	}

	return 0
}
