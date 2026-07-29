package physical

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

const (
	maxModeOrder = 28
	rootScanStep = math.Pi / 4
)

var ErrInvalidModeIndex = errors.New("invalid circular mode index")

// Orientation distinguishes the two degenerate members of an azimuthal mode.
type Orientation uint8

const (
	OrientationCosine Orientation = iota
	OrientationSine
)

func (o Orientation) String() string {
	if o == OrientationSine {
		return "sin"
	}

	return "cos"
}

type eigenmode struct {
	azimuthalOrder int
	radialOrder    int
	besselZero     float64
	wavenumber     float64
	angularFreq    float64
}

// BesselZero returns the radialIndex-th positive zero of J_order.
func BesselZero(order, radialIndex int) (float64, error) {
	if order < 0 || order > maxModeOrder ||
		radialIndex < 1 || radialIndex > maxModeOrder {
		return 0, fmt.Errorf("%w: order=%d radial=%d", ErrInvalidModeIndex, order, radialIndex)
	}

	return besselZeros(order, radialIndex)[radialIndex-1], nil
}

// GenerateModes constructs a frequency-ordered modal basis for the batter
// head, truncated by both the quality budget and the anti-alias frequency.
func GenerateModes(config PhysicalDrum) ([]Mode, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return generateHeadModes(config, config.Batter)
}

func generateHeadModes(config PhysicalDrum, head Head) ([]Mode, error) {
	frequencyLimit := head.FrequencyLimitFraction * config.SampleRateHz
	candidates := make([]eigenmode, 0, maxModeOrder*maxModeOrder)

	for azimuthalOrder := 0; azimuthalOrder <= maxModeOrder; azimuthalOrder++ {
		zeros := besselZeros(azimuthalOrder, maxModeOrder)
		for radialOffset, zero := range zeros {
			wavenumber := zero / head.RadiusM

			angularFreq := naturalAngularFrequency(head, wavenumber)

			highestAngularFreq := angularFreq
			if azimuthalOrder > 0 {
				highestAngularFreq *= 1 + head.TensionAsymmetry.SplitRatio/2
			}

			if highestAngularFreq/(2*math.Pi) > frequencyLimit {
				break
			}

			candidates = append(candidates, eigenmode{
				azimuthalOrder: azimuthalOrder,
				radialOrder:    radialOffset + 1,
				besselZero:     zero,
				wavenumber:     wavenumber,
				angularFreq:    angularFreq,
			})
		}
	}

	sort.Slice(candidates, func(first, second int) bool {
		left := candidates[first]

		right := candidates[second]
		if left.angularFreq != right.angularFreq {
			return left.angularFreq < right.angularFreq
		}

		if left.azimuthalOrder != right.azimuthalOrder {
			return left.azimuthalOrder < right.azimuthalOrder
		}

		return left.radialOrder < right.radialOrder
	})

	limit := config.Quality.ModeLimit()
	modes := make([]Mode, 0, limit)

	for _, candidate := range candidates {
		orientations := [...]Orientation{OrientationCosine, OrientationSine}

		orientationCount := len(orientations)
		if candidate.azimuthalOrder == 0 {
			orientationCount = 1
		}

		if len(modes)+orientationCount > limit {
			continue
		}

		for _, orientation := range orientations[:orientationCount] {
			mode, err := buildMode(config, head, candidate, orientation)
			if err != nil {
				return nil, err
			}

			modes = append(modes, mode)
		}

		if len(modes) == limit {
			break
		}
	}

	if len(modes) == 0 {
		return nil, fmt.Errorf("%w: no modes below frequency limit", ErrInvalidConfig)
	}

	// A large allowed split can move one member across a nearby mode. Preserve
	// the API's frequency-order guarantee; stable sorting keeps ideal
	// zero-split cosine/sine pairs in their original orientation order.
	sort.SliceStable(modes, func(first, second int) bool {
		return modes[first].FrequencyHz < modes[second].FrequencyHz
	})

	return modes, nil
}

// NaturalFrequencyHz evaluates the membrane/plate dispersion relation.
func NaturalFrequencyHz(head Head, besselZero float64) float64 {
	wavenumber := besselZero / head.RadiusM

	return naturalAngularFrequency(head, wavenumber) / (2 * math.Pi)
}

func naturalAngularFrequency(head Head, wavenumber float64) float64 {
	wavenumberSquared := wavenumber * wavenumber
	angularFreqSquared := head.TensionNPerM/head.SurfaceDensityKgPerM2*wavenumberSquared +
		head.BendingStiffnessNM/head.SurfaceDensityKgPerM2*wavenumberSquared*wavenumberSquared

	return math.Sqrt(angularFreqSquared)
}

