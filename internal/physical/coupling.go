package physical

import (
	"math"
	"sort"
)

// couplingQuadratureNodes is the Gauss-Legendre order of the radial quadrature
// every coupling coefficient is built from.
//
// The angular integral is closed form (see pairAngularComponents), so each
// coefficient reduces to one 1-D integral of a product of four Bessel factors
// over (0,R]. The integrand is smooth and oscillates at most 4*z_max/R times
// across the disc — about 130 half-waves at the shipped bank's top mode — and
// 200 Gauss-Legendre nodes resolve that with room to spare. It is checked rather
// than assumed: TestCouplingUniformChannelReproducesBergerStrain measures the
// c = 0 channel against the analytic Gamma_i and holds it to 1e-10 relative.
const couplingQuadratureNodes = 200

const (
	// maxCouplingPumps caps |P|. The channel count is 1 + |P|(|P|+1)/2, so eight
	// pumps is 37 channels and the build cost grows as its square.
	maxCouplingPumps = 8
	// maxCouplingCoefficients caps the retained table. The audio path walks it
	// once per nonlinear iteration per sample, so this is the cost knob.
	maxCouplingCoefficients = 4096
)

// maxCouplingCoefficientNPerM caps beta_tilde. It is measured, not derived, and
// the comment on couplingResidualGrowth says why nothing better is available:
// the fixed point contracts while beta_tilde h^2 rho(G) < 2 with G quadratic in
// the modal state, so the true ceiling moves with the strike and no function of
// the configuration alone can express it.
//
// What was measured is the largest coefficient whose one-second velocity-1
// render stays finite, bisected over the validated configuration space before
// the divergence guard existed:
//
//	quality high, 48 kHz     6.98e8   <- worst
//	pumps 8, 4096 coeffs     9.90e8
//	pumps 8, 96 kHz          3.16e9
//	shipped default, 48 kHz  5.59e9
//	44.1 kHz                 5.71e9
//	quality draft            2.30e10
//	96 kHz                   8.91e10
//	192 kHz                  3.49e11
//	default at velocity 0.1  7.29e11
//
// The scaling is the derivation's: halving the step raises the threshold about
// as h^-2, and dropping the strike velocity tenfold raises it about a hundredfold
// (7.29e11/5.59e9 = 130 against the |q|^-2 prediction of 100). Two of those rows
// sit below the 1e9 this used to allow, which is how a validated document came to
// render 52754 non-finite samples.
//
// 1e8 is a seventh of the worst row. That is a wider margin than
// MaximumTensionRatio's 0.2 against 0.2346, deliberately: that bound is derived
// and exact, this one is a bisection over a finite sweep, and the quantity it
// bounds is the one thing here that is not a function of the configuration. It
// leaves the shipped 7.0e5 a factor of 143.
const maxCouplingCoefficientNPerM = 1e8

// couplingResidualGrowth is how much the coupled fixed point's residual is
// allowed to grow from one iteration to the next before the step is abandoned
// and re-solved without the coupling.
//
// This exists because "passive" and "stable at this timestep" are different
// claims, and only the first one is unconditional here. U = (beta_tilde/4) sum_c
// g_c^2 is a sum of squares, so U >= 0 for any beta_tilde and the *continuous*
// potential is passive at every setting. What is conditional is the *discrete*
// scheme: linearising one sweep of the implicit-midpoint fixed point about the
// current iterate gives
//
//	dT'_c = -(beta_tilde h^2 / 2) sum_i (D^c q)_i (D^b q)_i / M_i  dT_b + O(h^3),
//
// because a change in the channel tensions moves the modal acceleration by
// -(1/M_i)(D^b q)_i, the midpoint solve turns that into a displacement change of
// h^2/2 times it (the denominator is 2/h to leading order), and the secant turns
// that back into a tension change of beta_tilde (D^c q)_i. So the iteration
// contracts while
//
//	beta_tilde h^2 rho(G) < 2,   G_cb = sum_i (D^c q)_i (D^b q)_i / M_i,
//
// and G is quadratic in the modal state. That is the whole difficulty: unlike the
// Berger law, whose tanh cap makes its stability independent of displacement
// magnitude (docs/physical-nonlinearity.md, "Bounds and alias control"), the
// channel tensions are uncapped and the bound moves with the strike. There is
// therefore no beta_tilde ceiling derivable from the configuration alone that
// holds at every amplitude — validateNonlinearCoupling can only carry a measured
// one with margin, and the solve has to be able to defend itself.
//
// The test is the residual, not the coefficient, so it needs no bound at all: a
// contraction's residual falls geometrically, so growth means the map is not a
// contraction at this state and its iterate carries no information. Measured
// across every quality tier, sample rate and velocity at coefficients from the
// shipped 7.0e5 to a hundred times it, the largest growth ratio a converging
// solve produces is 0.126 — it is a contraction with room to spare, not a
// marginal one — so 4 leaves a factor of 30 while still tripping on the
// configurations that used to render NaN.
const couplingResidualGrowth = 4

