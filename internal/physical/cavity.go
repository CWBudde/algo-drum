package physical

import (
	"fmt"
	"math"
	"sort"
	"sync"
)

const (
	// maxCavityModes caps the number of enclosed-air pressure states. The
	// midpoint solve eliminates them through a k x k dense system once per
	// nonlinear iteration per sample, so k is quadratic in the audio path and
	// cannot be left open. Eight covers the uniform mode plus the first two
	// transverse azimuthal families and the first radial overtone of the
	// axisymmetric one, which is every mode below about 1.5 kHz on a 12-inch
	// shell — past that the head's own modal thicket is denser than the air's.
	maxCavityModes = 8
	// Enumeration bounds for the candidate list. They only have to be wide
	// enough that maxCavityModes slots always fill from below; j'_mn grows with
	// both indices, so order 8 / radial 4 reaches far past the eighth mode.
	maxCavityAzimuthalOrder = 8
	maxCavityRadialOrder    = 4
	// Gauss-Legendre nodes for the radial overlap integral. The integrand is
	// J_m(z r/R)J_m(j' r/R)r with z below 21 and j' below 8 on any admissible
	// configuration, so it carries fewer than ten oscillations over the disc;
	// 96 nodes integrate that to roundoff and are paid once per mode pair at
	// construction time, never in Render.
	cavityOverlapNodes = 96
)

// CavityMode describes one rigid-walled cylindrical air mode of the enclosed
// volume.
//
// The family is deliberately restricted to the axially uniform modes,
// l = 0. Including axial order would mean pressure varying along the shell, and
// then the two heads no longer see the same pressure: the coupling coefficient
// would acquire a cos(l*pi*z/L) factor that is +1 at the batter and (-1)^l at the
// resonant head, which is a different and larger change than this one. The first
// axial mode also sits at c/2L = 858 Hz for the shipped 0.2 m depth, above the
// two transverse modes this exists to add. Axially uniform only is therefore a
// first cut rather than the complete cavity, and the uniform member of the family
// is exactly the single lumped compliance the model had before.
//
// Shapes are Psi_mn(r,theta) = J_m(j'_mn r/a) * {cos m*theta, sin m*theta}, where
// j'_mn is the n-th zero of J_m' — the derivative, because a rigid wall carries
// zero normal velocity and so imposes a Neumann condition. That is not the
// condition the heads obey: their J_m(z_mn) = 0 is a clamped edge. The uniform
// mode is the m = 0, j' = 0 member and is written here with RadialOrder 0 to keep
// it distinct from (0,1), whose zero is the first *positive* root of J_0' at
// 3.8317.
type CavityMode struct {
	AzimuthalOrder   int
	RadialOrder      int
	Orientation      Orientation
	NeumannZero      float64
	FrequencyHz      float64
	AngularFrequency float64
	// VolumeNormM3 is Lambda_c = integral of Psi_c^2 over the cavity volume.
	VolumeNormM3 float64
	// StiffnessPaPerM3 is K_c = s*rho*c^2/Lambda_c, the modal air spring. For
	// the uniform mode Lambda_c is the cavity volume and this is exactly the
	// rho*c^2/V the lumped model always used.
	StiffnessPaPerM3 float64
}

// IsUniform reports the single mode that reproduces the lumped compliance.
func (c CavityMode) IsUniform() bool {
	return c.AzimuthalOrder == 0 && c.RadialOrder == 0
}

// GenerateCavityModes builds the enclosed-air modal basis in frequency order.
//
// Selection mirrors the head banks: candidates are ordered by frequency and
// admitted until the configured count is reached, an m > 0 mode costing two slots
// for its cosine and sine members. Cavity.ModeCount of 1 admits the uniform mode
// alone, which is the pre-transverse model exactly.
func GenerateCavityModes(config PhysicalDrum) ([]CavityMode, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return generateCavityModes(config)
}

