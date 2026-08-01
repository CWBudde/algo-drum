package match

import (
	"math"
	"slices"
)

// The decay estimator: an exponential standing on a stationary noise floor,
// fitted where measureDecays used to draw a straight line through a truncated
// trace. Split out of features.go, which is the file that finds the partials
// this one then measures.
//
// measureDecays itself moved here in the same spirit when PLAN N17's window
// work pushed features.go past its length limit: what the two files divide is
// finding a partial from measuring how long it rings, and the bounds on what
// counts as a measurable ring time belong on this side of that line.

// decayTraceRateHz is the rate the decay trace is decimated to before the
// exponential-plus-floor refinement. The heterodyne low-passes the envelope at
// `cutoff`, bounded above at 40 Hz in measureDecays, so this is a factor of
// twenty-five clear of that envelope's own bandwidth and the decimation is
// information-preserving rather than a speed/accuracy trade.
const decayTraceRateHz = 2000

// decayFit is one partial's decay read as an exponential standing on a floor.
type decayFit struct {
	slopeDBPerSecond float64
	interceptDB      float64
	rSquared         float64
	// rangeDB is 10*log10(P0/N): how far the exponential falls before the floor
	// overtakes it, and so how much evidence the slope was read from.
	rangeDB float64
}

// decayFloorFit is the estimator of Karjalainen, Antsalo, Mäkivirta, Peltonen &
// Välimäki, "Estimation of Modal Decay Parameters from Noisy Response
// Measurements" (JAES 50(11):867-878, 2002), which exists to replace exactly the
// log-linear-with-truncation fit that seeds it.
//
// The defect it repairs is structural rather than statistical. A measured
// partial is not an exponential; it is an exponential *plus a floor* — the
// recording's noise, the room, the skirts of every other partial that leaked
// past the heterodyne. A straight line fitted to the decibels of that sum is
// biased towards the floor's slope, which is zero, so every ring time comes back
// long. The usual defence is to truncate before the floor arrives, which is what
// DecayFitFloorDB does, and it trades the bias for two new problems: the
// truncation point depends on the very decay being measured, and a fast partial
// is left with almost no trace to fit.
//
// Modelling the floor removes the need for either. The fit is over the whole
// window, floor included:
//
//	p(t) = P0*exp(-a*t) + N,  fitted to 10*log10 of the measured power
//
// in decibels because that is the domain the error is uniform in and the domain
// the term it feeds is stated in. Three parameters, held as logarithms so they
// cannot go negative, solved by Levenberg-Marquardt from the log-linear
// estimate. Deterministic: fixed iteration count, fixed damping schedule, no
// randomness, so a rendered candidate scores identically on every run.
//
// It reports failure rather than a fallback value. The caller keeps the
// log-linear reading when the refinement does not converge on a decaying
// exponential, which is the conservative direction — the seed is what this
// repository measured everything with until now.
func decayFloorFit(times, trace []float64, seedSlope, seedIntercept float64) (decayFit, bool) {
	const (
		maxIterations = 60
		minPoints     = 16
		// Convergence: an improvement smaller than this in the mean squared dB
		// residual is smaller than anything the term downstream can resolve.
		tolerance = 1e-9
	)

	if len(times) < minPoints || seedSlope >= 0 {
		return decayFit{}, false
	}

	// The power decays as exp(-a*t) where the seed's slope is in dB of power
	// per second, so a = -slope * ln(10)/10.
	logPower := seedIntercept * math.Ln10 / 10
	logRate := math.Log(-seedSlope * math.Ln10 / 10)

	// The floor seed is the quietest thing in the trace, backed off by a decade
	// so the first step is taken from below it rather than through it.
	quietest := trace[0]
	for _, level := range trace {
		quietest = min(quietest, level)
	}

	logFloor := quietest*math.Ln10/10 - math.Ln10

	residual := decayFloorResidual(times, trace, logPower, logRate, logFloor)
	damping := 1e-3

	for range maxIterations {
		// Normal equations for the three-parameter Gauss-Newton step, damped.
		var (
			normal [3][3]float64
			right  [3]float64
		)

		for i, elapsed := range times {
			modelled, gradient := decayFloorModel(elapsed, logPower, logRate, logFloor)
			delta := trace[i] - modelled

			for row := range 3 {
				right[row] += gradient[row] * delta

				for column := range 3 {
					normal[row][column] += gradient[row] * gradient[column]
				}
			}
		}

		improved := false

		for range 8 {
			damped := normal
			for row := range 3 {
				damped[row][row] *= 1 + damping
			}

			step, ok := solve3(damped, right)
			if !ok {
				break
			}

			trialPower := logPower + step[0]
			trialRate := logRate + step[1]
			trialFloor := logFloor + step[2]

			trial := decayFloorResidual(times, trace, trialPower, trialRate, trialFloor)
			if trial < residual {
				logPower, logRate, logFloor = trialPower, trialRate, trialFloor
				improved = residual-trial > tolerance*max(1, residual)
				residual = trial
				damping = max(damping/10, 1e-12)

				break
			}

			damping *= 10
		}

		if !improved {
			break
		}
	}

	rate := math.Exp(logRate)
	if !(rate > 0) || math.IsNaN(residual) || math.IsInf(residual, 0) {
		return decayFit{}, false
	}

	// The floor is only identifiable if the trace actually reaches it. A partial
	// still standing above the recording's noise at the end of the window
	// constrains N from above and not at all from below, so the fit drives it to
	// zero and 10*log10(P0/N) runs away — 2e4 dB and worse were observed on the
	// licensed reference before this bound.
	//
	// Reporting the runaway would be reporting a confidence of infinity for a
	// perfectly ordinary partial. What the trace can support is the span it
	// covers, so the range is capped there: no partial is credited with more
	// evidence than its own envelope shows.
	quietestFitted := trace[0]
	loudest := trace[0]

	for _, level := range trace {
		quietestFitted = min(quietestFitted, level)
		loudest = max(loudest, level)
	}

	var variance float64

	mean := 0.0
	for _, level := range trace {
		mean += level
	}

	mean /= float64(len(trace))

	for _, level := range trace {
		variance += (level - mean) * (level - mean)
	}

	quality := 0.0
	if variance > 0 {
		quality = max(0, 1-residual*float64(len(trace))/variance)
	}

	return decayFit{
		slopeDBPerSecond: -rate * 10 / math.Ln10,
		interceptDB:      logPower * 10 / math.Ln10,
		rSquared:         quality,
		rangeDB:          min((logPower-logFloor)*10/math.Ln10, loudest-quietestFitted),
	}, true
}

