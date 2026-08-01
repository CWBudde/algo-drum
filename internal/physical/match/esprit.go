package match

import (
	"fmt"
	"math"
	"math/cmplx"
	"slices"

	"github.com/cwbudde/algo-dsp/dsp/filter/biquad"
	"github.com/cwbudde/algo-dsp/dsp/filter/design"
)

// Subband ESPRIT with ESTER order selection: a second, independent measurement
// of the same partials detectPartials and measureDecays report.
//
// It exists because the fast estimator is resolution-limited in a way no
// parameter of it can fix. Detection reads an interpolated FFT peak over an
// 800 ms Hann window, whose main lobe is 5 Hz wide, and then refuses any second
// peak within MinSeparationHz of a peak already taken. A degenerate pair split
// by TensionAsymmetry sits 4.3 Hz apart at 213 Hz — inside one main lobe, and
// inside the separation guard by a factor of three. The pair is not merely
// unresolved: it is reported as a single partial whose envelope beats, and the
// log-linear decay fit run over a beating envelope terminates at the first null
// and returns a slope with a high R², so FitQuality does not flag it.
//
// ESPRIT is not resolution-limited that way. It fits a sum of damped complex
// exponentials to the signal directly, and the accuracy with which it separates
// two of them is set by the noise, not by an observation window. That is the
// whole reason to carry a second estimator: this one can say what the fast one
// cannot see, and the fast one is the only one cheap enough to run inside a
// fit.
//
// This is measurement equipment, not part of the objective. Nothing in
// cmd/fit-physical's inner loop calls it — an extraction here costs seconds,
// against the milliseconds a fit budgets per candidate — and Distance does not
// know it exists. Its job is to establish what is true, so that the fast
// estimator can be repaired against it and its residual error stated rather
// than assumed.
//
// Method and order selection follow Ege, Boutillon & David, "Vibroacoustics of
// the piano soundboard: (Non)linearity and modal properties in the low- and
// mid-frequency ranges", J. Sound Vib. 325(4-5):639-664 (2009), which is where
// the subband decomposition in front of ESPRIT comes from, and Badeau, David &
// Richard, "A new perturbation analysis for signal enumeration in rotational
// invariance techniques", IEEE Trans. Signal Process. 54(2):450-458 (2006),
// which is ESTER.

// EspritOptions controls the high-resolution estimator. The zero value is not
// usable; start from DefaultEspritOptions.
type EspritOptions struct {
	// The band the estimator sweeps, matching Options.MinFrequencyHz and
	// MaxFrequencyHz so the two estimators are asked the same question.
	MinFrequencyHz float64 `json:"minFrequencyHz"`
	MaxFrequencyHz float64 `json:"maxFrequencyHz"`

	// BandsPerOctave sets how finely that sweep is cut into subbands. This is
	// the estimator's main cost/resolution control: a narrower band means a
	// lower decimated rate for the same number of samples, so more of the
	// signal reaches the model, and fewer components have to share one order.
	BandsPerOctave float64 `json:"bandsPerOctave"`

	// The span of the hit the model is fitted over, from the onset. The start
	// clears the strike transient exactly as DecayFitStartSeconds does; the
	// subband filter's own settling is added to it per band, since it depends
	// on that band's width.
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds"`

	// MaxOrder bounds how many damped exponentials one subband may be given.
	// The stabilisation sweep runs every order up to it, so this is a cost
	// control as well as a ceiling.
	MaxOrder int `json:"maxOrder"`

	// Support is how many of those orders a component must appear at, within
	// the tolerances below, before it is reported. See selectStable: this, and
	// not ESTER, is what decides the model order here, and the reason is
	// measured rather than assumed.
	Support int `json:"support"`

	// The tolerances a component is tracked across orders with. A physical mode
	// barely moves as the order is raised; a component the fit invented to
	// absorb noise moves a great deal.
	StabilityCents      float64 `json:"stabilityCents"`
	StabilityT60Percent float64 `json:"stabilityT60Percent"`

	// A component is discarded unless its ring time falls inside these bounds.
	// Below the floor it is a filter transient or a click; above the ceiling it
	// is an undamped artefact of a nearly-singular fit, not a drum mode.
	MinT60Seconds float64 `json:"minT60Seconds"`
	MaxT60Seconds float64 `json:"maxT60Seconds"`

	// FloorDB is how far below the strongest component a component may sit and
	// still be reported, matching Options.PartialFloorDB.
	FloorDB float64 `json:"floorDB"`
}

