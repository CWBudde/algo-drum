package match

// The extractor's working memory and its caches, split out of features.go —
// which is the file that decides what to measure, where this one is only about
// not paying to rebuild the same thing per candidate.
//
// Two kinds of reuse live here and they are not interchangeable. The caches are
// shared, immutable and keyed by the parameters they are a function of: a Hann
// window, an FFT plan, a band layout. The scratch is mutable and private to one
// Extract call, because cmd/fit-physical has one goroutine per restart inside
// Extract at once. TestExtractIsConcurrencySafe is the licence for the second.

import (
	"fmt"
	"slices"
	"sync"

	"github.com/cwbudde/algo-dsp/dsp/spectrum"
	"github.com/cwbudde/algo-dsp/dsp/window"
	algofft "github.com/cwbudde/algo-fft"
)

// hannWindows caches periodic Hann windows by length, and realPlans caches real
// FFT plans by size.
//
// Both are functions of a length alone, and a fitting run measures many
// thousands of candidates at the same handful of lengths — so without a cache
// every measurement pays to rebuild the identical window (a cosine per sample)
// and the identical twiddle tables. algo-fft documents plans as safe for
// concurrent transforms, which is what lets one plan serve every restart
// goroutine.
var (
	hannWindows sync.Map // int -> []float64
	realPlans   sync.Map // int -> *algofft.PlanReal[float64, complex128]
)

// hannWindow returns a cached periodic Hann window of the given length. The
// result is shared and must not be modified by the caller.
func hannWindow(length int) []float64 {
	if cached, ok := hannWindows.Load(length); ok {
		coefficients, _ := cached.([]float64)

		return coefficients
	}

	coefficients := window.Generate(window.TypeHann, length, window.WithPeriodic())
	shared, _ := hannWindows.LoadOrStore(length, coefficients)
	result, _ := shared.([]float64)

	return result
}

// realPlan returns a cached real FFT plan for the given size.
func realPlan(size int) (*algofft.PlanReal[float64, complex128], error) {
	if cached, ok := realPlans.Load(size); ok {
		plan, _ := cached.(*algofft.PlanReal[float64, complex128])

		return plan, nil
	}

	plan, err := algofft.NewPlanReal64(size)
	if err != nil {
		return nil, err
	}

	shared, _ := realPlans.LoadOrStore(size, plan)
	result, _ := shared.(*algofft.PlanReal[float64, complex128])

	return result, nil
}

// extractScratch is the working memory one Extract call reuses across its
// partials, its transforms and its decay traces.
//
// The buffers here are what made an extraction cost tens of megabytes: a
// heterodyne allocates two arrays the length of the whole hit and runs once per
// detected partial, a magnitude spectrum allocates three arrays the length of
// the transform and runs seven times, and each partial's decay builds four
// traces at the full window's capacity. Every one of them is scratch — nothing
// that reaches Features points into any of it — so one set per extraction
// serves them all.
//
// It cannot be a package-level buffer: cmd/fit-physical runs one goroutine per
// restart and every one of them is inside Extract at once. It is per call, from
// a pool, so the sharing is between successive evaluations rather than between
// concurrent ones and the buffers still come back sized from the last hit.
type extractScratch struct {
	hit        []float64
	inPhase    []float64
	quadrature []float64

	fftInput     []float64
	fftBins      []complex128
	fftReal      []float64
	fftImaginary []float64
	magnitude    []float64

	envelope  []float64
	times     []float64
	trace     []float64
	fullTimes []float64
	fullTrace []float64
}

var extractScratchPool sync.Pool

func acquireExtractScratch() *extractScratch {
	if held, ok := extractScratchPool.Get().(*extractScratch); ok {
		return held
	}

	return &extractScratch{}
}

func releaseExtractScratch(work *extractScratch) {
	extractScratchPool.Put(work)
}

// growFloats returns a slice of exactly length, reusing buffer's array when it
// is large enough. The contents are whatever the previous user left; every call
// site below either overwrites them or clears what it does not.
func growFloats(buffer []float64, length int) []float64 {
	if cap(buffer) < length {
		return make([]float64, length)
	}

	return buffer[:length]
}