func buildMode(
	config PhysicalDrum,
	head Head,
	candidate eigenmode,
	orientation Orientation,
) (Mode, error) {
	azimuthalOrder := candidate.azimuthalOrder
	angularFrequency := splitAngularFrequency(
		candidate.angularFreq,
		azimuthalOrder,
		orientation,
		head.TensionAsymmetry.SplitRatio,
	)

	angularIntegral := math.Pi
	if azimuthalOrder == 0 {
		angularIntegral = 2 * math.Pi
	}

	boundarySlope := math.Jn(azimuthalOrder+1, candidate.besselZero)
	modalMass := head.SurfaceDensityKgPerM2 * angularIntegral * head.RadiusM * head.RadiusM *
		boundarySlope * boundarySlope / 2

	strikeShape := modeShape(
		azimuthalOrder,
		candidate.besselZero,
		orientation,
		config.Strike.Radius01,
		config.Strike.AngleRad-head.TensionAsymmetry.PrincipalAxisAngleRad,
	)
	footprint := circularFootprint(
		candidate.wavenumber * config.Strike.ContactRadiusM,
	)
	pickupShape := modeShape(
		azimuthalOrder,
		candidate.besselZero,
		orientation,
		config.Pickup.Radius01,
		config.Pickup.AngleRad-head.TensionAsymmetry.PrincipalAxisAngleRad,
	)
	radiationAmplitude := modalRadiationAmplitude(
		angularFrequency,
		azimuthalOrder,
		head.RadiusM,
		config.Cavity.SoundSpeedMPerS,
	)
	distanceGain := 1 / (1 + config.Pickup.DistanceM/head.RadiusM)
	structuralDecay := ModalDecayRatePerSecond(head, candidate.wavenumber)
	radiationDecay := head.RadiationLossPerSecond *
		radiationAmplitude * radiationAmplitude
	decayCorrection := modeDecayCorrection(
		head,
		candidate.azimuthalOrder,
		candidate.radialOrder,
	)

	decayRate := structuralDecay + radiationDecay + decayCorrection
	if decayRate < 0 {
		return Mode{}, fmt.Errorf(
			"%w: mode (%d,%d) decay rate %v is negative",
			ErrInvalidConfig,
			candidate.azimuthalOrder,
			candidate.radialOrder,
			decayRate,
		)
	}

	sweptArea := 0.0
	if azimuthalOrder == 0 {
		sweptArea = 2 * math.Pi * head.RadiusM * head.RadiusM *
			math.J1(candidate.besselZero) / candidate.besselZero
	}

	return Mode{
		AzimuthalOrder:           azimuthalOrder,
		RadialOrder:              candidate.radialOrder,
		Orientation:              orientation,
		BesselZero:               candidate.besselZero,
		WavenumberPerM:           candidate.wavenumber,
		FrequencyHz:              angularFrequency / (2 * math.Pi),
		AngularFrequency:         angularFrequency,
		StructuralDecayPerSecond: structuralDecay,
		RadiationDecayPerSecond:  radiationDecay,
		DecayCorrectionPerSecond: decayCorrection,
		DecayRatePerSecond:       decayRate,
		ModalMassKg:              modalMass,
		StrikeAccelerationPerN:   strikeShape * footprint / modalMass,
		PickupShape:              pickupShape,
		RadiationWeight:          pickupShape * radiationAmplitude * distanceGain,
		SweptAreaM2:              sweptArea,
	}, nil
}

func splitAngularFrequency(
	idealAngularFrequency float64,
	azimuthalOrder int,
	orientation Orientation,
	splitRatio float64,
) float64 {
	if azimuthalOrder == 0 || splitRatio == 0 {
		return idealAngularFrequency
	}

	multiplier := 1 - splitRatio/2
	if orientation == OrientationSine {
		multiplier = 1 + splitRatio/2
	}

	return idealAngularFrequency * multiplier
}

// ModalDecayRatePerSecond evaluates the two-parameter structural loss law
// d(k) = d0 + d2*k². Radiation loss and optional measured residuals remain
// separate in Mode so calibration can distinguish their causes.
func ModalDecayRatePerSecond(head Head, wavenumberPerM float64) float64 {
	return head.Loss0PerSecond +
		head.Loss2M2PerSecond*wavenumberPerM*wavenumberPerM
}

func modalRadiationAmplitude(
	angularFrequency float64,
	azimuthalOrder int,
	radiusM, soundSpeedMPerS float64,
) float64 {
	ka := angularFrequency * radiusM / soundSpeedMPerS
	base := ka / math.Sqrt(1+ka*ka)

	// Every azimuthal order adds one multipole cancellation. This tends to
	// zero for acoustically small heads and to one as ka grows.
	return math.Pow(base, float64(azimuthalOrder+1))
}

func modeDecayCorrection(head Head, azimuthalOrder, radialOrder int) float64 {
	for _, correction := range head.ModeDecayCorrections {
		if correction.AzimuthalOrder == azimuthalOrder &&
			correction.RadialOrder == radialOrder {
			return correction.DecayRatePerSecond
		}
	}

	return 0
}

// circularFootprint is the spatial low-pass response of a uniformly loaded
// circular contact patch, locally approximating the curved mode as a plane
// wave. Its limit at zero is one.
func circularFootprint(wavenumberRadius float64) float64 {
	if math.Abs(wavenumberRadius) < 1e-8 {
		return 1
	}

	return 2 * math.J1(wavenumberRadius) / wavenumberRadius
}

func modeShape(
	azimuthalOrder int,
	besselZero float64,
	orientation Orientation,
	radius01, angleRad float64,
) float64 {
	radialShape := math.Jn(azimuthalOrder, besselZero*radius01)

	angle := float64(azimuthalOrder) * angleRad
	if orientation == OrientationSine {
		return radialShape * math.Sin(angle)
	}

	return radialShape * math.Cos(angle)
}

func besselZeros(order, count int) []float64 {
	zeros := make([]float64, 0, count)
	left := 1e-8
	leftValue := math.Jn(order, left)

	for right := left + rootScanStep; len(zeros) < count; right += rootScanStep {
		rightValue := math.Jn(order, right)
		if leftValue*rightValue < 0 {
			zeros = append(zeros, bisectBesselRoot(order, left, right, leftValue))
		}

		left = right
		leftValue = rightValue
	}

	return zeros
}

func bisectBesselRoot(order int, left, right, leftValue float64) float64 {
	for range 64 {
		middle := 0.5 * (left + right)

		middleValue := math.Jn(order, middle)
		if leftValue*middleValue <= 0 {
			right = middle
			continue
		}

		left = middle
		leftValue = middleValue
	}

	return 0.5 * (left + right)
}
