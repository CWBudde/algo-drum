package physical

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"testing"

	algofft "github.com/cwbudde/algo-fft"
)

// shippedRenderDigest and migratedRenderDigest are SHA-256 over the raw IEEE
// bits of one second of the pre-coupling engine, captured from a git worktree at
// the tree this work started from rather than from this code comparing against
// itself. The first is DefaultPhysicalDrum with the coupling absent; the second
// is a version-10 document round-tripped through DecodeConfig, which is a
// different drum — a lumped one-state cavity — and therefore a second digest.
//
// Nothing but a change to the audio path can move these. If one does move,
// the question to answer is which sample first differs and why, not what the new
// digest is.
const (
	shippedRenderDigest  = "9090a197398f02284924aef9d0231d270545aa5612e053b25d5c0bf17c631f64"
	migratedRenderDigest = "09cc28f24839d9a9d4ceb9f353bfb2a22390096e5cd444bb82db2deaf679b89d"
)

// coupledRenderDigest and fullTableRenderDigest are the same statement made
// about the *coupled* path, which the two digests above deliberately do not
// reach: both are taken with the coupling absent, so no existing digest executes
// accumulateCouplingForces or channelValuesAt even once. Neither does the
// CI-diffed testdata/physical-reference-v2.json, which analysis/report.go
// generates through NewSingleHead — a model with no coupling code in it at all.
//
// Both were captured from a worktree at the tree this work started from, and
// reproduced bit-identically against the working tree it was written in — so
// unlike the pre-coupling pair they are, at capture, this code agreeing with
// itself. That is exactly what they are for: they pin the floating-point order
// of the walk so a later "bit-exact" claim about a rewrite of it can be
// refuted. If one moves, the question to answer is which sample first differs
// and why, not what the new digest is.
//
// TestCouplingDiscreteGradientIsExact and TestCouplingLosslessEnergyIsConserved
// do not cover this. Both compare against a tolerance, so both survive an edit
// that reassociates a sum or contracts a multiply-add — which is precisely the
// class of change a digest exists to catch.
const (
	coupledRenderDigest   = "3c83580f04a59f7039f6b35925a924f5b1a1b2fcd14727467d235b326e9bef08"
	fullTableRenderDigest = "33aee9ffa4e838ac621359fdf7699328b4e59a0e257275478be5105446316420"
)

// TestCouplingUniformChannelReproducesBergerStrain checks the quadrature against
// the one coefficient that is known in closed form.
//
// psi_0 = 1/sqrt(A) gives D^0_ij = Gamma_i delta_ij / sqrt(A) *exactly*, because
// the mode shapes are Dirichlet Laplacian eigenfunctions and their gradients are
// orthogonal by Green's identity — analytically, not to a tolerance. So this is
// simultaneously a test of the radial rule, of the angular decomposition, and of
// the claim that the coupling reduces to the shipped Berger law at zero channel
// count.
func TestCouplingUniformChannelReproducesBergerStrain(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()

	modes, err := generateHeadModes(config, config.Batter)
	if err != nil {
		t.Fatal(err)
	}

	builder := newCouplingBuilder(modes, config.Batter.RadiusM)
	area := math.Pi * config.Batter.RadiusM * config.Batter.RadiusM
	scratch := make([]float64, len(builder.radius))

	worst := 0.0
	for first := range min(16, len(modes)) {
		for second := first; second < min(16, len(modes)); second++ {
			components, count := pairAngularComponents(
				&modes[first],
				&modes[second],
			)

			measured := 0.0
			for _, component := range components[:count] {
				if component.key != (couplingKey{}) {
					continue
				}

				builder.evaluate(scratch, modes, first, second, component)
				for node := range scratch {
					measured += 2 * math.Pi * scratch[node] *
						builder.measure[node] / math.Sqrt(area)
				}
			}

			want := 0.0
			if first == second {
				want = modes[first].ModalMassKg /
					config.Batter.SurfaceDensityKgPerM2 *
					modes[first].WavenumberPerM * modes[first].WavenumberPerM /
					math.Sqrt(area)
			}

			relative := math.Abs(measured-want) / max(math.Abs(want), 1)
			worst = max(worst, relative)
		}
	}

	t.Logf("worst uniform-channel error %.3e", worst)

	if worst > 1e-10 {
		t.Fatalf(
			"uniform channel departs from the analytic Gamma by %.3e",
			worst,
		)
	}
}