// couplingChannelFloor drops a Gram-Schmidt direction whose residual norm has
// collapsed relative to the raw basis function it came from. Such a direction is
// a linear combination of ones already retained and carries no new potential;
// keeping it would divide by a rounding error.
const couplingChannelFloor = 1e-8

// couplingEntryFloor is the relative magnitude below which a coefficient is
// treated as a structural zero rather than a small number. Coefficients killed
// by the selection rule land 12 or more orders below the largest retained one.
const couplingEntryFloor = 1e-9

// couplingKey names one angular harmonic of the disc: an order and the parity of
// the trigonometric function carrying it.
type couplingKey struct {
	order int
	sine  bool
}

// angularComponent is one harmonic of grad(phi_a) . grad(phi_b).
//
// The gradient product of two Fourier-Bessel modes is exactly two harmonics, at
// |m_a - m_b| and m_a + m_b, each a fixed combination of the two radial products
//
//	R1(r) = k_a k_b J'_{m_a}(k_a r) J'_{m_b}(k_b r)
//	R2(r) = m_a m_b J_{m_a}(k_a r) J_{m_b}(k_b r) / r^2.
//
// scale1 and scale2 are that combination; see pairAngularComponents.
type angularComponent struct {
	key    couplingKey
	scale1 float64
	scale2 float64
}

// couplingTable is the sparse channel decomposition of the local quartic
// membrane potential, minus the uniform channel the Berger law already carries.
//
// The potential is written as a sum of squares over orthonormal channels psi_c
// on the head,
//
//	g = |grad w|^2,   g_c = <g, psi_c> = q^T D^c q,
//	U = (beta_tilde/4) sum_c g_c^2,
//
// which is the form this model wants for three reasons. It is non-negative
// structurally, so passivity is not conditional on the coefficients. Its c = 0
// member, psi_0 = 1/sqrt(A), gives D^0 = diag(Gamma_i)/sqrt(A) and reproduces the
// shipped Berger law *exactly* rather than approximately — so this table holds
// only c >= 1 and adds to the existing law instead of replacing it. And because
// each U_c is a function of one scalar quadratic form, the scalar discrete
// gradient that already makes the Berger update energy-exact is also the vector
// discrete gradient of the coupled potential, with no Gonzalez projection and no
// 0/0 branch at rest on a 96-vector. See docs/physical-nonlinearity.md.
//
// Entries are stored once per unordered index pair with row <= column; the
// diagonal counts once and an off-diagonal counts twice, which is what
// couplingWeight applies.
type couplingTable struct {
	// coefficientNPerM is beta_tilde = beta * A, the local quartic's coefficient
	// in the same units the uniform channel's beta*A would carry.
	coefficientNPerM float64

	channelCount int
	// channelFirst has channelCount+1 entries; channel c owns
	// [channelFirst[c], channelFirst[c+1]).
	channelFirst []int32
	entryRow     []int32
	entryColumn  []int32
	entryValue   []float64
	// entryDoubledValue is entryValue with couplingWeight already applied, so
	// channelValuesAt needs neither the multiplicity branch nor a second load.
	entryDoubledValue []float64
	// runs partitions the entries into maximal stretches sharing a channel and a
	// row. Both audio-path loops are written over it rather than over the entries
	// directly: within a run the row is constant, so displacement[row],
	// couplingBar[row] and couplingInverseMass[row] hoist out of the inner loop,
	// and the row-side force accumulates in a register instead of a
	// read-modify-write of couplingAccel[row] that serialises the whole run on
	// store-to-load forwarding. Built by scanning the sorted entries, so it
	// carries no assumption about how they were ordered.
	runs []couplingRun

	// pumpIndices are the batter modes the channel set was built from, in
	// selection order. Diagnostics and tests read them; the audio path does not.
	pumpIndices []int
	// worstForceFrequencyHz is the highest frequency the retained cubic force can
	// place energy at, before the tension detune is applied. See
	// couplingWorstForceHz.
	worstForceFrequencyHz float64
	// droppedCoefficients counts entries the magnitude budget discarded.
	droppedCoefficients int
	// candidateCoefficients counts structurally non-zero entries before the
	// magnitude budget.
	candidateCoefficients int
}

