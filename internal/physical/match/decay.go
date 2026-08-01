package match

import "math"

// The decay estimator: an exponential standing on a stationary noise floor,
// fitted where measureDecays used to draw a straight line through a truncated
// trace. Split out of features.go, which is the file that finds the partials
// this one then measures.

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