// DefaultEspritOptions mirrors DefaultOptions where the two estimators measure
// the same thing, so that a disagreement between them is a disagreement about
// the signal rather than about what was asked.
func DefaultEspritOptions() EspritOptions {
	defaults := DefaultOptions()

	return EspritOptions{
		MinFrequencyHz: defaults.MinFrequencyHz,
		MaxFrequencyHz: defaults.MaxFrequencyHz,
		// Half-octave bands. At the bottom of the sweep that is 25 Hz wide,
		// which is what puts a 4 Hz split comfortably inside one band with an
		// order to spare; at the top it is wide enough that the decimated
		// segment still holds enough samples to fit from.
		BandsPerOctave: 2,

		StartSeconds: defaults.DecayFitStartSeconds,
		EndSeconds:   defaults.DecayFitEndSeconds,

		// Generous on purpose. A half-octave band of this reference holds three
		// or four modes, but the subband filter only attenuates its neighbours
		// rather than removing them, and a strong partial an octave away still
		// costs an order to represent. Sixteen leaves room for both, and the
		// stabilisation sweep is what decides which of them are real.
		MaxOrder: 16,

		// Four of sixteen orders. Low enough that a mode which only separates
		// from its neighbour once the order is high still qualifies; high
		// enough that a component appearing at one order and gone at the next
		// does not.
		Support: 4,
		// A physical mode moves by a few cents as the order is raised. 20 is
		// wide enough to track one through the order at which its neighbour
		// splits off — where it does move — and, since it bounds the whole
		// cluster's width rather than the step between neighbours, comfortably
		// narrower than the 4 Hz splits this estimator exists to resolve, which
		// at 200 Hz are 35 cents.
		StabilityCents: 20,
		// Ring time is the noisier of the two, so its tolerance is wide. It is
		// there to reject components whose decay swings by a factor of two
		// between orders, not to pin the value.
		StabilityT60Percent: 40,

		// 20 ms is shorter than anything the modal bank produces and longer
		// than the subband filter's own transient. The ceiling is deliberately
		// close to the analysis span: a ring time much longer than the span
		// is an extrapolation from a decay that never visibly decayed, and
		// reporting one as a measurement would be a fiction.
		MinT60Seconds: 0.02,
		MaxT60Seconds: 3,

		FloorDB: defaults.PartialFloorDB,
	}
}

// HighResolutionPartial is one damped exponential the subspace estimator found,
// carrying the same four fields Partial does so that the two estimators' tables
// can be compared directly, plus what only this one can report.
type HighResolutionPartial struct {
	Partial

	// BandCentreHz identifies the subband the component was found in, which is
	// what makes a duplicate across an overlap visible.
	BandCentreHz float64 `json:"bandCentreHz"`

	// Order is how many exponentials the subband was fitted with. A component
	// reported at order 2 or more in a band the fast estimator reports one
	// partial in is the split-pair case this estimator was built to see.
	Order int `json:"order"`

	// Support is how many model orders the component survived — the evidence
	// that it is a mode of the drum rather than a component the fit invented.
	Support int `json:"support"`

	// EsterOrder is the order ESTER's criterion would have chosen for this
	// band, and is reported rather than used. See selectStable.
	EsterOrder int `json:"esterOrder"`
}

// ErrEsprit reports a high-resolution extraction that could not be performed.
var ErrEsprit = fmt.Errorf("%w: esprit", ErrInvalidOptions)

// ExtractHighResolution measures the partials of one hit with subband ESPRIT.
//
// samples may contain leading silence; the onset is found and the analysis is
// anchored to it, by the same code path Extract uses, so the two estimators are
// reading the same span of the same signal.
//
// Partial.FitQuality carries the fraction of the subband's energy the fitted
// exponentials account for, which is the closest analogue to the log-linear
// fit's R² this method has. Partial.LevelDB is extrapolated back to the onset
// from the fitted amplitude and decay, so it means what measureDecays' fitted
// intercept means.
func ExtractHighResolution(samples []float64, sampleRateHz float64,
	options Options, esprit EspritOptions,
) ([]HighResolutionPartial, error) {
	if err := esprit.validate(sampleRateHz); err != nil {
		return nil, err
	}

	hit, _, err := onsetAlignedHit(samples, sampleRateHz, options)
	if err != nil {
		return nil, err
	}

	var found []HighResolutionPartial

	for _, band := range espritBands(esprit) {
		found = append(found, measureBand(hit, sampleRateHz, esprit, band)...)
	}

	return finishHighResolution(found, esprit), nil
}