func generateCavityModes(config PhysicalDrum) ([]CavityMode, error) {
	radiusM := config.Batter.RadiusM
	volumeM3 := math.Pi * radiusM * radiusM * config.Cavity.DepthM

	stiffnessScale := 0.0
	if config.Cavity.Enabled {
		stiffnessScale = config.Cavity.StiffnessScale
	}

	// Written in exactly the order the lumped model always used it, so a
	// one-mode cavity reproduces its coefficient bit for bit.
	uniformStiffness := stiffnessScale *
		config.Cavity.AirDensityKgPerM3 *
		config.Cavity.SoundSpeedMPerS *
		config.Cavity.SoundSpeedMPerS /
		volumeM3

	modes := make([]CavityMode, 0, maxCavityModes)
	modes = append(modes, CavityMode{
		VolumeNormM3:     volumeM3,
		StiffnessPaPerM3: uniformStiffness,
	})

	candidates := cavityCandidates()

	waveNumberScale := config.Cavity.SoundSpeedMPerS / radiusM

	for _, entry := range candidates {
		orientations := [...]Orientation{OrientationCosine, OrientationSine}

		orientationCount := len(orientations)
		if entry.azimuthalOrder == 0 {
			orientationCount = 1
		}

		if len(modes)+orientationCount > config.Cavity.ModeCount {
			break
		}

		angularFrequency := waveNumberScale * entry.neumannZero

		norm := cavityVolumeNormM3(
			entry.azimuthalOrder,
			entry.neumannZero,
			radiusM,
			config.Cavity.DepthM,
		)
		for _, orientation := range orientations[:orientationCount] {
			modes = append(modes, CavityMode{
				AzimuthalOrder:   entry.azimuthalOrder,
				RadialOrder:      entry.radialOrder,
				Orientation:      orientation,
				NeumannZero:      entry.neumannZero,
				FrequencyHz:      angularFrequency / (2 * math.Pi),
				AngularFrequency: angularFrequency,
				VolumeNormM3:     norm,
				StiffnessPaPerM3: stiffnessScale *
					config.Cavity.AirDensityKgPerM3 *
					config.Cavity.SoundSpeedMPerS *
					config.Cavity.SoundSpeedMPerS / norm,
			})
		}

		if len(modes) == config.Cavity.ModeCount {
			break
		}
	}

	return modes, nil
}

type cavityCandidate struct {
	azimuthalOrder int
	radialOrder    int
	neumannZero    float64
}

// cavityCandidates is the frequency-ordered (m,n) list every configuration
// selects from. It depends on nothing a caller passes, and Validate builds a
// cavity basis on every call — including once per candidate the offline fitter
// evaluates — so it is built once per process rather than sorted afresh each
// time.
var cavityCandidates = sync.OnceValue(func() []cavityCandidate {
	candidates := make(
		[]cavityCandidate,
		0,
		(maxCavityAzimuthalOrder+1)*maxCavityRadialOrder,
	)
	for order := 0; order <= maxCavityAzimuthalOrder; order++ {
		zeros, err := neumannZeros(order, maxCavityRadialOrder)
		if err != nil {
			continue
		}

		for offset, zero := range zeros {
			candidates = append(candidates, cavityCandidate{
				azimuthalOrder: order,
				radialOrder:    offset + 1,
				neumannZero:    zero,
			})
		}
	}

	sort.Slice(candidates, func(first, second int) bool {
		if candidates[first].neumannZero != candidates[second].neumannZero {
			return candidates[first].neumannZero < candidates[second].neumannZero
		}

		return candidates[first].azimuthalOrder < candidates[second].azimuthalOrder
	})

	return candidates
})

// cavityVolumeNormM3 evaluates Lambda_c = integral of Psi_c^2 over the volume.
//
// With the axially uniform family the axial factor is the depth, the azimuthal
// factor is pi (2*pi for m = 0), and the radial factor is the standard Neumann
// normalization
//
//	integral_0^a J_m(j' r/a)^2 r dr = (a^2/2)(1 - m^2/j'^2) J_m(j')^2,
//
// which follows from the general Lommel result once J_m'(j') = 0 removes its
// derivative term. It is positive for every admissible mode because j'_mn > m.
func cavityVolumeNormM3(azimuthalOrder int, neumannZero, radiusM, depthM float64) float64 {
	angularIntegral := math.Pi
	if azimuthalOrder == 0 {
		angularIntegral = 2 * math.Pi
	}

	shapeValue := math.Jn(azimuthalOrder, neumannZero)
	radialFactor := radiusM * radiusM / 2 * shapeValue * shapeValue

	if azimuthalOrder > 0 {
		order := float64(azimuthalOrder)
		radialFactor *= 1 - order*order/(neumannZero*neumannZero)
	}

	return depthM * angularIntegral * radialFactor
}