// decayFloorModel is the fitted level in dB at one time, and its gradient with
// respect to the three log-parameters.
func decayFloorModel(elapsed, logPower, logRate, logFloor float64) (level float64, gradient [3]float64) {
	const perDecibel = 10 / math.Ln10

	signal := math.Exp(logPower - math.Exp(logRate)*elapsed)
	floor := math.Exp(logFloor)

	total := signal + floor
	if total <= 0 {
		return math.Inf(-1), gradient
	}

	signalShare := signal / total

	// d/dlogRate carries the extra factor of rate*t from the chain rule, and its
	// sign is what makes the rate identifiable at all: only the part of the
	// trace where the signal still stands above the floor constrains it.
	gradient[0] = perDecibel * signalShare
	gradient[1] = -perDecibel * signalShare * math.Exp(logRate) * elapsed
	gradient[2] = perDecibel * floor / total

	return perDecibel * math.Log(total), gradient
}

// decayFloorResidual is the mean squared dB error of one parameter set.
func decayFloorResidual(times, trace []float64, logPower, logRate, logFloor float64) float64 {
	sum := 0.0

	for i, elapsed := range times {
		modelled, _ := decayFloorModel(elapsed, logPower, logRate, logFloor)
		if math.IsInf(modelled, 0) {
			return math.Inf(1)
		}

		delta := trace[i] - modelled
		sum += delta * delta
	}

	return sum / float64(len(times))
}

