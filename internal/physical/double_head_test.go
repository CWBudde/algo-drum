package physical

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestDoubleHeadZeroCouplingMatchesSingleHead(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Cavity.Enabled = false
	config.Nonlinearity.Enabled = false
	single, err := NewSingleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	double, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := single.Trigger(0.8); err != nil {
		t.Fatal(err)
	}
	if err := double.Trigger(0.8); err != nil {
		t.Fatal(err)
	}

	for sampleIndex := range 20_000 {
		singleOutput := single.Tick()
		doubleOutput := double.Tick()
		if doubleOutput.BatterDisplacementM != singleOutput.DisplacementM ||
			doubleOutput.BatterVelocityMPerS != singleOutput.VelocityMPerS ||
			doubleOutput.BatterRawRadiated != singleOutput.RawRadiated ||
			doubleOutput.Radiated != singleOutput.Radiated {
			t.Fatalf(
				"sample %d zero-coupling output differs:\nsingle=%#v\ndouble=%#v",
				sampleIndex,
				singleOutput,
				doubleOutput,
			)
		}
		if doubleOutput.ResonantDisplacementM != 0 ||
			doubleOutput.ResonantVelocityMPerS != 0 ||
			doubleOutput.ResonantRawRadiated != 0 ||
			doubleOutput.CavityPressurePa != 0 ||
			doubleOutput.CavityMechanicalEnergyJ != 0 {
			t.Fatalf(
				"sample %d zero-coupling resonant/cavity output = %#v",
				sampleIndex,
				doubleOutput,
			)
		}
	}
}

func TestCavityCouplingZeroMatchesDisabledCavity(t *testing.T) {
	t.Parallel()

	disabledConfig := DefaultPhysicalDrum()
	disabledConfig.Cavity.Enabled = false
	disabledConfig.Cavity.Coupling01 = 0
	disabledConfig.Nonlinearity.Enabled = false
	disabled, err := NewDoubleHead(disabledConfig)
	if err != nil {
		t.Fatal(err)
	}

	zeroConfig := disabledConfig
	zeroConfig.Cavity.Enabled = true
	zeroConfig.Cavity.Coupling01 = 0
	zero, err := NewDoubleHead(zeroConfig)
	if err != nil {
		t.Fatal(err)
	}

	if err := disabled.Trigger(0.8); err != nil {
		t.Fatal(err)
	}
	if err := zero.Trigger(0.8); err != nil {
		t.Fatal(err)
	}

	for sampleIndex := range 20_000 {
		want := disabled.Tick()
		got := zero.Tick()
		if got != want {
			t.Fatalf("sample %d zero coupling differs:\ngot  %#v\nwant %#v",
				sampleIndex, got, want)
		}
	}
}

func TestDoubleHeadStrikeExcitesResonantHead(t *testing.T) {
	t.Parallel()

	model, err := NewDoubleHead(DefaultPhysicalDrum())
	if err != nil {
		t.Fatal(err)
	}
	if model.BatterModeCount() == 0 || model.ResonantModeCount() == 0 {
		t.Fatalf(
			"mode counts = batter %d resonant %d",
			model.BatterModeCount(),
			model.ResonantModeCount(),
		)
	}
	if err := model.Trigger(0.8); err != nil {
		t.Fatal(err)
	}

	resonantPeak := 0.0
	pressurePeak := 0.0
	for range 24_000 {
		output := model.Tick()
		resonantPeak = math.Max(
			resonantPeak,
			math.Abs(output.ResonantVelocityMPerS),
		)
		pressurePeak = math.Max(pressurePeak, math.Abs(output.CavityPressurePa))
	}
	if resonantPeak <= 1e-6 {
		t.Fatalf("resonant velocity peak = %v, want audible excitation", resonantPeak)
	}
	if pressurePeak == 0 {
		t.Fatal("cavity pressure remained zero")
	}
}