// TestCouplingSelectionRuleHoldsStructurallyAndNumerically checks the rule that
// makes the tensor affordable.
//
// A coefficient survives only when the two gradient products share an angular
// order *and* an orientation family. The second half is the one that removes
// most of the tensor, and it is not the naive +/-m_i +/-m_j +/-m_k +/-m_l = 0
// rule that the four-index form suggests. With an all-cosine pump set it has a
// consequence a test can state without any tolerance at all: no stored
// coefficient may connect a sine-orientation mode, because every channel is a
// cosine-family object and the pairing of a sine receiver with a cosine pump is
// a sine-family object.
func TestCouplingSelectionRuleHoldsStructurallyAndNumerically(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Nonlinearity.Coupling.MaxCoefficients = maxCouplingCoefficients

	modes, err := generateHeadModes(config, config.Batter)
	if err != nil {
		t.Fatal(err)
	}

	table := buildCouplingTable(config, modes)
	if len(table.entryValue) == 0 {
		t.Fatal("no coupling coefficients were built")
	}

	for _, pump := range table.pumpIndices {
		if modes[pump].Orientation != OrientationCosine {
			t.Fatalf(
				"pump mode (%d,%d) is not cosine; this test's structural half "+
					"assumes an all-cosine pump set",
				modes[pump].AzimuthalOrder,
				modes[pump].RadialOrder,
			)
		}
	}

	pumpOrders := map[int]struct{}{}
	for _, pump := range table.pumpIndices {
		pumpOrders[modes[pump].AzimuthalOrder] = struct{}{}
	}

	largest := 0.0
	for _, value := range table.entryValue {
		largest = max(largest, math.Abs(value))
	}

	// The candidate count is the whole point of the rule: |P| * N index pairs
	// across C channels is roughly 3800 here, and what survives is a tenth of it.
	t.Logf(
		"%d channels, %d structurally non-zero coefficients (%d dropped by the "+
			"magnitude budget), max |D| %.4e",
		table.channelCount,
		table.candidateCoefficients,
		table.droppedCoefficients,
		largest,
	)

	for slot, value := range table.entryValue {
		row := modes[table.entryRow[slot]]

		column := modes[table.entryColumn[slot]]
		if row.Orientation == OrientationSine ||
			column.Orientation == OrientationSine {
			t.Fatalf(
				"coefficient %.4e connects sine-orientation modes (%d,%d)%s "+
					"and (%d,%d)%s, which an all-cosine pump set cannot reach",
				value,
				row.AzimuthalOrder, row.RadialOrder, row.Orientation,
				column.AzimuthalOrder, column.RadialOrder, column.Orientation,
			)
		}
	}

	// The numerical half. Every mode whose azimuthal order cannot be written as
	// |m - p| or m + p for a pump order p is unreachable, and its coefficients
	// must measure zero rather than merely small.
	// A receiver is reached only if pairing it with some pump lands on an
	// angular order the channel set carries, and the channel set carries
	// |p - q| and p + q over pump orders p and q.
	channelOrders := map[int]struct{}{}
	for first := range pumpOrders {
		for second := range pumpOrders {
			difference := first - second
			if difference < 0 {
				difference = -difference
			}

			channelOrders[difference] = struct{}{}
			channelOrders[first+second] = struct{}{}
		}
	}

	reachable := func(order int) bool {
		for pumpOrder := range pumpOrders {
			difference := order - pumpOrder
			if difference < 0 {
				difference = -difference
			}

			if _, ok := channelOrders[difference]; ok {
				return true
			}

			if _, ok := channelOrders[order+pumpOrder]; ok {
				return true
			}
		}

		return false
	}

	checked := 0
	for index := range modes {
		if reachable(modes[index].AzimuthalOrder) {
			continue
		}

		for slot := range table.entryValue {
			if int(table.entryRow[slot]) != index &&
				int(table.entryColumn[slot]) != index {
				continue
			}

			t.Fatalf(
				"mode (%d,%d)%s at %.2f Hz is unreachable from the pump orders "+
					"but carries coefficient %.4e",
				modes[index].AzimuthalOrder,
				modes[index].RadialOrder,
				modes[index].Orientation,
				modes[index].FrequencyHz,
				table.entryValue[slot],
			)
		}

		checked++
	}

	if checked == 0 {
		t.Fatal("no unreachable modes were available to check")
	}

	t.Logf("%d unreachable modes carry no coefficient", checked)
}