// solve3 solves a 3x3 system by Gaussian elimination with partial pivoting. The
// matrix is a damped Gramian, so it is symmetric positive semi-definite and a
// Cholesky would do; pivoting is used anyway because the damping is what keeps
// it definite and the caller must be told when that has stopped being true.
func solve3(matrix [3][3]float64, right [3]float64) ([3]float64, bool) {
	var solution [3]float64

	for column := range 3 {
		pivot := column
		for row := column + 1; row < 3; row++ {
			if math.Abs(matrix[row][column]) > math.Abs(matrix[pivot][column]) {
				pivot = row
			}
		}

		if matrix[pivot][column] == 0 {
			return solution, false
		}

		matrix[column], matrix[pivot] = matrix[pivot], matrix[column]
		right[column], right[pivot] = right[pivot], right[column]

		for row := column + 1; row < 3; row++ {
			factor := matrix[row][column] / matrix[column][column]
			for inner := column; inner < 3; inner++ {
				matrix[row][inner] -= factor * matrix[column][inner]
			}

			right[row] -= factor * right[column]
		}
	}

	for column := 2; column >= 0; column-- {
		sum := right[column]
		for inner := column + 1; inner < 3; inner++ {
			sum -= matrix[column][inner] * solution[inner]
		}

		solution[column] = sum / matrix[column][column]
	}

	if math.IsNaN(solution[0]) || math.IsNaN(solution[1]) || math.IsNaN(solution[2]) {
		return solution, false
	}

	return solution, true
}

// t60From converts a log-magnitude slope in dB per second into a ring time.
func t60From(slopeDBPerSecond float64) float64 {
	return -60 / slopeDBPerSecond
}

// fastestObservableT60 is the shortest ring time anything the envelope filter
// can output, and so the fastest decay a fit through it can be measuring.
//
// The filter heterodyne builds is a fourth-order Butterworth. Its two pole
// pairs decay at zeta*omega_n with zeta = 1/(2Q), and the faster of them —
// Q = 0.5411961 — sets the bound below: no input whatsoever produces an output
// that falls 60 dB faster than that. A fit reporting one is not measuring a
// short partial, it is reading a fragment of something else, and the level
// implied by extrapolating that slope back to the strike is catastrophic rather
// than merely wrong. That is what this bound is enforced against.
//
// It is deliberately the *faster* pole and not the slower one. The slower pair
// rings for 2.4 times as long — 287 ms at the 10 Hz cutoff floor, longer than
// several of this reference's partials — and it is tempting to argue that a
// partial decaying faster than that cannot be observed either, because the
// filtered tail would be the filter's own transient rather than the partial's.
// That argument is wrong, and it was tested rather than reasoned about:
// TestCloseNeighboursDoNotBiasEitherRingTime puts two partials 18 Hz apart
// ringing for 280 ms through exactly that 10 Hz filter and recovers both to
// within three per cent. The fit is dominated by the signal term, which stands
// far above the filter transient until the -45 dB truncation has already ended
// the fit. Enforcing the slower bound would have discarded this drum's own
// fundamental for no reason at all.
func fastestObservableT60(cutoffHz float64) float64 {
	const dampingRatio = 1 / (2 * 0.5411961)

	return 3 * math.Ln10 / (dampingRatio * 2 * math.Pi * cutoffHz)
}

// minimumEvaluationFallDB is how far a partial must actually fall inside its fit
// window before a ring time may be extrapolated from it.
//
// 20 dB is ISO 3382's own answer. A T60 is a 60 dB fall, and a 60 dB fall is
// almost never observed above the noise, so the standard defines T20 and T30 —
// ring times read off a 20 or 30 dB decay and multiplied out. It sanctions
// nothing shorter than 20 dB, and this is the same judgement applied to a
// partial rather than to a room: below it the extrapolation factor passes three
// and the answer is mostly the estimator's opinion.
const minimumEvaluationFallDB = 20