// couplingRun is one maximal stretch of consecutive entries that share a channel
// and a row, as the (channel, row, column) sort order already groups them.
type couplingRun struct {
	first, last int32
	channel     int32
	row         int32
}

func (t *couplingTable) active() bool { return len(t.entryValue) > 0 }

// buildRuns derives the run partition and the pre-weighted coefficients from the
// entry arrays. Called once the entries are final; nothing here is on the audio
// path.
func (t *couplingTable) buildRuns() {
	t.entryDoubledValue = make([]float64, len(t.entryValue))
	for slot := range t.entryValue {
		t.entryDoubledValue[slot] = couplingWeight(t.entryRow[slot], t.entryColumn[slot]) *
			t.entryValue[slot]
	}

	t.runs = t.runs[:0]

	for channel := range t.channelCount {
		first := t.channelFirst[channel]
		last := t.channelFirst[channel+1]

		for start := first; start < last; {
			row := t.entryRow[start]

			end := start + 1
			for end < last && t.entryRow[end] == row {
				end++
			}

			t.runs = append(t.runs, couplingRun{
				first:   start,
				last:    end,
				channel: int32(channel),
				row:     row,
			})

			start = end
		}
	}
}

// couplingWeight is the multiplicity of a stored entry: a diagonal term appears
// once in q^T D q and an off-diagonal term twice.
func couplingWeight(row, column int32) float64 {
	if row == column {
		return 1
	}

	return 2
}

