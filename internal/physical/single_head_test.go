package physical

import (
	"errors"
	"math"
	"testing"
)

func TestSingleHeadDeterministicAndFinite(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	first, err := NewSingleHead(config)
	if err != nil {
		t.Fatalf("NewSingleHead(first) error = %v", err)
	}
	second, err := NewSingleHead(config)
	if err != nil {
		t.Fatalf("NewSingleHead(second) error = %v", err)
	}
	if err := first.Trigger(0.8); err != nil {
		t.Fatal(err)
	}
	if err := second.Trigger(0.8); err != nil {
		t.Fatal(err)
	}

	peak := 0.0
	for sampleIndex := range 48_000 {
		firstOutput := first.Tick()
		secondOutput := second.Tick()
		if firstOutput != secondOutput {
			t.Fatalf("sample %d differs: %#v versus %#v", sampleIndex, firstOutput, secondOutput)
		}
		if !isFinite(firstOutput.DisplacementM) ||
			!isFinite(firstOutput.VelocityMPerS) ||
			!isFinite(firstOutput.Radiated) ||
			!isFinite(firstOutput.MechanicalEnergyJ) {
			t.Fatalf("sample %d is non-finite: %#v", sampleIndex, firstOutput)
		}
		peak = math.Max(peak, math.Abs(firstOutput.Radiated))
	}
	if peak == 0 {
		t.Fatal("triggered model produced silence")
	}
}

func TestSingleHeadDiagnosticOutput(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	model, err := NewSingleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	for range model.PulseSamples() + 1 {
		output := model.Tick()
		want := config.Pickup.OutputGain * output.VelocityMPerS
		if output.Radiated != want {
			t.Fatalf("Radiated = %v, want gain * velocity = %v", output.Radiated, want)
		}
	}
}

func TestSingleHeadLosslessEnergyIsBounded(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Batter.Loss0PerSecond = 0
	config.Batter.Loss2M2PerSecond = 0
	model, err := NewSingleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	var referenceEnergy float64
	for sampleIndex := range model.PulseSamples() + 20_000 {
		output := model.Tick()
		if sampleIndex == model.PulseSamples()-1 {
			referenceEnergy = output.MechanicalEnergyJ
		}
		if sampleIndex < model.PulseSamples() {
			continue
		}
		if relativeDifference(output.MechanicalEnergyJ, referenceEnergy) > 2e-11 {
			t.Fatalf(
				"sample %d energy = %.15g, reference = %.15g",
				sampleIndex,
				output.MechanicalEnergyJ,
				referenceEnergy,
			)
		}
	}
}

func TestSingleHeadDampedEnergyIsMonotonicAfterContact(t *testing.T) {
	t.Parallel()

	model, err := NewSingleHead(DefaultPhysicalDrum())
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	for range model.PulseSamples() {
		model.Tick()
	}
	previousEnergy := model.Tick().MechanicalEnergyJ
	for sampleIndex := range 20_000 {
		energy := model.Tick().MechanicalEnergyJ
		tolerance := math.Max(previousEnergy, 1) * 1e-14
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

func TestSingleHeadStrikePositionChangesModeMix(t *testing.T) {
	t.Parallel()

	centerConfig := DefaultPhysicalDrum()
	centerConfig.Strike.Radius01 = 0
	edgeConfig := centerConfig
	edgeConfig.Strike.Radius01 = 0.65

	center, err := NewSingleHead(centerConfig)
	if err != nil {
		t.Fatal(err)
	}
	edge, err := NewSingleHead(edgeConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := center.Trigger(1); err != nil {
		t.Fatal(err)
	}
	if err := edge.Trigger(1); err != nil {
		t.Fatal(err)
	}

	differenceEnergy := 0.0
	for range 4096 {
		difference := center.Tick().Radiated - edge.Tick().Radiated
		differenceEnergy += difference * difference
	}
	if differenceEnergy < 1e-12 {
		t.Fatalf("center and off-center renders are indistinguishable: difference energy %v", differenceEnergy)
	}
}

func TestSingleHeadTriggerValidationAndReset(t *testing.T) {
	t.Parallel()

	model, err := NewSingleHead(DefaultPhysicalDrum())
	if err != nil {
		t.Fatal(err)
	}
	for _, velocity := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1)} {
		if err := model.Trigger(velocity); !errors.Is(err, ErrInvalidVelocity) {
			t.Errorf("Trigger(%v) error = %v, want ErrInvalidVelocity", velocity, err)
		}
	}

	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}
	for range 100 {
		model.Tick()
	}
	model.Reset()
	if model.IsActive() {
		t.Fatal("Reset model remains active")
	}
	if output := model.Tick(); output != (Output{}) {
		t.Fatalf("Tick() after Reset = %#v, want silence", output)
	}
}

func TestSingleHeadRenderDoesNotAllocate(t *testing.T) {
	config := DefaultPhysicalDrum()
	model, err := NewSingleHead(config)
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