// slowestSupportedT60 is the longest ring time a fit window of this length can
// carry evidence for, which by the constant above is exactly three times the
// window.
//
// This is the counterpart of fastestObservableT60 and catches the opposite
// failure. That bound rejects a decay too fast for the envelope filter to have
// produced; this one rejects a decay too slow for the window to have seen. The
// two are not symmetric in how they go wrong: an impossibly fast fit announces
// itself with a catastrophic intercept, while an unsupported slow one looks
// entirely ordinary and is simply the fitted line's opinion about audio that was
// never examined.
//
// The failure is not hypothetical. On reference/tt08x08/lp/hd, measured through
// the 0.60 s window this repository shipped until PLAN N17, the ~358 Hz partial
// came back between 5.1 and 10.4 s across the sixteen takes at R² 0.12-0.77 —
// a factor of two of disagreement about one component of one drum. It is a real
// partial: at a 1.60 s window the same component reads 2.48-2.59 s at R²
// 0.95-0.98, a factor of 1.04. Nothing about the drum changed, only whether the
// window contained the decay being reported.
//
// Note what this deliberately does not do. It does not bound a ring time against
// the file's own length, which was the first criterion tried and is wrong: a
// 2.5 s partial in a 2.08 s recording is perfectly measurable if it fell 37 dB
// while the window was open, and twelve of this reference's partials are exactly
// that. Evidence is the fall, not the duration.
func slowestSupportedT60(fitSpanSeconds float64) float64 {
	return 60 * fitSpanSeconds / minimumEvaluationFallDB
}

