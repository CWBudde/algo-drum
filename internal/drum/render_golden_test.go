package drum

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"runtime"
	"testing"
)

const (
	engineRenderGoldenDigest = "346319e6f6074ba3793c937e7449b8ec546d5034222d3ace7f093a415b6d184d"
	goldenRenderSampleRate   = 48_000.0
	goldenRenderStepSamples  = 6_000
	goldenRenderChunkSamples = 512
)

var engineRenderGoldenStepRMS = [MaxSteps]float64{
	0.2623268811901039,
	0.20699167045865238,
	0.24707495150687797,
	0.14616432088781137,
	0.15215225568588628,
	0.06459802081703268,
	0.23081669237815042,
	0.1818410423746217,
	0.2083964014983451,
	0.17113051599368997,
	0.2521322862197667,
	0.20252000824258942,
	0.20517876076465516,
	0.145867667667144,
	0.24386821465681333,
	0.23128399603958358,
}

// TestEngineRenderGolden protects the audible output of the complete engine,
// rather than only proving that two engines built by the same code agree. The
// fixture gives every procedural voice an isolated hit, then ends with a
// full-kit hit so the mix, reverb and limiter are covered as well.
//
// The digest is an amd64 fact: transcendental implementations can differ at
// the last bit across architectures. Every architecture still checks the
// per-step RMS windows, whose narrow tolerance catches meaningful sound drift
// without turning harmless floating-point differences into failures.
func TestEngineRenderGolden(t *testing.T) {
	rendered := renderEngineGoldenFixture()
	gotDigest := float32RenderDigest(rendered)
	gotRMS := renderStepRMS(rendered)
	failed := false

	for step, want := range engineRenderGoldenStepRMS {
		got := gotRMS[step]
		tolerance := max(1e-8, math.Abs(want)*1e-4)
		delta := math.Abs(got - want)
		if delta <= tolerance {
			continue
		}

		relativeDelta := math.Inf(1)
		if want != 0 {
			relativeDelta = delta / math.Abs(want)
		}

		t.Errorf(
			"step %02d RMS = %.17g, want %.17g (abs delta %.3g, relative delta %.3g, tolerance %.3g)",
			step,
			got,
			want,
			delta,
			relativeDelta,
			tolerance,
		)
		failed = true
	}

	if runtime.GOARCH == "amd64" && gotDigest != engineRenderGoldenDigest {
		t.Errorf("engine render digest %s, want %s", gotDigest, engineRenderGoldenDigest)
		failed = true
	}

	if failed {
		t.Logf("candidate engineRenderGoldenDigest = %q", gotDigest)
		t.Logf("candidate engineRenderGoldenStepRMS = %#v", gotRMS)
	}
}

func renderEngineGoldenFixture() []float32 {
	engine := NewEngine(goldenRenderSampleRate)
	engine.SetTempo(120)
	engine.SetSwing(0)
	engine.SetProbability(1)
	engine.SetHumanize(0)
	engine.SetReverb(0.35)

	isolatedSteps := [TrackCount]int{0, 2, 4, 6, 8, 10, 12}
	for track, step := range isolatedSteps {
		engine.SetCell(0, track, step, 1)
		engine.SetCell(0, track, 14, 0.7)
	}

	engine.SetRunning(true)

	rendered := make([]float32, MaxSteps*goldenRenderStepSamples)
	for start := 0; start < len(rendered); start += goldenRenderChunkSamples {
		end := min(start+goldenRenderChunkSamples, len(rendered))
		engine.Render(rendered[start:end])
	}

	return rendered
}

func float32RenderDigest(rendered []float32) string {
	digest := sha256.New()
	var word [4]byte

	for _, sample := range rendered {
		binary.LittleEndian.PutUint32(word[:], math.Float32bits(sample))
		digest.Write(word[:])
	}

	return hex.EncodeToString(digest.Sum(nil))
}

func renderStepRMS(rendered []float32) [MaxSteps]float64 {
	var rms [MaxSteps]float64

	for step := range MaxSteps {
		window := rendered[step*goldenRenderStepSamples : (step+1)*goldenRenderStepSamples]
		var sumSquares float64

		for _, sample := range window {
			value := float64(sample)
			sumSquares += value * value
		}

		rms[step] = math.Sqrt(sumSquares / float64(len(window)))
	}

	return rms
}