// pairAngularComponents returns the exact angular decomposition of
// grad(phi_a) . grad(phi_b).
//
// With phi = J_m(kr) C(theta), C being cos(m theta) or sin(m theta),
//
//	grad phi_a . grad phi_b = R1 C_a C_b + (J_a J_b / r^2) C_a' C_b',
//
// and the product-to-sum identities give, exactly,
//
//	cos,cos: (1/2)(R1+R2) cos(D t) + (1/2)(R1-R2) cos(S t)
//	sin,sin: (1/2)(R1+R2) cos(D t) - (1/2)(R1-R2) cos(S t)
//	cos,sin: -(1/2)(R1+R2) sin(D t) + (1/2)(R1-R2) sin(S t)
//
// with D = m_a - m_b and S = m_a + m_b. cos is even in D and sin is odd, hence
// the sign(D) factor below. When either order is zero, |D| = S and the two
// harmonics are the same one: they are summed rather than stored twice.
//
// The selection rule follows and is a property of this decomposition rather than
// a separate claim: a quartic coefficient vanishes unless the two gradient
// products share an angular order *and* a family. That second condition is the
// one that removes most of the tensor, and it is not the naive
// +/-m_i +/-m_j +/-m_k +/-m_l = 0 rule. There is no radial selection rule at all.
func pairAngularComponents(first, second *Mode) ([2]angularComponent, int) {
	azimuthalA := first.AzimuthalOrder
	azimuthalB := second.AzimuthalOrder
	difference := azimuthalA - azimuthalB
	sum := azimuthalA + azimuthalB

	absoluteDifference := difference
	if absoluteDifference < 0 {
		absoluteDifference = -absoluteDifference
	}

	var components [2]angularComponent

	if first.Orientation == second.Orientation {
		sign := 1.0
		if first.Orientation == OrientationSine {
			sign = -1
		}

		components[0] = angularComponent{
			key:    couplingKey{order: absoluteDifference},
			scale1: 0.5,
			scale2: 0.5,
		}
		components[1] = angularComponent{
			key:    couplingKey{order: sum},
			scale1: 0.5 * sign,
			scale2: -0.5 * sign,
		}
	} else {
		parity := 1.0
		if first.Orientation == OrientationSine {
			parity = -1
		}

		differenceSign := 0.0

		switch {
		case difference > 0:
			differenceSign = 1
		case difference < 0:
			differenceSign = -1
		}

		components[0] = angularComponent{
			key:    couplingKey{order: absoluteDifference, sine: true},
			scale1: -0.5 * parity * differenceSign,
			scale2: -0.5 * parity * differenceSign,
		}
		components[1] = angularComponent{
			key:    couplingKey{order: sum, sine: true},
			scale1: 0.5,
			scale2: -0.5,
		}
	}

	if absoluteDifference == sum {
		components[0] = angularComponent{
			key:    components[1].key,
			scale1: components[0].scale1 + components[1].scale1,
			scale2: components[0].scale2 + components[1].scale2,
		}

		return components, 1
	}

	return components, 2
}

// angularFactor is the integral of the squared harmonic over theta: 2*pi for the
// constant, pi for every other cosine or sine harmonic. A sine harmonic at order
// zero is identically zero and never reaches this.
func angularFactor(key couplingKey) float64 {
	if key.order == 0 {
		if key.sine {
			return 0
		}

		return 2 * math.Pi
	}

	return math.Pi
}

// couplingBuilder holds the radial quadrature and the per-mode Bessel tables the
// coefficients are assembled from. It is build-time only.
type couplingBuilder struct {
	radius  []float64
	measure []float64
	value   [][]float64
	deriv   [][]float64
	keys    []couplingKey
	keyOf   map[couplingKey]int
}

func newCouplingBuilder(modes []Mode, radiusM float64) *couplingBuilder {
	nodes, weights := gaussLegendreNodes(couplingQuadratureNodes)
	builder := &couplingBuilder{
		radius:  make([]float64, len(nodes)),
		measure: make([]float64, len(nodes)),
		value:   make([][]float64, len(modes)),
		deriv:   make([][]float64, len(modes)),
		keyOf:   make(map[couplingKey]int),
	}

	for index, node := range nodes {
		builder.radius[index] = 0.5 * radiusM * (node + 1)
		builder.measure[index] = 0.5 * radiusM * weights[index] *
			builder.radius[index]
	}

	for modeIndex := range modes {
		mode := &modes[modeIndex]
		values := make([]float64, len(nodes))
		derivs := make([]float64, len(nodes))
		order := mode.AzimuthalOrder

		for nodeIndex, radius := range builder.radius {
			argument := mode.WavenumberPerM * radius
			values[nodeIndex] = math.Jn(order, argument)
			derivs[nodeIndex] = mode.WavenumberPerM *
				besselDerivative(order, argument)
		}

		builder.value[modeIndex] = values
		builder.deriv[modeIndex] = derivs
	}

	return builder
}

func (b *couplingBuilder) keyIndex(key couplingKey) int {
	if index, ok := b.keyOf[key]; ok {
		return index
	}

	index := len(b.keys)
	b.keys = append(b.keys, key)
	b.keyOf[key] = index

	return index
}