func TestDoubleHeadLosslessEnergyExchangeIsConservative(t *testing.T) {
	t.Parallel()

	config := losslessDoubleHeadConfig()
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	var (
		referenceEnergy float64
		minCavityEnergy = math.Inf(1)
		maxCavityEnergy float64
	)
	for sampleIndex := range model.PulseSamples() + 30_000 {
		output := model.Tick()
		if sampleIndex == model.PulseSamples()-1 {
			referenceEnergy = output.TotalMechanicalEnergyJ
		}
		if sampleIndex < model.PulseSamples() {
			continue
		}

		if difference := relativeDifference(
			output.TotalMechanicalEnergyJ,
			referenceEnergy,
		); difference > 2e-10 {
			t.Fatalf(
				"sample %d total energy = %.15g, reference = %.15g",
				sampleIndex,
				output.TotalMechanicalEnergyJ,
				referenceEnergy,
			)
		}
		minCavityEnergy = min(minCavityEnergy, output.CavityMechanicalEnergyJ)
		maxCavityEnergy = max(maxCavityEnergy, output.CavityMechanicalEnergyJ)
	}
	if maxCavityEnergy-minCavityEnergy <= referenceEnergy*1e-4 {
		t.Fatalf(
			"cavity energy did not exchange with heads: min=%v max=%v total=%v",
			minCavityEnergy,
			maxCavityEnergy,
			referenceEnergy,
		)
	}
}

func TestDoubleHeadLossesDissipateEnergy(t *testing.T) {
	t.Parallel()

	model, err := NewDoubleHead(DefaultPhysicalDrum())
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}
	for range model.PulseSamples() {
		model.Tick()
	}

	previousEnergy := model.Tick().TotalMechanicalEnergyJ
	for sampleIndex := range 20_000 {
		energy := model.Tick().TotalMechanicalEnergyJ
		tolerance := math.Max(previousEnergy, 1) * 2e-14
		if energy > previousEnergy+tolerance {
			t.Fatalf(
				"sample %d energy increased from %.15g to %.15g",
				sampleIndex,
				previousEnergy,
				energy,
			)
		}
		previousEnergy = energy
	}
}

func TestDoubleHeadInPhaseAndOutOfPhaseModesSplit(t *testing.T) {
	t.Parallel()

	config := losslessDoubleHeadConfig()
	config.Resonant = config.Batter
	outOfPhase, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	inPhase, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	resonantFirst := outOfPhase.batterModeCount
	const displacement = 1e-5
	outOfPhase.displacement[0] = displacement
	outOfPhase.displacement[resonantFirst] = -displacement
	inPhase.displacement[0] = displacement
	inPhase.displacement[resonantFirst] = displacement
	inPhase.cavityPressurePa = inPhase.cavityBulkStiffnessPaPerM3 *
		(inPhase.modes[0].SweptAreaM2*displacement +
			inPhase.modes[resonantFirst].SweptAreaM2*displacement)

	outCrossing := firstModeZeroCrossing(outOfPhase)
	inCrossing := firstModeZeroCrossing(inPhase)
	if outCrossing <= 0 || inCrossing <= 0 {
		t.Fatalf(
			"zero crossings = out-of-phase %d in-phase %d",
			outCrossing,
			inCrossing,
		)
	}
	if inCrossing >= outCrossing {
		t.Fatalf(
			"in-phase crossing %d is not earlier than out-of-phase %d",
			inCrossing,
			outCrossing,
		)
	}
	if math.Abs(outOfPhase.cavityPressurePa) > 1e-9 {
		t.Fatalf(
			"ideal out-of-phase pair generated cavity pressure %v",
			outOfPhase.cavityPressurePa,
		)
	}
}

func TestDoubleHeadReferenceTransferMatchesTimeDomain(t *testing.T) {
	t.Parallel()

	model, err := NewDoubleHead(DefaultPhysicalDrum())
	if err != nil {
		t.Fatal(err)
	}

	const frequencyHz = 137.0
	reference, err := model.ReferenceFrequencyResponse(frequencyHz)
	if err != nil {
		t.Fatal(err)
	}

	sampleRate := model.config.SampleRateHz
	angularFrequency := 2 * math.Pi * frequencyHz
	settleSamples := int(5 * sampleRate)
	measureSamples := int(sampleRate)
	var measured complex128
	for sampleIndex := range settleSamples + measureSamples {
		timeAtMidpoint := (float64(sampleIndex) + 0.5) / sampleRate
		forceN := math.Cos(angularFrequency * timeAtMidpoint)
		output := model.tickCoupled(forceN)
		if sampleIndex < settleSamples {
			continue
		}

		timeAtOutput := float64(sampleIndex+1) / sampleRate
		measured += complex(output.RawRadiated, 0) *
			cmplx.Exp(complex(0, -angularFrequency*timeAtOutput))
	}
	measured *= complex(2/float64(measureSamples), 0)

	if difference := relativeDifference(
		cmplx.Abs(measured),
		cmplx.Abs(reference.RawRadiated),
	); difference > 5e-3 {
		t.Fatalf(
			"time-domain transfer magnitude %.15g, reference %.15g (relative difference %.3g)",
			cmplx.Abs(measured),
			cmplx.Abs(reference.RawRadiated),
			difference,
		)
	}
}