// TestCouplingDiscreteGradientIsExact is the passivity argument, checked as an
// identity rather than as a drift.
//
// With D^c symmetric, g_c^{n+1} - g_c^n = 2 q_bar^T D^c dq exactly, so the
// scalar secant T_c = 2[U_c(g^{n+1}) - U_c(g^n)]/(g^{n+1} - g^n) makes
// F_bar . dq equal to -(U^{n+1} - U^n) with no approximation and no Gonzalez
// projection. A mismatch here is a broken symmetry or a channel evaluated with
// one function and differentiated with another, both of which a loose energy
// tolerance would hide.
func TestCouplingDiscreteGradientIsExact(t *testing.T) {
	t.Parallel()

	model, err := NewDoubleHead(DefaultPhysicalDrum())
	if err != nil {
		t.Fatal(err)
	}

	if !model.couplingActive {
		t.Fatal("the shipped default did not build a coupling table")
	}

	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	// Away from rest, and away from the contact window, so neither the strike
	// nor a zero state can make the identity trivially true.
	for range 4_000 {
		model.Tick()
	}

	batterCount := model.batterModeCount
	before := append([]float64(nil), model.displacement[:batterCount]...)
	oldChannel := append([]float64(nil), model.channelValue...)

	model.Tick()

	after := append([]float64(nil), model.displacement[:batterCount]...)
	newChannel := make([]float64, model.coupling.channelCount)
	model.channelValuesAt(after, newChannel)

	midpoint := make([]float64, batterCount)
	step := make([]float64, batterCount)

	for index := range batterCount {
		midpoint[index] = 0.5 * (before[index] + after[index])
		step[index] = after[index] - before[index]
	}

	force := make([]float64, batterCount)
	for channel := range model.coupling.channelCount {
		tension := model.coupling.coefficientNPerM *
			0.5 * (oldChannel[channel] + newChannel[channel])

		first := model.coupling.channelFirst[channel]
		for slot := first; slot < model.coupling.channelFirst[channel+1]; slot++ {
			row := model.coupling.entryRow[slot]

			column := model.coupling.entryColumn[slot]
			scaled := tension * model.coupling.entryValue[slot]
			force[row] -= scaled * midpoint[column]

			if row != column {
				force[column] -= scaled * midpoint[row]
			}
		}
	}

	work := 0.0
	for index := range batterCount {
		work += force[index] * step[index]
	}

	potentialChange := 0.0
	for channel := range model.coupling.channelCount {
		potentialChange += 0.25 * model.coupling.coefficientNPerM *
			(newChannel[channel]*newChannel[channel] -
				oldChannel[channel]*oldChannel[channel])
	}

	if potentialChange == 0 {
		t.Fatal("the channel potential did not move; the step is degenerate")
	}

	residual := math.Abs(work+potentialChange) / math.Abs(potentialChange)
	t.Logf(
		"work %.6e J, potential change %.6e J, relative residual %.3e",
		work,
		potentialChange,
		residual,
	)

	if residual > 1e-12 {
		t.Fatalf("discrete-gradient identity violated by %.3e", residual)
	}
}

// TestCouplingLosslessEnergyIsConserved is the same statement made about the
// solver rather than about the table: with every loss zeroed, including the
// cavity's, the complete stored energy — linear modal, Berger potential and the
// coupling's channel potentials — must not move.
func TestCouplingLosslessEnergyIsConserved(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	setUniformLoss(&config.Batter, 0)
	setUniformLoss(&config.Resonant, 0)
	config.Batter.RadiationLossPerSecond = 0
	config.Resonant.RadiationLossPerSecond = 0
	config.Cavity.LossPerSecond = 0
	config.Attack.Enabled = false

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	if !model.couplingActive {
		t.Fatal("the coupling table is empty; this measures nothing")
	}

	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	settled := model.PulseSamples() + 16
	reference := 0.0
	worst := 0.0
	couplingShare := 0.0

	for index := range int(config.SampleRateHz) {
		output := model.Tick()
		if index < settled {
			continue
		}

		if reference == 0 {
			reference = output.TotalMechanicalEnergyJ
		}

		couplingShare = max(
			couplingShare,
			output.CouplingPotentialEnergyJ/reference,
		)
		worst = max(
			worst,
			math.Abs(output.TotalMechanicalEnergyJ-reference)/reference,
		)
	}

	t.Logf(
		"lossless drift %.3e over 1 s (E = %.4g J, coupling potential up to "+
			"%.2f %% of it)",
		worst,
		reference,
		100*couplingShare,
	)

	if worst > 1e-9 {
		t.Fatalf("lossless energy drifted by %.3e", worst)
	}
}

// TestCouplingDissipatesWithShippedLosses is the other half of passivity: with
// the losses on, the coupled system may only lose energy.
func TestCouplingDissipatesWithShippedLosses(t *testing.T) {
	t.Parallel()

	for _, velocity := range []float64{0.2, 0.6, 1} {
		config := DefaultPhysicalDrum()
		config.Attack.Enabled = false

		model, err := NewDoubleHead(config)
		if err != nil {
			t.Fatal(err)
		}

		if err := model.Trigger(velocity); err != nil {
			t.Fatal(err)
		}

		settled := model.PulseSamples() + 16
		previous := math.Inf(1)
		worstRise := 0.0

		for index := range int(0.3 * config.SampleRateHz) {
			energy := model.Tick().TotalMechanicalEnergyJ
			if index >= settled {
				worstRise = max(worstRise, energy-previous)
			}

			previous = energy
		}

		t.Logf("velocity %.1f: worst energy rise %.3e J", velocity, worstRise)

		if worstRise > 1e-15 {
			t.Fatalf(
				"velocity %.1f gained %.3e J after contact",
				velocity,
				worstRise,
			)
		}
	}
}