// evaluate writes one angular component's radial function onto dst.
func (b *couplingBuilder) evaluate(
	dst []float64,
	modes []Mode,
	first, second int,
	component angularComponent,
) {
	orderProduct := float64(
		modes[first].AzimuthalOrder * modes[second].AzimuthalOrder,
	)
	valueFirst := b.value[first]
	valueSecond := b.value[second]
	derivFirst := b.deriv[first]
	derivSecond := b.deriv[second]

	for index, radius := range b.radius {
		radial := component.scale1 * derivFirst[index] * derivSecond[index]
		if orderProduct != 0 && component.scale2 != 0 {
			radial += component.scale2 * orderProduct *
				valueFirst[index] * valueSecond[index] / (radius * radius)
		}

		dst[index] = radial
	}
}

// couplingFunction is a scalar field on the disc expanded in angular harmonics,
// stored as one radial sample vector per retained harmonic.
type couplingFunction struct {
	rows []float64 // keyCount * nodeCount
}

func newCouplingFunction(keyCount, nodeCount int) couplingFunction {
	return couplingFunction{rows: make([]float64, keyCount*nodeCount)}
}

func (b *couplingBuilder) inner(first, second couplingFunction) float64 {
	nodeCount := len(b.radius)
	total := 0.0

	for keyIndex, key := range b.keys {
		factor := angularFactor(key)
		if factor == 0 {
			continue
		}

		offset := keyIndex * nodeCount
		partial := 0.0

		for nodeIndex := range nodeCount {
			partial += first.rows[offset+nodeIndex] *
				second.rows[offset+nodeIndex] * b.measure[nodeIndex]
		}

		total += factor * partial
	}

	return total
}

// gaussLegendreNodes returns the n-point Gauss-Legendre rule on [-1,1].
func gaussLegendreNodes(count int) ([]float64, []float64) {
	nodes := make([]float64, count)
	weights := make([]float64, count)

	for index := range count {
		// Chebyshev start point, then Newton on the Legendre polynomial. The
		// rule is symmetric, so only the lower half is solved.
		guess := math.Cos(math.Pi * (float64(index) + 0.75) /
			(float64(count) + 0.5))

		var derivative float64

		for range 100 {
			// legendre is P_count(guess) and trailing is P_{count-1}(guess),
			// built by the three-term recurrence.
			legendre := 1.0
			trailing := 0.0

			for degree := 1; degree <= count; degree++ {
				older := trailing
				trailing = legendre
				legendre = (float64(2*degree-1)*guess*trailing -
					float64(degree-1)*older) / float64(degree)
			}

			derivative = float64(count) * (guess*legendre - trailing) /
				(guess*guess - 1)

			step := legendre / derivative
			guess -= step

			if math.Abs(step) < 1e-15 {
				break
			}
		}

		nodes[index] = guess
		weights[index] = 2 / ((1 - guess*guess) * derivative * derivative)
	}

	return nodes, weights
}

// referenceModalAmplitudes ranks batter modes by their peak displacement under a
// reference velocity-1 strike, in closed form.
//
// A mode driven by a force pulse of unit impulse and duration tau reaches a
// displacement amplitude of a_i * |H(omega_i tau)| / omega_i, where a_i is the
// mode's strike acceleration per newton and H is the normalised transform of the
// half-sine the prescribed contact lays down. That is the right weight for a
// *cubic* force, which goes as the third power of displacement rather than of
// energy, and it is not the frequency ordering: on the shipped bank the (1,2) at
// 437 Hz outranks both the (2,1) at 320 Hz and the (0,2) at 345 Hz, so a
// frequency-ordered pump set would retain the wrong modes.
func referenceModalAmplitudes(config PhysicalDrum, modes []Mode) []float64 {
	samples := contactSampleCount(
		config.SampleRateHz,
		config.Strike.Hardness01,
		1,
	)
	pulseSeconds := float64(samples) / config.SampleRateHz

	amplitudes := make([]float64, len(modes))
	for index := range modes {
		mode := &modes[index]
		if mode.AngularFrequency <= 0 {
			continue
		}

		amplitudes[index] = math.Abs(
			mode.StrikeAccelerationPerN*
				halfSineTransform(mode.AngularFrequency*pulseSeconds),
		) / mode.AngularFrequency
	}

	return amplitudes
}

