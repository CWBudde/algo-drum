package drum

import "math"

const (
	masterCeilingDB  = -1.0
	masterLookaheadS = 0.003
	masterReleaseS   = 0.100
)

type peakPoint struct {
	index uint64
	level float64
}

// peakLimiter is a true lookahead peak limiter. The program path is delayed
// while a monotonic deque tracks the largest absolute input over the delayed
// sample's complete lookahead window. Gain reduction is instantaneous and
// recovery is smoothed, so the output cannot cross ceiling even for a
// one-sample noise transient.
type peakLimiter struct {
	ceiling   float64
	release   float64
	gain      float64
	delay     []float64
	delayPos  int
	peaks     []peakPoint
	peakHead  int
	peakCount int
	sample    uint64
	lookahead uint64
}

func newPeakLimiter(sampleRate float64) *peakLimiter {
	lookahead := int(math.Round(sampleRate * masterLookaheadS))
	if lookahead < 1 {
		lookahead = 1
	}

	return &peakLimiter{
		ceiling:   math.Pow(10, masterCeilingDB/20),
		release:   1 - math.Exp(-1/(sampleRate*masterReleaseS)),
		gain:      1,
		delay:     make([]float64, lookahead),
		peaks:     make([]peakPoint, lookahead+2),
		lookahead: uint64(lookahead),
	}
}

func (l *peakLimiter) ProcessSample(input float64) float64 {
	level := math.Abs(input)
	l.pushPeak(peakPoint{index: l.sample, level: level})

	if l.sample > l.lookahead {
		l.expirePeaks(l.sample - l.lookahead)
	}

	delayed := l.delay[l.delayPos]
	l.delay[l.delayPos] = input
	l.delayPos++

	if l.delayPos == len(l.delay) {
		l.delayPos = 0
	}

	target := 1.0
	if peak := l.peaks[l.peakHead].level; peak > l.ceiling {
		target = l.ceiling / peak
	}

	if target < l.gain {
		l.gain = target
	} else {
		l.gain += (target - l.gain) * l.release
	}

	l.sample++

	return delayed * l.gain
}

func (l *peakLimiter) pushPeak(point peakPoint) {
	for l.peakCount > 0 {
		back := (l.peakHead + l.peakCount - 1) % len(l.peaks)
		if l.peaks[back].level > point.level {
			break
		}

		l.peakCount--
	}

	tail := (l.peakHead + l.peakCount) % len(l.peaks)
	l.peaks[tail] = point
	l.peakCount++
}

func (l *peakLimiter) expirePeaks(minIndex uint64) {
	for l.peakCount > 0 && l.peaks[l.peakHead].index < minIndex {
		l.peakHead = (l.peakHead + 1) % len(l.peaks)
		l.peakCount--
	}
}