func (o EspritOptions) validate(sampleRateHz float64) error {
	switch {
	case !(sampleRateHz > 0) || math.IsInf(sampleRateHz, 0):
		return fmt.Errorf("%w: sample rate %v", ErrEsprit, sampleRateHz)
	case o.MinFrequencyHz <= 0 || o.MaxFrequencyHz <= o.MinFrequencyHz:
		return fmt.Errorf("%w: band %v..%v Hz", ErrEsprit, o.MinFrequencyHz, o.MaxFrequencyHz)
	case o.MaxFrequencyHz >= sampleRateHz/2:
		return fmt.Errorf("%w: %v Hz is at or above Nyquist", ErrEsprit, o.MaxFrequencyHz)
	case o.BandsPerOctave <= 0:
		return fmt.Errorf("%w: bands per octave %v", ErrEsprit, o.BandsPerOctave)
	case o.EndSeconds <= o.StartSeconds:
		return fmt.Errorf("%w: span %v..%v s", ErrEsprit, o.StartSeconds, o.EndSeconds)
	case o.MaxOrder < 1:
		return fmt.Errorf("%w: max order %d", ErrEsprit, o.MaxOrder)
	case o.MinT60Seconds <= 0 || o.MaxT60Seconds <= o.MinT60Seconds:
		return fmt.Errorf("%w: ring times %v..%v s", ErrEsprit, o.MinT60Seconds, o.MaxT60Seconds)
	}

	return nil
}

// espritBand is one subband of the sweep.
type espritBand struct {
	lowHz, highHz float64
	centreHz      float64
	// cutoffHz is the baseband low-pass corner, wider than half the band so
	// that a component sitting on a band edge is not attenuated by the very
	// filter that admits it.
	cutoffHz float64
}

// bandGuard is how far past the band's own half-width the baseband filter's
// corner is placed. At the edge of the band a component then sits at 1/1.6 of
// the corner, where a fourth-order Butterworth is 0.2 dB down — small enough
// that the gain correction applied later is a correction rather than a rescue.
const bandGuard = 1.6

// espritBands cuts the sweep into geometrically spaced subbands.
func espritBands(options EspritOptions) []espritBand {
	ratio := math.Pow(2, 1/options.BandsPerOctave)

	var bands []espritBand

	for low := options.MinFrequencyHz; low < options.MaxFrequencyHz; low *= ratio {
		high := min(low*ratio, options.MaxFrequencyHz)
		if high <= low {
			break
		}

		bands = append(bands, espritBand{
			lowHz:  low,
			highHz: high,
			// The geometric centre, so that the band is symmetric in the
			// coordinate the bands are laid out in.
			centreHz: math.Sqrt(low * high),
			cutoffHz: bandGuard * (high - low) / 2,
		})
	}

	return bands
}

// measureBand runs the estimator over one subband.
func measureBand(hit []float64, sampleRateHz float64,
	options EspritOptions, band espritBand,
) []HighResolutionPartial {
	baseband, decimatedRateHz, startSeconds, gain := bandBaseband(hit, sampleRateHz, options, band)
	if len(baseband) < 32 {
		return nil
	}

	// L, the number of rows of the Hankel matrix, is the subspace dimension the
	// estimate is drawn from. A third of the segment is the usual choice and is
	// what maximises resolution for a given segment; the cap keeps the
	// eigendecomposition small on the wide upper bands, where the extra rows
	// buy nothing because the order is small anyway.
	rows := min(len(baseband)/3, 48)
	if rows < options.MaxOrder+2 {
		rows = min(len(baseband)/2, options.MaxOrder+2)
	}

	if rows < 4 || rows >= len(baseband) {
		return nil
	}

	_, vectors := hermitianEigen(covariance(baseband, rows))

	// hermitianEigen returns ascending eigenvalues, so the signal subspace is
	// at the far end.
	slices.Reverse(vectors)

	maxOrder := min(options.MaxOrder, rows-2)

	stable := selectStable(vectors, maxOrder, band, decimatedRateHz, options)
	if len(stable) == 0 {
		return nil
	}

	poles := make([]complex128, len(stable))
	for index, component := range stable {
		poles[index] = component.pole(band, decimatedRateHz)
	}

	amplitudes, fit := fitAmplitudes(baseband, poles)

	return bandComponents(stable, amplitudes, bandResult{
		band:            band,
		decimatedRateHz: decimatedRateHz,
		startSeconds:    startSeconds,
		gain:            gain,
		order:           len(stable),
		esterOrder:      esterOrder(vectors, maxOrder),
		fit:             fit,
	}, options)
}