// halfSineTransform is |F(omega)| / F(0) for one half-sine lobe of duration tau,
// evaluated at x = omega*tau. Its 0/0 point at x = pi has the limit pi/4.
func halfSineTransform(argument float64) float64 {
	ratio := argument / math.Pi

	denominator := 1 - ratio*ratio
	if math.Abs(denominator) < 1e-9 {
		return math.Pi / 4
	}

	return math.Cos(0.5*argument) / denominator
}

// selectPumpModes returns the pump set P: the loudest modes under the reference
// strike, restricted to the configured pump band. Ties break on frequency then
// index, so the set is a deterministic function of the configuration.
func selectPumpModes(
	config PhysicalDrum,
	modes []Mode,
	amplitudes []float64,
) []int {
	coupling := config.Nonlinearity.Coupling
	candidates := make([]int, 0, len(modes))

	for index := range modes {
		if modes[index].FrequencyHz > coupling.PumpMaxFrequencyHz {
			continue
		}

		if amplitudes[index] == 0 {
			continue
		}

		candidates = append(candidates, index)
	}

	sort.SliceStable(candidates, func(first, second int) bool {
		left := candidates[first]

		right := candidates[second]
		if amplitudes[left] != amplitudes[right] {
			return amplitudes[left] > amplitudes[right]
		}

		if modes[left].FrequencyHz != modes[right].FrequencyHz {
			return modes[left].FrequencyHz < modes[right].FrequencyHz
		}

		return left < right
	})

	if len(candidates) > coupling.PumpCount {
		candidates = candidates[:coupling.PumpCount]
	}

	return candidates
}

type couplingCandidate struct {
	channel    int32
	row        int32
	column     int32
	value      float64
	importance float64
}