// HeadCavityCouplingM2 is the overlap integral of one head mode shape against one
// cavity mode shape over the head's disc,
//
//	C_ic = integral phi_i Psi_c dA,
//
// which is the coefficient that appears symmetrically in both directions of the
// coupling: the cavity mode is driven by C_ic*qdot_i and the head mode is loaded
// by C_ic*P_c. That symmetry is what makes the coupled system passive.
//
// The azimuthal integral separates and gives a selection rule: it vanishes unless
// the azimuthal orders match, and for matching orders it is a rotation through
// m*psi, where psi is the head's principal tension axis. At psi = 0 — every
// shipped configuration — that rotation is the identity and orientations must
// match too, so each head mode couples to at most one cavity mode per radial
// order.
//
// The uniform cavity mode is returned as the mode's own analytic swept area
// rather than through the quadrature below. The two agree to roundoff, and the
// test suite checks that they do; using the analytic value keeps a one-mode
// cavity numerically identical to the model this replaced.
func HeadCavityCouplingM2(
	head Head,
	shellRadiusM float64,
	mode Mode,
	cavity CavityMode,
) float64 {
	if mode.AzimuthalOrder != cavity.AzimuthalOrder {
		return 0
	}

	azimuthal := azimuthalOverlap(
		mode.AzimuthalOrder,
		mode.Orientation,
		cavity.Orientation,
		head.TensionAsymmetry.PrincipalAxisAngleRad,
	)
	if azimuthal == 0 {
		return 0
	}

	if cavity.IsUniform() {
		return mode.SweptAreaM2
	}

	return azimuthal * radialOverlapM2(
		mode.AzimuthalOrder,
		mode.BesselZero,
		cavity.NeumannZero*head.RadiusM/shellRadiusM,
		head.RadiusM,
	)
}

// azimuthalOverlap evaluates the angular factor of the overlap integral,
//
//	integral_0^2pi Theta_o(m(theta - psi)) Theta_p(m theta) d theta,
//
// where Theta is cosine or sine according to the orientation. Expanding the
// shifted argument turns it into pi times a plane rotation through m*psi, and
// into 2*pi for m = 0 where only the cosine member exists.
func azimuthalOverlap(
	azimuthalOrder int,
	headOrientation, cavityOrientation Orientation,
	principalAxisAngleRad float64,
) float64 {
	if azimuthalOrder == 0 {
		return 2 * math.Pi
	}

	angle := float64(azimuthalOrder) * principalAxisAngleRad

	switch {
	case headOrientation == OrientationCosine && cavityOrientation == OrientationCosine:
		return math.Pi * math.Cos(angle)
	case headOrientation == OrientationCosine && cavityOrientation == OrientationSine:
		return math.Pi * math.Sin(angle)
	case headOrientation == OrientationSine && cavityOrientation == OrientationCosine:
		return -math.Pi * math.Sin(angle)
	default:
		return math.Pi * math.Cos(angle)
	}
}

// radialOverlapM2 evaluates
//
//	integral_0^R J_m(z r/R) J_m(j' r/R) r dr
//
// by Gauss-Legendre quadrature. Unlike the swept area — where J_m(z) = 0 collapses
// the Lommel integral to a closed form — the two Bessel functions here have
// different arguments and different boundary conditions, so no clean closed form
// exists. The integral is evaluated once per coupled mode pair when the model is
// built and stored; nothing in Render touches it.
func radialOverlapM2(azimuthalOrder int, headZero, cavityZero, radiusM float64) float64 {
	rule := gaussLegendreUnit()

	sum := 0.0
	for index, node := range rule.nodes {
		sum += rule.weights[index] * node *
			math.Jn(azimuthalOrder, headZero*node) *
			math.Jn(azimuthalOrder, cavityZero*node)
	}

	return radiusM * radiusM * sum
}

type quadratureRule struct {
	nodes   []float64
	weights []float64
}