// bandResult groups what measureBand knows about a subband so that converting
// its poles into partials does not need eight positional arguments.
type bandResult struct {
	band            espritBand
	decimatedRateHz float64
	startSeconds    float64
	// gain evaluates the baseband filter's magnitude response, so that a
	// component's level can be corrected for the filter that admitted it.
	gain       func(offsetHz float64) float64
	order      int
	esterOrder int
	fit        float64
}

// bandBaseband heterodynes the band to zero frequency, low-passes it causally,
// discards the filter's own settling transient and decimates.
//
// Causally, and this is the one place in this package that matters. Everything
// else here filters forwards and backwards for zero phase, which is right when
// the quantity wanted is an envelope or a phase slope. It is wrong here: the
// backward pass applied to a decaying exponential produces an anti-causal
// response, and a subspace method asked to explain the result would fit the
// filter's poles alongside the drum's. A causal filter leaves the poles exactly
// where they were and adds a transient that decays at the filter's own rate,
// which is why that transient is skipped rather than corrected.
func bandBaseband(hit []float64, sampleRateHz float64,
	options EspritOptions, band espritBand,
) (baseband []complex128, decimatedRateHz, startSeconds float64, gain func(float64) float64) {
	// A fourth-order Butterworth: two sections is the same selectivity
	// measureDecays uses, and its stopband is what sets how far the signal can
	// be decimated.
	coefficients := []biquad.Coefficients{
		design.Lowpass(band.cutoffHz, 0.5411961, sampleRateHz),
		design.Lowpass(band.cutoffHz, 1.3065630, sampleRateHz),
	}

	inPhase := biquad.NewChain(coefficients)
	quadrature := biquad.NewChain(coefficients)

	step := -2 * math.Pi * band.centreHz / sampleRateHz

	mixed := make([]complex128, len(hit))

	for index, sample := range hit {
		sine, cosine := math.Sincos(step * float64(index))
		mixed[index] = complex(inPhase.ProcessSample(sample*cosine),
			quadrature.ProcessSample(sample*sine))
	}

	// Decimate to eight times the corner, so that everything folded back by the
	// decimation came from at least four times the corner, where the filter is
	// 48 dB down. Aliased content is not merely quiet in a subspace method: it
	// is a pole the model would otherwise have to spend an order on.
	decimation := max(1, int(sampleRateHz/(8*band.cutoffHz)))
	decimatedRateHz = sampleRateHz / float64(decimation)

	// The settling allowance is six time constants of the slower of the two
	// sections, whose pole sits at a damping ratio of 1/(2Q).
	settleSeconds := 6 * 1.3065630 / (math.Pi * band.cutoffHz)
	startSeconds = options.StartSeconds + settleSeconds

	first := int(startSeconds * sampleRateHz)
	last := min(int(options.EndSeconds*sampleRateHz), len(mixed))

	for index := first; index < last; index += decimation {
		baseband = append(baseband, mixed[index])
	}

	chain := biquad.NewChain(coefficients)

	return baseband, decimatedRateHz, startSeconds, func(offsetHz float64) float64 {
		return cmplx.Abs(chain.Response(offsetHz, sampleRateHz))
	}
}

// covariance builds the sample covariance of the Hankel data matrix whose rows
// are `rows` successive lags of the segment.
//
// R = X X^H with X[i][j] = signal[i+j], formed directly rather than by
// assembling X first: the product is what the eigendecomposition wants and X
// itself is never needed.
func covariance(signal []complex128, rows int) [][]complex128 {
	columns := len(signal) - rows + 1

	matrix := make([][]complex128, rows)
	for row := range rows {
		matrix[row] = make([]complex128, rows)
	}

	for row := range rows {
		for column := row; column < rows; column++ {
			var sum complex128
			for k := range columns {
				sum += signal[row+k] * cmplx.Conj(signal[column+k])
			}

			matrix[row][column] = sum
			matrix[column][row] = cmplx.Conj(sum)
		}
	}

	return matrix
}