// TestCouplingExcitesModesTheStrikeCannotReach is the M1 claim itself.
//
// Every mode outside the pump set has its strike projection zeroed, so the only
// path into it is the cubic force. The measurement is on the modal amplitudes
// rather than on the radiated spectrum, deliberately: a spectrum of four damped
// sinusoids has Lorentzian skirts that put a -37 dB floor across 476-700 Hz, and
// that floor is leakage from the pumps rather than content in the band. Read off
// the modes, the uncoupled model puts a measurable but tiny amount there through
// the cavity and, with the cavity off, exactly nothing.
func TestCouplingExcitesModesTheStrikeCannotReach(t *testing.T) {
	t.Parallel()

	coupled, modes, pumps := couplingModalPeaks(t, true)
	uncoupled, _, _ := couplingModalPeaks(t, false)

	fundamental := coupled[0]

	bands := []struct {
		lowHz, highHz float64
		atLeastDB     float64
	}{
		// The band docs/physical-excitation-gap.md eliminated every other
		// mechanism for and landed on the contact force. The coupling reaches it
		// without depending on |F(f)| there at all.
		{476, 700, 30},
		{700, 1000, 40},
	}

	for _, band := range bands {
		best := -1
		for index := range modes {
			if pumps[index] {
				continue
			}

			frequency := modes[index].FrequencyHz
			if frequency < band.lowHz || frequency > band.highHz {
				continue
			}

			if best < 0 || coupled[index] > coupled[best] {
				best = index
			}
		}

		if best < 0 {
			t.Fatalf("no non-pump mode in %.0f-%.0f Hz", band.lowHz, band.highHz)
		}

		coupledDB := 20 * math.Log10(coupled[best]/fundamental)
		uncoupledDB := 20 * math.Log10(uncoupled[best]/fundamental+1e-300)
		rise := coupledDB - uncoupledDB

		t.Logf(
			"%.0f-%.0f Hz: (%d,%d)%s at %.1f Hz measures %.1f dB coupled "+
				"against %.1f dB uncoupled, a %.1f dB rise",
			band.lowHz, band.highHz,
			modes[best].AzimuthalOrder,
			modes[best].RadialOrder,
			modes[best].Orientation,
			modes[best].FrequencyHz,
			coupledDB,
			uncoupledDB,
			rise,
		)

		if rise < band.atLeastDB {
			t.Fatalf(
				"%.0f-%.0f Hz rose only %.1f dB, below the %.0f dB this claims",
				band.lowHz, band.highHz, rise, band.atLeastDB,
			)
		}
	}
}

// couplingModalPeaks renders a velocity-1 hit whose only excited modes are the
// pumps, and returns each batter mode's peak displacement.
func couplingModalPeaks(t *testing.T, coupled bool) ([]float64, []Mode, map[int]bool) {
	t.Helper()

	config := DefaultPhysicalDrum()
	config.Attack.Enabled = false
	config.Nonlinearity.Coupling.Enabled = coupled

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	batter := model.modes[:model.batterModeCount]

	pumps := map[int]bool{}
	for _, index := range selectPumpModes(
		config,
		batter,
		referenceModalAmplitudes(config, batter),
	) {
		pumps[index] = true
	}

	for index := range model.batterModeCount {
		if pumps[index] {
			continue
		}

		model.modes[index].StrikeAccelerationPerN = 0
		model.strikeWeight[index] = 0
	}

	// d.modes is the source of truth and the midpoint kernel reads mirrors of it.
	// Skipping this leaves every mode still struck directly, which turns the 30 dB
	// claim below into 0.6 dB and looks like a physics regression rather than a
	// stale cache. TestModeArraysMirrorTheBank guards the construction path.
	model.syncModeArrays()

	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	peaks := make([]float64, model.batterModeCount)
	for range int(0.4 * config.SampleRateHz) {
		model.Tick()
		for index := range peaks {
			peaks[index] = max(
				peaks[index],
				math.Abs(model.displacement[index]),
			)
		}
	}

	return peaks, batter, pumps
}

