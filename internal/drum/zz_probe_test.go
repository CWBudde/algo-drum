package drum

import (
	"math"
	"testing"
)

// Throwaway probe for PLAN.md E7. Not committed.
func TestProbeE7(t *testing.T) {
	const n = 48000 * 3

	// render returns peak, count of samples at the clamp, and RMS.
	render := func(setup func(e *Engine), bypassLimiter bool, volScale float64) (float64, int, float64) {
		e := NewEngine(48000)
		setup(e)

		for tr := range TrackCount {
			e.volumes[tr] *= volScale
			e.liveVol[tr] *= volScale
		}

		if bypassLimiter {
			e.limiter = nil
		}

		e.SetRunning(true)

		buf := make([]float32, n)
		e.Render(buf)

		clipped := 0
		peak := 0.0
		sum := 0.0

		for _, v := range buf {
			a := math.Abs(float64(v))
			if a > peak {
				peak = a
			}

			if a >= 1.0 {
				clipped++
			}

			sum += float64(v) * float64(v)
		}

		return peak, clipped, math.Sqrt(sum / n)
	}

	ordinary := func(e *Engine) {
		for s := 0; s < 16; s += 4 {
			e.SetCell(0, s, 1.0)
		}

		e.SetCell(1, 4, 1.0)
		e.SetCell(1, 12, 1.0)

		for s := 0; s < 16; s += 2 {
			e.SetCell(2, s, 0.7)
		}
	}

	worst := func(e *Engine) {
		for tr := range TrackCount {
			for s := range MaxSteps {
				e.SetCell(tr, s, 1.0)
			}

			e.SetVolume(tr, 1)
			e.SetDecay(tr, 1)
		}

		e.SetTempo(300)
		e.SetReverb(1)
	}

	solo := func(track int) func(e *Engine) {
		return func(e *Engine) {
			for s := 0; s < 16; s += 4 {
				e.SetCell(track, s, 1.0)
			}
		}
	}

	cases := []struct {
		name  string
		setup func(e *Engine)
	}{
		{"solo bass", solo(0)},
		{"solo snare", solo(1)},
		{"solo hat", solo(2)},
		{"solo tom", solo(3)},
		{"solo cymbal", solo(4)},
		{"solo tom2", solo(5)},
		{"solo perc", solo(6)},
		{"bass alone", func(e *Engine) {
			for s := 0; s < 16; s += 4 {
				e.SetCell(0, s, 1.0)
			}
		}},
		{"ordinary rev=0", ordinary},
		{"ordinary rev=0.3", func(e *Engine) { ordinary(e); e.SetReverb(0.3) }},
		{"ordinary rev=1", func(e *Engine) { ordinary(e); e.SetReverb(1) }},
		{"worst case", worst},
	}

	// 1/16 volume keeps the bypassed chain well under the clamp, so the
	// recovered peak is the true pre-limiter level (the path is linear).
	const scale = 1.0 / 16.0

	t.Logf("%-18s | %-26s | %s", "case", "limiter on", "limiter bypassed (true level)")

	for _, c := range cases {
		peakOn, clipOn, rmsOn := render(c.setup, false, 1)
		peakOff, clipOff, rmsOff := render(c.setup, true, scale)

		t.Logf("%-18s | peak=%.4f clip=%-5d rms=%.4f | peak=%.4f (%+.1f dBFS) clip@1/16=%d rms=%.4f",
			c.name, peakOn, clipOn, rmsOn,
			peakOff/scale, 20*math.Log10(peakOff/scale), clipOff, rmsOff/scale)
	}
}