// esterOrder is ESTER: the order whose signal subspace best satisfies the
// rotational invariance the method assumes.
//
// It is computed and reported, and it is deliberately not what selects the
// model order here. That is a measurement, not a preference. ESTER's criterion
// needs no threshold and no noise estimate, which is what makes it attractive,
// but it assumes the noise subspace is white, and on a struck drum it is not:
// the stochastic attack layer, the room and the residual glide are all
// coloured and none is small in the first tenths of a second. On the 960-1358
// Hz band of this repository's reference the criterion runs
//
//	33.7  32.0  20.9  9.3  9.9  15.7  16.7  25.5  21.5  25.4  19.8  11.2   (dB)
//
// whose argmax is order 1 — for a band in which the fast estimator alone finds
// four partials. It is not near-unimodal with a wrong peak; it is not unimodal.
// Adopting the argmax would have thrown away most of the drum.
//
// So the order is decided by selectStable instead, and this is kept so that a
// reader can see the disagreement rather than take the claim on trust.
func esterOrder(vectors [][]complex128, maxOrder int) int {
	best, bestCriterion := 0, 0.0

	for candidate := 1; candidate <= maxOrder; candidate++ {
		lower, upper := shiftedBlocks(vectors[:candidate])

		rotation := solveLeastSquares(lower, upper)
		if rotation == nil {
			continue
		}

		residual := 0.0

		for row := range upper {
			for column := range candidate {
				predicted := complex128(0)
				for k := range candidate {
					predicted += lower[row][k] * rotation[k][column]
				}

				magnitude := cmplx.Abs(upper[row][column] - predicted)
				residual += magnitude * magnitude
			}
		}

		if residual <= 0 {
			// An exactly invariant subspace: nothing can beat it, and the
			// reciprocal below would be an infinity.
			return candidate
		}

		if criterion := 1 / residual; criterion > bestCriterion {
			best, bestCriterion = candidate, criterion
		}
	}

	return best
}

// stableComponent is one damped exponential the stabilisation sweep found,
// described in physical units rather than as a pole so that the tolerances it
// is tracked with can be stated in cents and per cent.
type stableComponent struct {
	frequencyHz float64
	t60Seconds  float64
	// support is how many distinct model orders produced it.
	support int
}

// pole reconstructs the discrete-time pole from the clustered estimate.
func (c stableComponent) pole(band espritBand, decimatedRateHz float64) complex128 {
	decayPerSecond := 3 * math.Ln10 / c.t60Seconds
	angle := 2 * math.Pi * (c.frequencyHz - band.centreHz) / decimatedRateHz

	return cmplx.Exp(complex(-decayPerSecond/decimatedRateHz, angle))
}

// selectStable decides the model order the way experimental modal analysis
// decides it: by running every order and keeping the components that do not
// move.
//
// A stabilisation diagram is the standard answer to exactly the problem ESTER
// fails at above — an unknown number of modes, in noise that is not white, with
// no threshold anybody can defend. A pole that corresponds to a mode of the
// drum sits at very nearly the same frequency and decay whichever order the
// model is fitted at, because it is in the data. A pole the fit invented to
// absorb noise moves, because there is nothing holding it anywhere. Requiring a
// component to survive several orders separates the two without needing to know
// the noise level, and the requirement is stated in cents and per cent rather
// than in units of a variance nobody has measured.
//
// The components are returned as cluster medians rather than as the estimates
// from any single order, which is what makes this more accurate than picking an
// order and trusting it: the highest order is where a real pole is most
// perturbed by the noise directions dragged in beside it.
func selectStable(vectors [][]complex128, maxOrder int, band espritBand,
	decimatedRateHz float64, options EspritOptions,
) []stableComponent {
	var observations []observation

	for order := 1; order <= maxOrder; order++ {
		for _, pole := range espritPoles(vectors[:order]) {
			magnitude := cmplx.Abs(pole)
			if magnitude <= 0 || magnitude >= 1 {
				continue
			}

			frequencyHz := band.centreHz +
				cmplx.Phase(pole)*decimatedRateHz/(2*math.Pi)
			if frequencyHz <= 0 {
				continue
			}

			t60 := 3 * math.Ln10 / (-math.Log(magnitude) * decimatedRateHz)
			if t60 < options.MinT60Seconds || t60 > options.MaxT60Seconds {
				continue
			}

			observations = append(observations, observation{
				frequencyHz: frequencyHz, t60Seconds: t60, order: order,
			})
		}
	}

	if len(observations) == 0 {
		return nil
	}

	slices.SortFunc(observations, func(a, b observation) int {
		switch {
		case a.frequencyHz < b.frequencyHz:
			return -1
		case a.frequencyHz > b.frequencyHz:
			return 1
		default:
			return 0
		}
	})

	var (
		stable  []stableComponent
		cluster []observation
	)

	flush := func() {
		if len(cluster) == 0 {
			return
		}

		if component, ok := clusterComponent(cluster, options); ok {
			stable = append(stable, component)
		}

		cluster = cluster[:0]
	}

	for _, current := range observations {
		if len(cluster) > 0 {
			// Two conditions, and the second one is load-bearing. Breaking only
			// on the gap to the previous observation is single-linkage
			// clustering, which chains: a degenerate pair 35 cents apart is
			// bridged into one cluster by the intermediate estimates the low
			// orders produce, and the split this estimator exists to resolve
			// disappears at the last step. Bounding the cluster's own width
			// stops that. TestHighResolutionResolvesADegenerateSplit is the
			// case, and it failed exactly this way before the width bound.
			previous := cluster[len(cluster)-1]
			first := cluster[0]

			if 1200*math.Log2(current.frequencyHz/previous.frequencyHz) > options.StabilityCents ||
				1200*math.Log2(current.frequencyHz/first.frequencyHz) > options.StabilityCents {
				flush()
			}
		}

		cluster = append(cluster, current)
	}

	flush()

	return stable
}