func TestDoubleHeadReconfigureIsValidatedAndAtomic(t *testing.T) {
	t.Parallel()

	model, err := NewDoubleHead(DefaultPhysicalDrum())
	if err != nil {
		t.Fatal(err)
	}
	original := model.Config()
	invalid := original
	invalid.Cavity.DepthM = math.NaN()
	if err := model.Reconfigure(invalid); err == nil {
		t.Fatal("Reconfigure(invalid) succeeded")
	}
	if got := model.Config(); got.Cavity.DepthM != original.Cavity.DepthM {
		t.Fatalf(
			"rejected update changed cavity depth from %v to %v",
			original.Cavity.DepthM,
			got.Cavity.DepthM,
		)
	}

	updated := original
	updated.Batter.TensionNPerM *= 1.1
	updated.Resonant.TensionNPerM *= 0.9
	updated.Cavity.DepthM *= 1.5
	updated.Cavity.AirDensityKgPerM3 *= 0.95
	firstBatterBefore, _ := model.BatterMode(0)
	firstResonantBefore, _ := model.ResonantMode(0)
	stiffnessBefore := model.CavityBulkStiffnessPaPerM3()
	if err := model.Reconfigure(updated); err != nil {
		t.Fatal(err)
	}
	firstBatterAfter, _ := model.BatterMode(0)
	firstResonantAfter, _ := model.ResonantMode(0)
	if firstBatterAfter.FrequencyHz <= firstBatterBefore.FrequencyHz {
		t.Fatal("higher batter tension did not raise batter tuning")
	}
	if firstResonantAfter.FrequencyHz >= firstResonantBefore.FrequencyHz {
		t.Fatal("lower resonant tension did not lower resonant tuning")
	}
	if model.CavityBulkStiffnessPaPerM3() >= stiffnessBefore {
		t.Fatal("deeper/lower-density cavity did not reduce air stiffness")
	}
	if model.IsActive() {
		t.Fatal("successful reconfiguration did not reset dynamic state")
	}

	copy := model.Config()
	copy.Batter.ModeDecayCorrections = append(
		copy.Batter.ModeDecayCorrections,
		ModeDecayCorrection{AzimuthalOrder: 0, RadialOrder: 1},
	)
	if len(model.Config().Batter.ModeDecayCorrections) !=
		len(updated.Batter.ModeDecayCorrections) {
		t.Fatal("Config returned aliased mode-correction storage")
	}
}

func TestDoubleHeadRenderDoesNotAllocate(t *testing.T) {
	model, err := NewDoubleHead(DefaultPhysicalDrum())
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}
	buffer := make([]float64, 512)

	allocations := testing.AllocsPerRun(100, func() {
		model.Render(buffer)
	})
	if allocations != 0 {
		t.Fatalf("Render allocated %v times per call, want 0", allocations)
	}
}

func losslessDoubleHeadConfig() PhysicalDrum {
	config := DefaultPhysicalDrum()
	config.Nonlinearity.Enabled = false
	config.Batter.Loss0PerSecond = 0
	config.Batter.Loss2M2PerSecond = 0
	config.Batter.RadiationLossPerSecond = 0
	config.Resonant.Loss0PerSecond = 0
	config.Resonant.Loss2M2PerSecond = 0
	config.Resonant.RadiationLossPerSecond = 0
	config.Cavity.LossPerSecond = 0

	return config
}

func firstModeZeroCrossing(model *DoubleHead) int {
	for sampleIndex := 1; sampleIndex < 24_000; sampleIndex++ {
		model.tickCoupled(0)
		if model.displacement[0] <= 0 {
			return sampleIndex
		}
	}

	return -1
}
