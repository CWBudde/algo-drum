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

	if head.AxisymmetricOnly {
		modes = retainAxisymmetric(modes)
	}

	return modes, nil
}

// retainAxisymmetric drops every m > 0 mode, after selection rather than during
// it.
//
// The selection loop above fills a slot budget, so skipping these candidates
// inside it would free their slots and the loop would keep walking up the
// frequency-sorted list admitting higher-order *axisymmetric* modes instead —
// modes with non-zero swept area, which drive the cavity. That is a different
// instrument, not a cheaper one. Filtering afterwards reproduces exactly the
// subset the unfiltered selection would have produced, at any budget.
func retainAxisymmetric(modes []Mode) []Mode {
	retained := modes[:0]
	for _, mode := range modes {
		if mode.AzimuthalOrder == 0 {
			retained = append(retained, mode)
		}
	}

	return retained
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
	// The observation direction, and with it how much non-axisymmetric content
	// reaches the microphone, follows from the microphone geometry already in
	// the configuration.
	traceArgument := angularFrequency * head.RadiusM /
		config.Cavity.SoundSpeedMPerS *
		observationPolarSine(config.Pickup, head.RadiusM)
	radiatingMoment := radiatingMomentM2(
		azimuthalOrder,
		candidate.besselZero,
		boundarySlope,
		head.RadiusM,
		traceArgument,
	)
	directivity := azimuthalDirectivity(
		azimuthalOrder,
		orientation,
		config.Pickup.AngleRad-head.TensionAsymmetry.PrincipalAxisAngleRad,
	)
	distanceGain := 1 / (1 + config.Pickup.DistanceM/head.RadiusM)
	nearFieldWeight := evanescentNearFieldM2(
		config.Pickup,
		head.RadiusM,
		candidate.besselZero,
		pickupShape,
	)
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
		RadiationWeight: radiatingMoment*directivity*distanceGain +
			nearFieldWeight,
		RadiatingMomentM2:    radiatingMoment,
		RadiationDirectivity: directivity,
		SweptAreaM2:          sweptArea,
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

// ModalDecayRatePerSecond evaluates the structural loss law
// d(k) = d0 + d1*k + d2*k². Radiation loss and optional measured residuals
// remain separate in Mode so calibration can distinguish their causes.
//
// The k¹ term carries constant Q. Without it the law cannot express a fixed
// fraction of critical damping at all: d0 alone gives T60 independent of
// frequency and d2 alone gives T60 ∝ 1/f², where the measured membrane
// behaviour is T60 ∝ 1/f.
func ModalDecayRatePerSecond(head Head, wavenumberPerM float64) float64 {
	return head.Loss0PerSecond +
		head.Loss1MPerSecond*wavenumberPerM +
		head.Loss2M2PerSecond*wavenumberPerM*wavenumberPerM
}

// modalRadiationAmplitude is a radiation *efficiency*, used only to apportion
// radiation damping across the mode series. It is deliberately not the output
// weight: as an amplitude ratio against modal velocity it already contains one
// factor of ka, so reusing it beside the volume-acceleration observable would
// count that factor twice, and raising it to m+1 stands in for a multipole
// cancellation whose true magnitude is 1/(2^m m!) — off by seven orders at the
// highest retained azimuthal order. radiatingMomentM2 is the output weight.
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

// observationPolarSine is the sine of the microphone's polar angle from the
// head axis, from its projected radius and its distance.
func observationPolarSine(pickup Pickup, radiusM float64) float64 {
	offsetM := pickup.Radius01 * radiusM

	return offsetM / math.Hypot(offsetM, pickup.DistanceM)
}

// radiatingMomentM2 is the exact far-field geometric factor of one circular
// mode: the Rayleigh integral of its shape against the observation direction,
//
//	G = 2*pi*R^2 * z * J_{m+1}(z) * J_m(u) / (z^2 - u^2),
//
// where u = k_air*R*sin(theta) is the acoustic trace wavenumber across the
// head. It is a Lommel integral, and because J_m(z_mn) = 0 by construction it
// collapses to this closed form with no series left over. Two properties carry
// the physics:
//
//   - at u = 0 it is exactly the swept area 2*pi*R^2*J_1(z)/z for m = 0 and
//     exactly zero for m > 0, so an on-axis microphone hears the axisymmetric
//     modes and nothing else — which is the correct and measurable result;
//   - J_m(u) ~ (u/2)^m/m! for small u, so multipole cancellation comes out of
//     the integral rather than being approximated by a rolloff exponent.
//
// Far-field pressure from a compact source is proportional to volume
// *acceleration* with no further frequency dependence, so this factor is the
// whole weight. The approximation is a compact one and is good to ka of about
// 3, which covers the retained band.
func radiatingMomentM2(
	azimuthalOrder int,
	besselZero, boundarySlope, radiusM, traceArgument float64,
) float64 {
	area := 2 * math.Pi * radiusM * radiusM

	// A membrane carries waves far slower than air does, so u/z is bounded by
	// c_membrane/c_air — about 0.12 here — and the denominator cannot vanish
	// for any real head. The coincidence limit is kept so it stays finite
	// anyway rather than dividing by something unchecked.
	separation := besselZero*besselZero - traceArgument*traceArgument
	if math.Abs(separation) < 1e-9*besselZero*besselZero {
		return 0.5 * area * boundarySlope * boundarySlope
	}

	return area * besselZero * boundarySlope *
		math.Jn(azimuthalOrder, traceArgument) / separation
}

// evanescentNearFieldM2 is the second of the two terms that make up a
// microphone weight, and the one a close-miked drum is mostly made of.
//
// radiatingMomentM2 is a far-field quantity, and in the far field a 12-inch head
// below about 600 Hz is very nearly a monopole: measured on the shipped basis,
// multipole cancellation leaves every m > 0 mode at least 22 dB down even with
// the microphone against the head. A real tom mic sits at d/a of about a third,
// which is not the far field at all, and what it picks up there is the
// non-propagating part of the field — the part that never radiates.
//
// For a structural wave slower than sound that part is evanescent, decaying as
// exp(-k*d) with the mode's own structural wavenumber, so higher modes fade out
// of it faster than low ones. Its shape at the microphone is the mode shape,
// which is what PickupShape has always been and is the right object here — as
// an additive term, not as a factor on a far-field efficiency, which is how the
// two used to be conflated.
//
// NearFieldScale is fitted rather than derived: the effective area of an
// evanescent patch is not something this reduced model can compute. The exp()
// carries the whole distance law of this term, so distanceGain — the geometric
// spreading of the propagating part — deliberately does not appear.
func evanescentNearFieldM2(
	pickup Pickup,
	radiusM, besselZero, pickupShape float64,
) float64 {
	if pickup.NearFieldScale == 0 {
		return 0
	}

	return pickup.NearFieldScale * 2 * math.Pi * radiusM * radiusM *
		math.Exp(-besselZero*pickup.DistanceM/radiusM) * pickupShape
}

// azimuthalDirectivity is one mode's far-field azimuthal pattern. It depends on
// the microphone's angle and never on its radius; the polar dependence lives in
// radiatingMomentM2's trace argument. Axisymmetric modes are always the cosine
// member, so this is 1 for them without a special case.
func azimuthalDirectivity(
	azimuthalOrder int,
	orientation Orientation,
	angleRad float64,
) float64 {
	angle := float64(azimuthalOrder) * angleRad
	if orientation == OrientationSine {
		return math.Sin(angle)
	}

	return math.Cos(angle)
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

	return radialShape *
		azimuthalDirectivity(azimuthalOrder, orientation, angleRad)
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
