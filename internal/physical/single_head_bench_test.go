package physical

import "testing"

// retriggerEvery bounds how long a single strike is allowed to decay before the
// benchmark hits it again.
//
// Without it these benchmarks measure subnormals rather than the model. One
// strike left to ring decays into the subnormal range after roughly fifteen
// seconds of audio, and from there every multiply and add in the modal update
// takes a microcode assist: measured on this bank, 40x realtime at 3 s of tail,
// 20x at 11 s, 2.4x at 21 s and 0.77x at 85 s, a hundredfold slowdown that says
// nothing about the cost of rendering a drum hit. Nothing reaches that state in
// practice — physicalTom.Tick gates on IsActive long before it, and the offline
// tools render 1.2 to 2 seconds — so the benchmark is what needed fixing, not
// the model. 512 chunks is about 2.7 s of audio, well inside the normal range.
const retriggerEvery = 256

func BenchmarkSingleHeadRender48k(b *testing.B) {
	config := DefaultPhysicalDrum()
	model, err := NewSingleHead(config)
	if err != nil {
		b.Fatal(err)
	}

	const chunkSamples = 512
	buffer := make([]float64, chunkSamples)
	b.ReportAllocs()
	b.SetBytes(chunkSamples * 8)
	b.ResetTimer()
	for index := range b.N {
		if index%retriggerEvery == 0 {
			if err := model.Trigger(1); err != nil {
				b.Fatal(err)
			}
		}

		model.Render(buffer)
	}
	b.StopTimer()

	samples := float64(b.N * chunkSamples)
	samplesPerSecond := samples / b.Elapsed().Seconds()
	b.ReportMetric(samplesPerSecond, "samples/s")
	b.ReportMetric(samplesPerSecond/config.SampleRateHz, "x_realtime")
	b.ReportMetric(float64(model.ModeCount()), "modes")
}

func BenchmarkDoubleHeadRender48k(b *testing.B) {
	config := DefaultPhysicalDrum()
	model, err := NewDoubleHead(config)
	if err != nil {
		b.Fatal(err)
	}

	const chunkSamples = 512
	buffer := make([]float64, chunkSamples)
	b.ReportAllocs()
	b.SetBytes(chunkSamples * 8)
	b.ResetTimer()
	for index := range b.N {
		if index%retriggerEvery == 0 {
			if err := model.Trigger(1); err != nil {
				b.Fatal(err)
			}
		}

		model.Render(buffer)
	}
	b.StopTimer()

	samples := float64(b.N * chunkSamples)
	samplesPerSecond := samples / b.Elapsed().Seconds()
	b.ReportMetric(samplesPerSecond, "samples/s")
	b.ReportMetric(samplesPerSecond/config.SampleRateHz, "x_realtime")
	b.ReportMetric(
		float64(model.BatterModeCount()+model.ResonantModeCount()),
		"modes",
	)
}

func BenchmarkNonlinearDoubleHeadActive48k(b *testing.B) {
	config := DefaultPhysicalDrum()
	model, err := NewDoubleHead(config)
	if err != nil {
		b.Fatal(err)
	}

	const chunkSamples = 512
	buffer := make([]float64, chunkSamples)
	b.ReportAllocs()
	b.SetBytes(chunkSamples * 8)
	b.ResetTimer()
	for range b.N {
		if err := model.Trigger(1); err != nil {
			b.Fatal(err)
		}
		model.Render(buffer)
	}
	b.StopTimer()

	samples := float64(b.N * chunkSamples)
	samplesPerSecond := samples / b.Elapsed().Seconds()
	b.ReportMetric(samplesPerSecond, "samples/s")
	b.ReportMetric(samplesPerSecond/config.SampleRateHz, "x_realtime")
	b.ReportMetric(
		float64(model.BatterModeCount()+model.ResonantModeCount()),
		"modes",
	)
}