// measureDecays fits each partial's own decay, and corrects its level for it.
//
// Two measurements, from two different views of the same partial, because
// neither view can do both jobs.
//
// The *rate* comes from the time domain: the signal is heterodyned to
// baseband and low-passed, and the log of the resulting envelope is fitted
// with a straight line.
//
// The *level* is that same fitted line's value at the strike — its intercept.
//
// It must not be the envelope's peak inside the fit window: resolving a pair
// 10 Hz apart needs a filter whose impulse response is longer than 150 ms, and
// the strike transient smeared through one of those put this reference's
// 212 Hz partial 32 dB too loud, above the fundamental. The intercept is not
// that reading. The fit starts at DecayFitStartSeconds, after the transient,
// and the intercept extrapolates the fitted decay back to the origin.
//
// Nor can it be the detection spectrum divided by the attenuation that
// spectrum's window applied to a partial decaying at the fitted rate. That
// divisor is exact for an isolated exponential and hopelessly ill-conditioned:
// across the ring times this reference actually contains it spans 16 dB to
// 100 dB, so the reported level was mostly a restatement of the fitted rate. A
// ten per cent error in T60 at 0.12 s moved the level by 4.5 dB, and a partial
// whose fit came back short was promoted past the fundamental — a 73 ms
// component was reported as the loudest thing in the recording, which pushed
// every genuine partial below the detection floor, because levels are relative
// to the strongest. TestShortPartialsDoNotOutrankLongOnes pins it.
func measureDecays(hit []float64, sampleRateHz float64, options Options, partials []Partial) []Partial {
	levelLinear := make([]float64, len(partials))

	for index := range partials {
		// Half the distance to the nearest neighbour, bounded: too narrow and
		// the filter's own ring outlasts the partial, too wide and the
		// neighbour's decay is measured instead of this one's.
		cutoff := clampFloat(0.5*neighbourSpacing(partials, index), 10, 40)

		inPhase, quadrature := heterodyne(hit, sampleRateHz, partials[index].FrequencyHz, cutoff, 2)

		start := clampIndex(int(options.DecayFitStartSeconds*sampleRateHz), len(hit))

		end := clampIndex(int(options.DecayFitEndSeconds*sampleRateHz), len(hit))
		if end-start < 16 {
			continue
		}

		// Both loops below work in squared magnitude, so neither takes a square
		// root at all — this is not Hypot traded for a cheaper Sqrt.
		//
		// The peak survives because x -> sqrt(x) is increasing, so the largest
		// squared magnitude belongs to the largest magnitude; one root at the end
		// recovers it. The trace survives because 20*log10(sqrt(x)) is 10*log10(x)
		// identically, so the root the decibel conversion was undoing never needs
		// taking. The floor comparison is squared to match, which is exact for
		// non-negative operands.
		peakSquared := 0.0

		for sample := start; sample < end; sample++ {
			squared := inPhase[sample]*inPhase[sample] +
				quadrature[sample]*quadrature[sample]
			if squared > peakSquared {
				peakSquared = squared
			}
		}

		if peakSquared <= 0 {
			continue
		}

		times := make([]float64, 0, end-start)
		trace := make([]float64, 0, end-start)
		floor := math.Sqrt(peakSquared) * math.Pow(10, options.DecayFitFloorDB/20)
		floorSquared := floor * floor

		for sample := start; sample < end; sample++ {
			squared := inPhase[sample]*inPhase[sample] +
				quadrature[sample]*quadrature[sample]
			if squared < floorSquared {
				// Below the fit floor the trace is noise or a neighbour, and
				// including it would flatten every slope towards zero.
				break
			}

			times = append(times, float64(sample)/sampleRateHz)
			trace = append(trace, 10*math.Log10(squared))
		}

		if len(times) < 16 {
			continue
		}

		// The refinement below is fitted past the truncation, floor and all,
		// because in its model the floor is a parameter rather than something to
		// truncate away. But it is not fitted over the *whole* window, and the
		// bound is what keeps a long window from being worse than a short one.
		//
		// Its model is one exponential plus a **stationary** floor. That holds
		// for as long as the band contains this partial and the recording's
		// noise. It stops holding once the partial has gone: what is left in the
		// band is the skirt of whatever else is near it, and a neighbour that
		// rings longer is a second *decaying* exponential, which the model can
		// only account for by bending the first one towards it.
		//
		// The failure is not subtle. Two partials 27.7 Hz apart, the lower
		// ringing 0.21 s and the louder-lived upper one 0.64 s: fitted to a
		// 0.60 s window the lower reads T60 0.240 s at its true level, and
		// fitted to a 1.60 s window it reads 0.443 s at -21.5 dB, because two
		// thirds of that window is the neighbour. The level is the worse of the
		// two errors — it is the fitted line extrapolated back to the strike, so
		// a slope bent flat drags the intercept down with it.
		//
		// So the window is per partial and is stated in the partial's own terms:
		// the span over which it stood above its floor, doubled. One span of
		// decay to fit, one span of floor to identify the floor, and nothing
		// beyond that, where there is no more information about *this* partial
		// and a growing amount about its neighbours. It is still bounded by the
		// global window, which is what a short recording enforces.
		fitEnd := min(end, start+2*len(times))

		// It is decimated first: the envelope has already been low-passed at
		// `cutoff`, which is at most 40 Hz, so at decayTraceRateHz it is
		// oversampled several-fold and the fit runs over hundreds of points
		// rather than tens of thousands. That matters — a fit run scores
		// thousands of candidates, and an undecimated Levenberg-Marquardt here
		// would dominate the whole search.
		step := max(1, int(sampleRateHz/decayTraceRateHz))
		fullTimes := make([]float64, 0, (fitEnd-start)/step+1)
		fullTrace := make([]float64, 0, (fitEnd-start)/step+1)

		for sample := start; sample < fitEnd; sample += step {
			squared := inPhase[sample]*inPhase[sample] +
				quadrature[sample]*quadrature[sample]
			if squared <= 0 {
				continue
			}

			fullTimes = append(fullTimes, float64(sample)/sampleRateHz)
			fullTrace = append(fullTrace, 10*math.Log10(squared))
		}

		slope, interceptDB, fitQuality := linearFit(times, trace)
		// Without the refinement the honest dynamic range is the span the
		// truncated trace actually covered, which the -45 dB floor bounds.
		rangeDB := trace[0] - trace[len(trace)-1]

		if refined, ok := decayFloorFit(fullTimes, fullTrace, slope, interceptDB); ok {
			slope, interceptDB = refined.slopeDBPerSecond, refined.interceptDB
			fitQuality, rangeDB = refined.rSquared, refined.rangeDB
		}

		if slope >= 0 {
			// No decay to speak of: the trace is noise, or a neighbour bleeding
			// through. Neither its level nor its ring time means anything, so
			// the partial is dropped rather than reported with one of them made
			// up.
			continue
		}

		if t60From(slope) < fastestObservableT60(cutoff) {
			// Faster than anything the filter the envelope was measured through
			// can output. See fastestObservableT60: such a fit is reading a
			// fragment of something else, and the intercept it implies is
			// catastrophic rather than merely wrong.
			//
			// On v10 of the licensed reference this one condition is the
			// difference between a table of one partial and a table of sixteen.
			// The 2349.6 Hz candidate there was fitted over 6.1 ms of trace at
			// -4034 dB/s and extrapolated back to an intercept of +137 dB —
			// 144 dB above the loudest real partial — after which every genuine
			// partial sat below the -42 dB relative floor and was discarded.
			// TestImpossibleDecaysDoNotSilenceTheTable pins it.
			continue
		}

		if t60From(slope) > slowestSupportedT60(float64(end-start)/sampleRateHz) {
			// Slower than this window can carry evidence for. See
			// slowestSupportedT60: the partial is dropped rather than reported
			// with a ring time the recording was never examined far enough to
			// support.
			//
			// The span is measured from the clamped sample indices rather than
			// from DecayFitEndSeconds, so a file that ends before the window
			// does is judged on the audio it actually has.
			continue
		}

		// The level is the fitted line's value at the strike, read off the
		// partial's own envelope. The previous form divided the sustain
		// transform's magnitude by that window's attenuation integral, which is
		// exact for an isolated exponential and hopelessly ill-conditioned: over
		// the T60 range this reference actually contains, that divisor spans
		// 16 dB to 100 dB, so the reported level was mostly a restatement of the
		// fitted decay rate. A ten per cent error in T60 at 0.12 s moved the
		// level by 4.5 dB, and a partial whose fit came back short could be
		// promoted past the fundamental — which is what happened, repeatedly,
		// and what made the detection guards load-bearing.
		//
		// The intercept is the same quantity — amplitude at t=0 — measured
		// directly. It extrapolates back only as far as DecayFitStartSeconds
		// rather than through the whole Hann taper, and it is a least-squares
		// fit over hundreds of samples rather than a ratio of two numbers.
		levelLinear[index] = math.Pow(10, interceptDB/20)
		partials[index].T60Seconds = t60From(slope)
		partials[index].FitQuality = fitQuality
		partials[index].DecayRangeDB = rangeDB
	}

	strongest := 0.0
	for _, level := range levelLinear {
		strongest = max(strongest, level)
	}

	if strongest <= 0 {
		return nil
	}

	kept := partials[:0]

	for index := range partials {
		if levelLinear[index] <= 0 {
			continue
		}

		levelDB := 20 * math.Log10(levelLinear[index]/strongest)
		if levelDB < options.PartialFloorDB {
			continue
		}

		partials[index].LevelDB = levelDB
		kept = append(kept, partials[index])
	}

	if len(kept) > options.MaxPartials {
		byLevel := slices.Clone(kept)
		slices.SortFunc(byLevel, func(a, b Partial) int {
			switch {
			case a.LevelDB > b.LevelDB:
				return -1
			case a.LevelDB < b.LevelDB:
				return 1
			default:
				return 0
			}
		})

		kept = byLevel[:options.MaxPartials]
		slices.SortFunc(kept, func(a, b Partial) int {
			switch {
			case a.FrequencyHz < b.FrequencyHz:
				return -1
			case a.FrequencyHz > b.FrequencyHz:
				return 1
			default:
				return 0
			}
		})
	}

	return kept
}