// TestCouplingBrightensTheAttackWithStrikingForce is the Dahl measurement, run
// with the one mechanism the model already had for it held still: the contact
// pulse's duration is frozen at its velocity-1 value, so its spectrum is the
// same shape at every dynamic and only its scale changes, and the attack layer
// is off.
//
// What is *not* held still is the Berger detune, which raises every partial with
// amplitude and therefore moves a centroid on its own. So this is not an
// isolation of the coupling; it is a comparison, and the number that means
// something is the difference between the two slopes. Dahl's published slope is
// not in this repository, so only the sign and the order of magnitude are
// asserted.
func TestCouplingBrightensTheAttackWithStrikingForce(t *testing.T) {
	t.Parallel()

	velocities := []float64{0.2, 0.4, 0.6, 0.8, 1}
	slopes := map[bool]float64{}

	for _, coupled := range []bool{false, true} {
		centroids := make([]float64, len(velocities))
		for index, velocity := range velocities {
			centroids[index] = couplingAttackCentroid(t, coupled, velocity)
			t.Logf(
				"coupling %v, velocity %.1f: attack centroid %.2f Hz",
				coupled,
				velocity,
				centroids[index],
			)

			if index > 0 && centroids[index] <= centroids[index-1] {
				t.Fatalf(
					"coupling %v: centroid fell from %.2f to %.2f Hz between "+
						"velocity %.1f and %.1f",
					coupled,
					centroids[index-1],
					centroids[index],
					velocities[index-1],
					velocity,
				)
			}
		}

		slopes[coupled] = linearSlope(velocities, centroids)
		t.Logf("coupling %v: slope %.2f Hz per unit velocity", coupled, slopes[coupled])
	}

	if slopes[true] <= 0 {
		t.Fatalf("coupled centroid slope %.2f is not positive", slopes[true])
	}

	if slopes[true] <= slopes[false] {
		t.Fatalf(
			"the coupling did not steepen the centroid slope: %.2f coupled "+
				"against %.2f uncoupled",
			slopes[true],
			slopes[false],
		)
	}
}

func linearSlope(inputs, outputs []float64) float64 {
	meanInput := 0.0
	meanOutput := 0.0

	for index := range inputs {
		meanInput += inputs[index] / float64(len(inputs))
		meanOutput += outputs[index] / float64(len(outputs))
	}

	covariance := 0.0
	variance := 0.0

	for index := range inputs {
		covariance += (inputs[index] - meanInput) *
			(outputs[index] - meanOutput)
		variance += (inputs[index] - meanInput) * (inputs[index] - meanInput)
	}

	return covariance / variance
}

// couplingAttackCentroid measures the spectral centroid above the fundamental
// over the same 43 ms attack window the P4 calibration uses, with the contact
// duration frozen so velocity changes only the impulse.
func couplingAttackCentroid(t *testing.T, coupled bool, velocity float64) float64 {
	t.Helper()

	config := DefaultPhysicalDrum()
	config.Attack.Enabled = false
	config.Nonlinearity.Coupling.Enabled = coupled

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	samples := contactSampleCount(
		config.SampleRateHz,
		config.Strike.Hardness01,
		1,
	)
	impulse := config.Strike.MalletMassKg * config.Strike.VelocityMPerS *
		velocity
	addContactPulse(
		model.contact.pending,
		0,
		samples,
		impulse*config.SampleRateHz,
	)
	model.contact.pendingSamples = samples
	model.contact.contactSamples = samples

	const fftSize = 2048

	windowed := make([]float64, fftSize)
	for index := range windowed {
		window := 0.5 - 0.5*math.Cos(
			2*math.Pi*float64(index)/float64(fftSize),
		)
		windowed[index] = model.Tick().BatterRawRadiated * window
	}

	plan, err := algofft.NewPlanReal64(fftSize)
	if err != nil {
		t.Fatal(err)
	}

	bins := make([]complex128, plan.SpectrumLen())
	if err := plan.Forward(bins, windowed); err != nil {
		t.Fatal(err)
	}

	fundamental, ok := model.BatterMode(0)
	if !ok {
		t.Fatal("batter head has no modes")
	}

	floorHz := 1.4 * fundamental.FrequencyHz
	weighted := 0.0
	power := 0.0

	for index, bin := range bins {
		frequency := float64(index) * config.SampleRateHz / fftSize
		if frequency < floorHz {
			continue
		}

		magnitude := real(bin)*real(bin) + imag(bin)*imag(bin)
		weighted += frequency * magnitude
		power += magnitude
	}

	if power == 0 {
		t.Fatal("no spectral power above the fundamental")
	}

	return weighted / power
}

// TestCouplingAliasBoundRejectsAnUnsafeConfiguration checks the bound that
// replaces r < 1/(4 alpha^2) - 1 for this term, and confirms the shipped default
// clears it.
func TestCouplingAliasBoundRejectsAnUnsafeConfiguration(t *testing.T) {
	t.Parallel()

	if err := DefaultPhysicalDrum().Validate(); err != nil {
		t.Fatalf("the shipped default fails its own alias bound: %v", err)
	}

	model, err := NewDoubleHead(DefaultPhysicalDrum())
	if err != nil {
		t.Fatal(err)
	}

	config := DefaultPhysicalDrum()
	limitHz := config.Nonlinearity.Coupling.AliasFraction * config.SampleRateHz
	t.Logf(
		"retained force reaches %.1f Hz against the %.1f Hz bound",
		model.CouplingWorstForceHz(),
		limitHz,
	)

	if model.CouplingWorstForceHz() >= limitHz {
		t.Fatalf(
			"the retained table reaches %.1f Hz, past the %.1f Hz bound",
			model.CouplingWorstForceHz(),
			limitHz,
		)
	}

	tight := DefaultPhysicalDrum()
	tight.Nonlinearity.Coupling.AliasFraction = 0.02
	if err := tight.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("a 0.02 alias fraction was accepted: %v", err)
	} else {
		t.Logf("rejected as expected: %v", err)
	}

	single := DefaultPhysicalDrum()
	single.Nonlinearity.Coupling.PumpCount = 1
	if err := single.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("a single-pump coupling was accepted: %v", err)
	}
}