// gaussLegendreUnit builds the cavityOverlapNodes-point Gauss-Legendre rule
// mapped onto [0,1], by Newton iteration on the Legendre polynomial from the
// standard Chebyshev-like starting guess.
var gaussLegendreUnit = sync.OnceValue(func() quadratureRule {
	const count = cavityOverlapNodes

	rule := quadratureRule{
		nodes:   make([]float64, count),
		weights: make([]float64, count),
	}
	for index := range count {
		abscissa := math.Cos(math.Pi * (float64(index) + 0.75) / (float64(count) + 0.5))

		derivative := 0.0

		for range 100 {
			value, previous := 1.0, 0.0
			for degree := range count {
				value, previous = ((2*float64(degree)+1)*abscissa*value-
					float64(degree)*previous)/(float64(degree)+1), value
			}

			derivative = float64(count) * (abscissa*value - previous) /
				(abscissa*abscissa - 1)

			step := value / derivative

			abscissa -= step
			if math.Abs(step) < 1e-15 {
				break
			}
		}

		// The rule on [-1,1] has weight 2/((1-x^2)P'^2); halving it maps the
		// rule onto [0,1] together with the node.
		rule.nodes[index] = 0.5 * (abscissa + 1)
		rule.weights[index] = 1 /
			((1 - abscissa*abscissa) * derivative * derivative)
	}

	return rule
})

// neumannZeros returns the first count positive zeros of J_order'.
//
// For order 0 the derivative is -J_1, so its positive zeros are the zeros of J_1
// and the table the head banks already build supplies them. For higher orders the
// derivative is (J_{m-1} - J_{m+1})/2 and the zeros are found the same way the
// head's are: scan for a sign change, then bisect.
func neumannZeros(order, count int) ([]float64, error) {
	if order < 0 || order > maxCavityAzimuthalOrder || count < 1 {
		return nil, fmt.Errorf("%w: neumann order=%d count=%d",
			ErrInvalidModeIndex, order, count)
	}

	if order == 0 {
		zeros := besselZeros(1, count)

		return append([]float64(nil), zeros[:count]...), nil
	}

	return besselDerivativeZeroTable()[order][:count:count], nil
}

var besselDerivativeZeroTable = sync.OnceValue(func() [][]float64 {
	table := make([][]float64, maxCavityAzimuthalOrder+1)
	for order := 1; order < len(table); order++ {
		table[order] = computeBesselDerivativeZeros(order, maxCavityRadialOrder)
	}

	return table
})

func besselDerivative(order int, argument float64) float64 {
	return 0.5 * (math.Jn(order-1, argument) - math.Jn(order+1, argument))
}

func computeBesselDerivativeZeros(order, count int) []float64 {
	zeros := make([]float64, 0, count)
	left := 1e-8
	leftValue := besselDerivative(order, left)

	for right := left + rootScanStep; len(zeros) < count; right += rootScanStep {
		rightValue := besselDerivative(order, right)
		if leftValue*rightValue < 0 {
			zeros = append(
				zeros,
				bisectBesselDerivativeRoot(order, left, right, leftValue),
			)
		}

		left = right
		leftValue = rightValue
	}

	return zeros
}

func bisectBesselDerivativeRoot(order int, left, right, leftValue float64) float64 {
	for range 64 {
		middle := 0.5 * (left + right)

		middleValue := besselDerivative(order, middle)
		if leftValue*middleValue <= 0 {
			right = middle
			continue
		}

		left = middle
		leftValue = middleValue
	}

	return 0.5 * (left + right)
}

// cavityAzimuthalOrders reports which head azimuthal orders the enclosed air can
// reach at all. A head mode whose order appears in no cavity mode has an
// identically zero coupling coefficient by the selection rule above, so nothing
// can drive it through the air.
func cavityAzimuthalOrders(config PhysicalDrum) map[int]struct{} {
	orders := map[int]struct{}{0: {}}
	if !config.Cavity.Enabled || config.Cavity.Coupling01 == 0 {
		return orders
	}

	modes, err := generateCavityModes(config)
	if err != nil {
		return orders
	}

	for _, mode := range modes {
		orders[mode.AzimuthalOrder] = struct{}{}
	}

	return orders
}