// buildCouplingTable assembles the channel decomposition for one head's modal
// bank. It runs at Reconfigure time and allocates freely; nothing here is on the
// audio path.
//
// The table depends only on the retained (m, n, orientation) list and on R, so it
// is invariant under tension, surface density, bending stiffness, losses and
// RetuneTension — but it is rebuilt with the bank anyway, because
// NewDoubleHead is the only thing that builds one.
func buildCouplingTable(config PhysicalDrum, modes []Mode) couplingTable {
	coupling := config.Nonlinearity.Coupling
	if !config.Nonlinearity.Enabled || !coupling.Enabled ||
		coupling.CoefficientNPerM == 0 || coupling.MaxCoefficients == 0 ||
		len(modes) == 0 {
		return couplingTable{}
	}

	amplitudes := referenceModalAmplitudes(config, modes)

	pumps := selectPumpModes(config, modes, amplitudes)
	if len(pumps) < 2 {
		return couplingTable{}
	}

	builder := newCouplingBuilder(modes, config.Batter.RadiusM)

	// The raw basis: the constant channel first, so Gram-Schmidt makes every
	// other channel orthogonal to the one the Berger law already carries, then
	// every gradient product among the pumps.
	type basisPair struct{ first, second int }

	pairs := make([]basisPair, 0, len(pumps)*(len(pumps)+1)/2)
	for firstIndex, first := range pumps {
		for _, second := range pumps[firstIndex:] {
			pairs = append(pairs, basisPair{first: first, second: second})
		}
	}

	// Reserve the harmonics the basis functions live in. Target pairs outside
	// this key set contribute nothing, because every psi_c is zero there.
	builder.keyIndex(couplingKey{})

	for _, pair := range pairs {
		components, count := pairAngularComponents(
			&modes[pair.first],
			&modes[pair.second],
		)
		for _, component := range components[:count] {
			if angularFactor(component.key) == 0 {
				continue
			}

			builder.keyIndex(component.key)
		}
	}

	nodeCount := len(builder.radius)
	keyCount := len(builder.keys)
	scratch := make([]float64, nodeCount)

	// The uniform channel, psi_0 = 1/sqrt(A), written in the same
	// representation so one inner product serves everything.
	area := math.Pi * config.Batter.RadiusM * config.Batter.RadiusM

	uniform := newCouplingFunction(keyCount, nodeCount)
	for nodeIndex := range nodeCount {
		uniform.rows[nodeIndex] = 1 / math.Sqrt(area)
	}

	channels := make([]couplingFunction, 0, len(pairs)+1)
	channels = append(channels, uniform)

	for _, pair := range pairs {
		function := newCouplingFunction(keyCount, nodeCount)
		components, count := pairAngularComponents(
			&modes[pair.first],
			&modes[pair.second],
		)

		for _, component := range components[:count] {
			if angularFactor(component.key) == 0 {
				continue
			}

			builder.evaluate(scratch, modes, pair.first, pair.second, component)

			offset := builder.keyOf[component.key] * nodeCount
			for nodeIndex := range nodeCount {
				function.rows[offset+nodeIndex] += scratch[nodeIndex]
			}
		}

		rawNorm := math.Sqrt(builder.inner(function, function))
		if rawNorm == 0 {
			continue
		}

		for _, existing := range channels {
			projection := builder.inner(function, existing)
			for index := range function.rows {
				function.rows[index] -= projection * existing.rows[index]
			}
		}

		norm := math.Sqrt(builder.inner(function, function))
		if norm <= couplingChannelFloor*rawNorm {
			continue
		}

		for index := range function.rows {
			function.rows[index] /= norm
		}

		channels = append(channels, function)
	}

	// channels[0] is the uniform channel, which the Berger law owns. Everything
	// from here on is c >= 1.
	table := couplingTable{
		coefficientNPerM: coupling.CoefficientNPerM,
		channelCount:     len(channels) - 1,
		pumpIndices:      pumps,
	}
	if table.channelCount == 0 {
		return couplingTable{}
	}

	inPump := make([]bool, len(modes))
	for _, pump := range pumps {
		inPump[pump] = true
	}

	// Every retained entry has at least one index in P, so every retained
	// quartic term has at least two — which is the |P| >= 2 requirement made
	// structural: a term with a single pump index could only ever reach f and 3f.
	candidates := make([]couplingCandidate, 0, len(pumps)*len(modes)*table.channelCount)
	channelScale := make([]float64, table.channelCount)

	for row := range modes {
		for column := row; column < len(modes); column++ {
			if !inPump[row] && !inPump[column] {
				continue
			}

			components, count := pairAngularComponents(
				&modes[row],
				&modes[column],
			)

			for channelIndex := 1; channelIndex < len(channels); channelIndex++ {
				channel := channels[channelIndex]
				value := 0.0

				for _, component := range components[:count] {
					factor := angularFactor(component.key)
					if factor == 0 {
						continue
					}

					keyIndex, ok := builder.keyOf[component.key]
					if !ok {
						continue
					}

					builder.evaluate(scratch, modes, row, column, component)

					offset := keyIndex * nodeCount
					partial := 0.0

					for nodeIndex := range nodeCount {
						partial += channel.rows[offset+nodeIndex] *
							scratch[nodeIndex] * builder.measure[nodeIndex]
					}

					value += factor * partial
				}

				if value == 0 {
					continue
				}

				weight := couplingWeight(int32(row), int32(column))
				channelScale[channelIndex-1] += math.Abs(value) * weight *
					amplitudes[row] * amplitudes[column]

				candidates = append(candidates, couplingCandidate{
					channel: int32(channelIndex - 1),
					row:     int32(row),
					column:  int32(column),
					value:   value,
				})
			}
		}
	}

	if len(candidates) == 0 {
		return couplingTable{}
	}

	largest := 0.0
	for _, candidate := range candidates {
		largest = max(largest, math.Abs(candidate.value))
	}

	retained := candidates[:0]
	for _, candidate := range candidates {
		if math.Abs(candidate.value) <= couplingEntryFloor*largest {
			continue
		}

		// The importance of one entry is the peak modal force it can deliver:
		// the channel's own reference level times the coefficient times the
		// louder of the two modes it connects. The *louder* of the two, not the
		// product, because the point of the coupling is to drive a mode that is
		// quiet — ranking by the product would delete exactly the entries that
		// reach the unexcited modes this exists to excite.
		candidate.importance = math.Abs(candidate.value) *
			channelScale[candidate.channel] *
			max(amplitudes[candidate.row], amplitudes[candidate.column])
		retained = append(retained, candidate)
	}

	table.candidateCoefficients = len(retained)

	sort.SliceStable(retained, func(first, second int) bool {
		left := retained[first]

		right := retained[second]
		if left.importance != right.importance {
			return left.importance > right.importance
		}

		if left.channel != right.channel {
			return left.channel < right.channel
		}

		if left.row != right.row {
			return left.row < right.row
		}

		return left.column < right.column
	})

	if len(retained) > coupling.MaxCoefficients {
		table.droppedCoefficients = len(retained) - coupling.MaxCoefficients
		retained = retained[:coupling.MaxCoefficients]
	}

	sort.SliceStable(retained, func(first, second int) bool {
		left := retained[first]

		right := retained[second]
		if left.channel != right.channel {
			return left.channel < right.channel
		}

		if left.row != right.row {
			return left.row < right.row
		}

		return left.column < right.column
	})

	table.channelFirst = make([]int32, table.channelCount+1)
	table.entryRow = make([]int32, 0, len(retained))
	table.entryColumn = make([]int32, 0, len(retained))
	table.entryValue = make([]float64, 0, len(retained))

	channel := 0
	for _, candidate := range retained {
		for channel < int(candidate.channel) {
			channel++
			table.channelFirst[channel] = int32(len(table.entryValue))
		}

		table.entryRow = append(table.entryRow, candidate.row)
		table.entryColumn = append(table.entryColumn, candidate.column)
		table.entryValue = append(table.entryValue, candidate.value)
	}

	for channel++; channel <= table.channelCount; channel++ {
		table.channelFirst[channel] = int32(len(table.entryValue))
	}

	table.worstForceFrequencyHz = couplingWorstForceHz(&table, modes)
	table.buildRuns()

	return table
}