// TestCouplingRendersWithoutAllocating holds the audio-path contract the whole
// model is written to.
func TestCouplingRendersWithoutAllocating(t *testing.T) {
	model, err := NewDoubleHead(DefaultPhysicalDrum())
	if err != nil {
		t.Fatal(err)
	}

	if !model.couplingActive {
		t.Fatal("the coupling table is empty; this measures nothing")
	}

	buffer := make([]float64, 512)
	allocations := testing.AllocsPerRun(64, func() {
		if err := model.Trigger(1); err != nil {
			t.Fatal(err)
		}

		model.Render(buffer)
	})

	if allocations != 0 {
		t.Fatalf("Trigger+Render allocated %.1f times per run", allocations)
	}
}

// TestCouplingSolveIterationsStayBounded measures what the coupling costs the
// fixed point. The bound is a reporting threshold rather than a limit: the solve
// is capped at nonlinearSolveIterations either way, so exceeding it would be a
// number to record, not a failure to route around.
func TestCouplingSolveIterationsStayBounded(t *testing.T) {
	t.Parallel()

	for _, coupled := range []bool{false, true} {
		config := DefaultPhysicalDrum()
		config.Nonlinearity.Coupling.Enabled = coupled

		model, err := NewDoubleHead(config)
		if err != nil {
			t.Fatal(err)
		}

		if err := model.Trigger(1); err != nil {
			t.Fatal(err)
		}

		total := 0
		highest := 0
		count := int(0.3 * config.SampleRateHz)

		for range count {
			iterations := model.Tick().NonlinearSolveIterations
			total += iterations
			highest = max(highest, iterations)
		}

		mean := float64(total) / float64(count)
		t.Logf(
			"coupling %v: mean %.3f iterations, highest %d",
			coupled,
			mean,
			highest,
		)

		if coupled && mean > 4 {
			t.Fatalf("mean solve iterations %.3f exceeds 4.0", mean)
		}
	}
}

// TestCouplingDisabledMatchesTheShippedEngine is the checkable limit.
//
// With the coupling absent, NewDoubleHead must build a zero-length table and
// Tick must take the path that shipped — not a coupled path multiplied by zero.
// The comparison is against digests captured from a worktree of the pre-coupling
// tree, so it cannot be satisfied by this code agreeing with itself.
func TestCouplingDisabledMatchesTheShippedEngine(t *testing.T) {
	t.Parallel()

	absent := DefaultPhysicalDrum()
	absent.Nonlinearity.Coupling = NonlinearCoupling{}

	model, digest := couplingRenderDigest(t, absent)
	if model.CouplingCoefficientCount() != 0 || model.couplingActive {
		t.Fatalf(
			"a disabled coupling built %d coefficients",
			model.CouplingCoefficientCount(),
		)
	}

	if digest != shippedRenderDigest {
		t.Fatalf(
			"default render digest %s, want the pre-coupling %s",
			digest,
			shippedRenderDigest,
		)
	}

	legacy := DefaultPhysicalDrum()
	legacy.Version = selectableContactVersion

	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}

	cavity, ok := document["cavity"].(map[string]any)
	if !ok {
		t.Fatalf("cavity is not an object: %#v", document["cavity"])
	}

	// Neither field existed in version 10, so a stored document carries neither.
	delete(cavity, "modeCount")
	delete(document, "resonantModeLimit")

	nonlinearity, ok := document["nonlinearity"].(map[string]any)
	if !ok {
		t.Fatalf("nonlinearity is not an object: %#v", document["nonlinearity"])
	}

	delete(nonlinearity, "coupling")

	encoded, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeConfig(encoded)
	if err != nil {
		t.Fatalf("DecodeConfig(v10) error = %v", err)
	}

	if decoded.Nonlinearity.Coupling.Enabled ||
		decoded.Nonlinearity.Coupling.CoefficientNPerM != 0 {
		t.Fatalf(
			"the v10 migration enabled the coupling: %#v",
			decoded.Nonlinearity.Coupling,
		)
	}

	migrated, migratedDigest := couplingRenderDigest(t, decoded)
	if migrated.CouplingCoefficientCount() != 0 {
		t.Fatalf(
			"a migrated document built %d coefficients",
			migrated.CouplingCoefficientCount(),
		)
	}

	if migratedDigest != migratedRenderDigest {
		t.Fatalf(
			"migrated render digest %s, want the pre-coupling %s",
			migratedDigest,
			migratedRenderDigest,
		)
	}
}