// observation is one pole seen at one trial order, in physical units.
type observation struct {
	frequencyHz float64
	t60Seconds  float64
	order       int
}

// clusterComponent reduces one frequency cluster to a component, or rejects it
// for want of support.
//
// The ring time is the awkward half. Frequency clusters cleanly, but a cluster
// can contain one order that put the mode's decay somewhere unrelated, so the
// representative is the median rather than the mean, and an order whose ring
// time is far from that median does not count towards the support. A component
// supported only by orders that disagree about how fast it decays has not been
// measured.
func clusterComponent(cluster []observation, options EspritOptions) (stableComponent, bool) {
	ringTimes := make([]float64, len(cluster))
	for index, current := range cluster {
		ringTimes[index] = current.t60Seconds
	}

	middle := medianOf(ringTimes)

	orders := make(map[int]bool, len(cluster))

	var frequencies, agreeing []float64

	for _, current := range cluster {
		if 100*math.Abs(current.t60Seconds-middle)/middle > options.StabilityT60Percent {
			continue
		}

		orders[current.order] = true

		frequencies = append(frequencies, current.frequencyHz)
		agreeing = append(agreeing, current.t60Seconds)
	}

	if len(orders) < options.Support {
		return stableComponent{}, false
	}

	return stableComponent{
		frequencyHz: medianOf(frequencies),
		t60Seconds:  medianOf(agreeing),
		support:     len(orders),
	}, true
}

// medianOf returns the middle value of a non-empty sample. The input is sorted
// in place, which every caller here is done with.
func medianOf(values []float64) float64 {
	slices.Sort(values)

	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}

	return (values[middle-1] + values[middle]) / 2
}

// shiftedBlocks returns the signal subspace basis with its last row removed and
// with its first row removed, as matrices in row-major order.
//
// These are the two halves of the rotational invariance: a subspace spanned by
// damped exponentials satisfies lower·psi = upper exactly, with psi similar to
// the diagonal matrix of the poles. Everything ESPRIT and ESTER do follows from
// that one identity.
func shiftedBlocks(vectors [][]complex128) (lower, upper [][]complex128) {
	order := len(vectors)
	rows := len(vectors[0])

	lower = make([][]complex128, rows-1)
	upper = make([][]complex128, rows-1)

	for row := range rows - 1 {
		lower[row] = make([]complex128, order)
		upper[row] = make([]complex128, order)

		for column := range order {
			lower[row][column] = vectors[column][row]
			upper[row][column] = vectors[column][row+1]
		}
	}

	return lower, upper
}

// espritPoles returns the poles of the subspace: the eigenvalues of the
// rotation that maps its shifted halves onto each other.
func espritPoles(vectors [][]complex128) []complex128 {
	lower, upper := shiftedBlocks(vectors)

	rotation := solveLeastSquares(lower, upper)
	if rotation == nil {
		return nil
	}

	return eigenvalues(rotation)
}

