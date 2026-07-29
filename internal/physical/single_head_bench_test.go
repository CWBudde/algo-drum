package physical

import "testing"

func BenchmarkSingleHeadRender48k(b *testing.B) {
	config := DefaultPhysicalDrum()
	model, err := NewSingleHead(config)
	if err != nil {
		b.Fatal(err)
	}
	if err := model.Trigger(1); err != nil {
		b.Fatal(err)
	}

	const chunkSamples = 512
	buffer := make([]float64, chunkSamples)
	b.ReportAllocs()
	b.SetBytes(chunkSamples * 8)
	b.ResetTimer()
	for range b.N {
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
	if err := model.Trigger(1); err != nil {
		b.Fatal(err)
	}

	const chunkSamples = 512
	buffer := make([]float64, chunkSamples)
	b.ReportAllocs()
	b.SetBytes(chunkSamples * 8)
	b.ResetTimer()
	for range b.N {
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