// TestCoupledRenderIsBitExact pins the coupled audio path itself.
//
// The shipped default retains 256 of the candidate coefficients; raising
// MaxCoefficients past the candidate count retains all 408, which is a longer
// walk over a different table and so must be a different render. The counts are
// asserted rather than assumed, because two equal digests here would otherwise
// read as "the walk is stable" when what actually happened is that the second
// configuration silently reproduced the first.
func TestCoupledRenderIsBitExact(t *testing.T) {
	t.Parallel()

	shipped := DefaultPhysicalDrum()
	if !shipped.Nonlinearity.Coupling.Enabled {
		t.Fatal("the shipped default no longer enables the coupling")
	}

	shippedModel, shippedDigest := couplingRenderDigest(t, shipped)
	shippedCount := shippedModel.CouplingCoefficientCount()
	t.Logf("shipped: %d coefficients, digest %s", shippedCount, shippedDigest)

	if shippedCount != shipped.Nonlinearity.Coupling.MaxCoefficients {
		t.Fatalf(
			"the shipped table holds %d coefficients, want the %d cap",
			shippedCount,
			shipped.Nonlinearity.Coupling.MaxCoefficients,
		)
	}

	// Any cap above the candidate count retains the whole table, and 4096 is
	// both the validator's ceiling and an order of magnitude past the 408
	// candidates the shipped geometry produces.
	full := DefaultPhysicalDrum()
	full.Nonlinearity.Coupling.MaxCoefficients = 4096

	fullModel, fullDigest := couplingRenderDigest(t, full)
	fullCount := fullModel.CouplingCoefficientCount()
	t.Logf("full table: %d coefficients, digest %s", fullCount, fullDigest)

	if fullCount <= shippedCount {
		t.Fatalf(
			"the uncapped table holds %d coefficients, not more than the shipped %d",
			fullCount,
			shippedCount,
		)
	}

	if shippedDigest != coupledRenderDigest {
		t.Fatalf(
			"coupled render digest %s, want %s",
			shippedDigest,
			coupledRenderDigest,
		)
	}

	if fullDigest != fullTableRenderDigest {
		t.Fatalf(
			"full-table render digest %s, want %s",
			fullDigest,
			fullTableRenderDigest,
		)
	}
}

func couplingRenderDigest(t *testing.T, config PhysicalDrum) (*DoubleHead, string) {
	t.Helper()

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	rendered := make([]float64, 48_000)
	model.Render(rendered)

	digest := sha256.New()

	var word [8]byte
	for _, sample := range rendered {
		binary.LittleEndian.PutUint64(word[:], math.Float64bits(sample))
		digest.Write(word[:])
	}

	return model, hex.EncodeToString(digest.Sum(nil))
}

// TestCouplingPumpSelectionIsNotFrequencyOrdered records the measurement the
// pump rule rests on. Ranking by peak displacement under the reference strike is
// not the same as ranking by frequency, and it is not the same as ranking by
// energy either: the force is cubic, so displacement is the right weight.
func TestCouplingPumpSelectionIsNotFrequencyOrdered(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()

	modes, err := generateHeadModes(config, config.Batter)
	if err != nil {
		t.Fatal(err)
	}

	amplitudes := referenceModalAmplitudes(config, modes)

	ranked := make([]int, 0, len(modes))
	for index := range modes {
		if amplitudes[index] > 0 {
			ranked = append(ranked, index)
		}
	}

	for outer := range min(8, len(ranked)) {
		best := outer
		for inner := outer; inner < len(ranked); inner++ {
			if amplitudes[ranked[inner]] > amplitudes[ranked[best]] {
				best = inner
			}
		}

		ranked[outer], ranked[best] = ranked[best], ranked[outer]
		mode := modes[ranked[outer]]
		t.Logf(
			"rank %d: (%d,%d)%s at %.1f Hz, amplitude %.4g",
			outer,
			mode.AzimuthalOrder,
			mode.RadialOrder,
			mode.Orientation,
			mode.FrequencyHz,
			amplitudes[ranked[outer]],
		)
	}

	inversions := 0
	for index := 1; index < min(8, len(ranked)); index++ {
		if modes[ranked[index]].FrequencyHz <
			modes[ranked[index-1]].FrequencyHz {
			inversions++
		}
	}

	if inversions == 0 {
		t.Fatal(
			"the amplitude ranking agrees with the frequency ordering over the " +
				"top eight modes, which would make the rule pointless",
		)
	}

	t.Logf("%d frequency inversions in the top eight", inversions)
}