// fitAmplitudes solves for the complex amplitude of each pole in the least
// squares sense, and reports the fraction of the segment's energy the fitted
// exponentials account for.
func fitAmplitudes(signal []complex128, poles []complex128) (amplitudes []complex128, fit float64) {
	basis := make([][]complex128, len(signal))

	running := make([]complex128, len(poles))
	for index := range running {
		running[index] = 1
	}

	for sample := range signal {
		basis[sample] = make([]complex128, len(poles))
		copy(basis[sample], running)

		for index, pole := range poles {
			running[index] *= pole
		}
	}

	target := make([][]complex128, len(signal))
	for sample := range signal {
		target[sample] = []complex128{signal[sample]}
	}

	solution := solveLeastSquares(basis, target)
	if solution == nil {
		return nil, 0
	}

	amplitudes = make([]complex128, len(poles))
	for index := range poles {
		amplitudes[index] = solution[index][0]
	}

	var residual, energy float64

	for sample := range signal {
		predicted := complex128(0)
		for index := range poles {
			predicted += basis[sample][index] * amplitudes[index]
		}

		difference := cmplx.Abs(signal[sample] - predicted)
		residual += difference * difference

		magnitude := cmplx.Abs(signal[sample])
		energy += magnitude * magnitude
	}

	if energy > 0 {
		fit = max(0, 1-residual/energy)
	}

	return amplitudes, fit
}

// bandComponents turns a subband's poles and amplitudes into partials in the
// units the rest of this package speaks, discarding what cannot be a mode.
func bandComponents(stable []stableComponent, amplitudes []complex128,
	result bandResult, options EspritOptions,
) []HighResolutionPartial {
	if amplitudes == nil {
		return nil
	}

	var found []HighResolutionPartial

	for index, component := range stable {
		frequencyHz := component.frequencyHz

		if frequencyHz < result.band.lowHz || frequencyHz >= result.band.highHz {
			// Outside the band's own span. Either it belongs to a neighbour,
			// which will report it from a filter that did not attenuate it, or
			// it is the model reaching into the stopband. Either way it stayed
			// in the amplitude fit, so that it does not distort the components
			// that are reported.
			continue
		}

		offsetHz := frequencyHz - result.band.centreHz
		decayPerSecond := 3 * math.Ln10 / component.t60Seconds

		gain := result.gain(offsetHz)
		if gain <= 0 {
			continue
		}

		// Two corrections, and both are needed for this level to mean what
		// measureDecays' fitted intercept means. The filter's own gain at the
		// component's offset is divided out, and the amplitude — which is the
		// fitted value at the start of the analysis segment — is extrapolated
		// back along its own decay to the onset.
		level := cmplx.Abs(amplitudes[index]) / gain *
			math.Exp(decayPerSecond*result.startSeconds)

		if level <= 0 {
			continue
		}

		found = append(found, HighResolutionPartial{
			Partial: Partial{
				FrequencyHz: frequencyHz,
				// Held in linear units until the whole sweep is in and the
				// strongest component is known; finishHighResolution converts.
				LevelDB:    level,
				T60Seconds: component.t60Seconds,
				FitQuality: result.fit,
			},
			BandCentreHz: result.band.centreHz,
			Order:        result.order,
			Support:      component.support,
			EsterOrder:   result.esterOrder,
		})
	}

	return found
}

// finishHighResolution converts levels to decibels relative to the strongest
// component, applies the reporting floor and orders the table by frequency.
func finishHighResolution(found []HighResolutionPartial,
	options EspritOptions,
) []HighResolutionPartial {
	strongest := 0.0
	for _, component := range found {
		strongest = max(strongest, component.LevelDB)
	}

	if strongest <= 0 {
		return nil
	}

	kept := found[:0]

	for _, component := range found {
		component.LevelDB = 20 * math.Log10(component.LevelDB/strongest)
		if component.LevelDB < options.FloorDB {
			continue
		}

		kept = append(kept, component)
	}

	slices.SortFunc(kept, func(a, b HighResolutionPartial) int {
		switch {
		case a.FrequencyHz < b.FrequencyHz:
			return -1
		case a.FrequencyHz > b.FrequencyHz:
			return 1
		default:
			return 0
		}
	})

	return kept
}