// couplingWorstForceHz is the highest frequency the retained cubic force can
// place energy at, read off the table that was actually retained.
//
// The force on one mode is a product of three modal signals, so its spectrum
// reaches the sum of three retained frequencies. Within a channel, the tension
// factor carries a pair (i,j) from that channel's own entries and the force
// factor carries a single further index from another of them, so the worst case
// per channel is max(f_i + f_j) + max(f_k). That is *not* three times the top
// retained mode: the truncation guarantees at least one pump index in every
// entry, which is exactly what keeps the sum below 3*f_top. It is also not the
// 2*max_P f + f_top a free receiver would give, because a receiver that is
// itself a pump admits two free indices.
func couplingWorstForceHz(table *couplingTable, modes []Mode) float64 {
	worst := 0.0

	for channel := range table.channelCount {
		pairMax := 0.0
		singleMax := 0.0

		first := table.channelFirst[channel]
		for slot := first; slot < table.channelFirst[channel+1]; slot++ {
			rowHz := modes[table.entryRow[slot]].FrequencyHz
			columnHz := modes[table.entryColumn[slot]].FrequencyHz
			pairMax = max(pairMax, rowHz+columnHz)
			singleMax = max(singleMax, max(rowHz, columnHz))
		}

		worst = max(worst, pairMax+singleMax)
	}

	return worst
}