// TestObserveReusesCurrentChannelValues pins the invariant observe(true) rests
// on: after a coupled tick, channelTrial already holds g_c at the displacement
// tickCoupled committed, so reusing it is a copy and not an approximation.
//
// The optimisation it licenses — dropping a third traversal of the coupling
// table per sample — is invisible when it goes wrong: a stale channelValue
// misreports CouplingPotentialEnergyJ and seeds the next fixed point slightly
// off, and nothing else complains. If solveMidpoint ever stops evaluating the
// channels at the endpoint the caller commits, this fails loudly instead.
func TestObserveReusesCurrentChannelValues(t *testing.T) {
	config := DefaultPhysicalDrum()
	config.SampleRateHz = 44100

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatalf("NewDoubleHead: %v", err)
	}

	if !model.couplingActive {
		t.Fatal("coupling inactive at the shipped defaults; the test proves nothing")
	}

	if err := model.Trigger(1); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	recomputed := make([]float64, model.coupling.channelCount)

	// Long enough to cover the strike, the decay, and any diverged step that
	// re-solves without the coupling along the way.
	for sample := range 8820 {
		model.Tick()
		model.channelValuesAt(model.displacement, recomputed)

		for channel, want := range recomputed {
			// Exact: same function, same inputs, so anything but equality means
			// channelTrial was not evaluated at the committed state.
			if model.channelValue[channel] != want {
				t.Fatalf(
					"sample %d channel %d: reused g_c = %g, recomputed %g",
					sample, channel, model.channelValue[channel], want,
				)
			}
		}
	}
}

// TestModeArraysMirrorTheBank pins the cache contract the midpoint kernel rests
// on: the struct-of-arrays mirrors must equal the fields they are derived from.
//
// The kernel cannot read the 144-byte Mode struct — a vector load needs
// contiguous float64 — so the mirrors are not an optimisation that can be
// dropped. Exact equality, because they are copies, not computations.
func TestModeArraysMirrorTheBank(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatalf("NewDoubleHead: %v", err)
	}

	for index := range model.modes {
		mode := &model.modes[index]

		if got := model.modeWavenumberPerM[index]; got != mode.WavenumberPerM {
			t.Fatalf("mode %d wavenumber mirror %g, bank %g", index, got, mode.WavenumberPerM)
		}

		want := mode.AngularFrequency * mode.AngularFrequency
		if got := model.modeOmegaSquared[index]; got != want {
			t.Fatalf("mode %d omega-squared mirror %g, bank %g", index, got, want)
		}

		if got := model.modeStrikeAccelPerN[index]; got != mode.StrikeAccelerationPerN {
			t.Fatalf("mode %d strike mirror %g, bank %g", index, got, mode.StrikeAccelerationPerN)
		}

		if got := model.modeRadiationWeight[index]; got != mode.RadiationWeight {
			t.Fatalf("mode %d radiation mirror %g, bank %g", index, got, mode.RadiationWeight)
		}

		if got := model.modePickupShape[index]; got != mode.PickupShape {
			t.Fatalf("mode %d pickup mirror %g, bank %g", index, got, mode.PickupShape)
		}

		if got := model.modeSweptAreaM2[index]; got != mode.SweptAreaM2 {
			t.Fatalf("mode %d swept-area mirror %g, bank %g", index, got, mode.SweptAreaM2)
		}

		if got := model.modeModalMassKg[index]; got != mode.ModalMassKg {
			t.Fatalf("mode %d modal-mass mirror %g, bank %g", index, got, mode.ModalMassKg)
		}
	}
}

// TestSingleHeadModeArraysMirrorTheBank is TestModeArraysMirrorTheBank for the
// P2 reference, whose Tick reads the same five quantities out of mirrors rather
// than out of the 144-byte bank. Same contract, same reason: s.modes is the
// source of truth and these are copies of it.
func TestSingleHeadModeArraysMirrorTheBank(t *testing.T) {
	t.Parallel()

	model, err := NewSingleHead(DefaultPhysicalDrum())
	if err != nil {
		t.Fatalf("NewSingleHead: %v", err)
	}

	for index := range model.modes {
		mode := &model.modes[index]

		if got := model.modePickupShape[index]; got != mode.PickupShape {
			t.Fatalf("mode %d pickup mirror %g, bank %g", index, got, mode.PickupShape)
		}

		if got := model.modeRadiationWeight[index]; got != mode.RadiationWeight {
			t.Fatalf("mode %d radiation mirror %g, bank %g", index, got, mode.RadiationWeight)
		}

		if got := model.modeModalMassKg[index]; got != mode.ModalMassKg {
			t.Fatalf("mode %d modal-mass mirror %g, bank %g", index, got, mode.ModalMassKg)
		}

		if got := model.modeStrikeAccelPerN[index]; got != mode.StrikeAccelerationPerN {
			t.Fatalf("mode %d strike mirror %g, bank %g", index, got, mode.StrikeAccelerationPerN)
		}

		want := mode.AngularFrequency * mode.AngularFrequency
		if got := model.modeOmegaSquared[index]; got != want {
			t.Fatalf("mode %d omega-squared mirror %g, bank %g", index, got, want)
		}
	}
}