// basebandPair returns the two heterodyne buffers, sized to the hit. Their
// contents are stale; heterodyneInto overwrites both in full.
func (work *extractScratch) basebandPair(length int) (inPhase, quadrature []float64) {
	work.inPhase = growFloats(work.inPhase, length)
	work.quadrature = growFloats(work.quadrature, length)

	return work.inPhase, work.quadrature
}

// magnitudeSpectrum is a Hann-windowed, zero-padded real FFT magnitude,
// allocating its result. Extract goes through spectrumInto instead; this form
// is what the cache-equivalence test measures against.
func magnitudeSpectrum(segment []float64, fftSize int) ([]float64, error) {
	var work extractScratch

	magnitude, err := work.spectrum(segment, fftSize)
	if err != nil {
		return nil, err
	}

	return slices.Clone(magnitude), nil
}

// spectrum is magnitudeSpectrum into the scratch buffers. The returned slice is
// owned by work and is valid only until the next call.
func (work *extractScratch) spectrum(segment []float64, fftSize int) ([]float64, error) {
	if len(segment) == 0 {
		return nil, fmt.Errorf("%w: empty segment", ErrInvalidOptions)
	}

	size := min(len(segment), fftSize)
	coefficients := hannWindow(size)

	input := growFloats(work.fftInput, fftSize)
	work.fftInput = input

	for i := range size {
		input[i] = segment[i] * coefficients[i]
	}

	// Only the zero padding needs clearing: the head is overwritten in full
	// above, so a reused buffer can only carry stale values past the segment.
	clear(input[size:])

	plan, err := realPlan(fftSize)
	if err != nil {
		return nil, err
	}

	length := plan.SpectrumLen()
	if cap(work.fftBins) < length {
		work.fftBins = make([]complex128, length)
	}

	bins := work.fftBins[:length]

	if err := plan.Forward(bins, input); err != nil {
		return nil, err
	}

	// spectrum.Magnitude would allocate its result; MagnitudeFromParts is the
	// same vectorized kernel writing into a caller's buffer, so the bins are
	// split into parts here rather than a magnitude being allocated per
	// transform. TestCachedSpectrumMatchesFreshBuild holds the two forms
	// together bit for bit.
	parts, imaginary := growFloats(work.fftReal, length), growFloats(work.fftImaginary, length)
	work.fftReal, work.fftImaginary = parts, imaginary

	for i, bin := range bins {
		parts[i], imaginary[i] = real(bin), imag(bin)
	}

	magnitude := growFloats(work.magnitude, length)
	work.magnitude = magnitude

	spectrum.MagnitudeFromParts(magnitude, parts, imaginary)

	return magnitude, nil
}

// bandLayouts caches fractionalOctaveBands by the four numbers it is a function
// of, the way hannWindows caches a window by its length.
//
// The layout is the same two dozen bands for every candidate of a whole fit
// run, and it was being rebuilt — a math.Pow, a Sqrt and two growing slices —
// once per extraction.
var bandLayouts sync.Map // bandLayoutKey -> bandLayout

type bandLayoutKey struct {
	minHz, maxHz, sampleRateHz float64
	perOctave                  int
}

type bandLayout struct {
	centres []float64
	edges   [][2]float64
}

// cachedFractionalOctaveBands returns the shared layout. The edges are read-only
// scratch for the caller; the centres are copied, because they reach Features
// and a caller owns what it is given.
func cachedFractionalOctaveBands(minHz, maxHz float64, perOctave int, sampleRateHz float64) ([]float64, [][2]float64) {
	key := bandLayoutKey{minHz: minHz, maxHz: maxHz, sampleRateHz: sampleRateHz, perOctave: perOctave}

	cached, ok := bandLayouts.Load(key)
	if !ok {
		centres, edges := fractionalOctaveBands(minHz, maxHz, perOctave, sampleRateHz)
		cached, _ = bandLayouts.LoadOrStore(key, bandLayout{centres: centres, edges: edges})
	}

	layout, _ := cached.(bandLayout)

	return slices.Clone(layout.centres), layout.edges
}
