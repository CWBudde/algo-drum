package match

import (
	"math"
	"testing"
)

// decayTrace builds the decibel trace of one partial whose power is exactly an
// exponential standing on a stationary floor — the model decayFloorFit fits, so
// the answer it should return is known to the digit.
//
// It is sampled the way measureDecays samples it: from DecayFitStartSeconds to
// DecayFitEndSeconds at decayTraceRateHz.
func decayTrace(t60Seconds, floorBelowDB float64) (times, trace []float64) {
	const (
		start = 0.05
		end   = 0.60
	)

	rate := 6 * math.Ln10 / t60Seconds
	floor := math.Pow(10, -floorBelowDB/10)

	for at := start; at < end; at += 1.0 / decayTraceRateHz {
		times = append(times, at)
		trace = append(trace, 10*math.Log10(math.Exp(-rate*at)+floor))
	}

	return times, trace
}

// truncatedLogLinear is what measureDecays did before the Karjalainen
// refinement, and still does to seed it: cut the trace at DecayFitFloorDB below
// its own peak and draw a straight line through what is left.
func truncatedLogLinear(times, trace []float64) (slope, intercept float64) {
	peak := trace[0]
	for _, level := range trace {
		peak = max(peak, level)
	}

	floor := peak + DefaultOptions().DecayFitFloorDB

	var keptTimes, keptTrace []float64

	for i, level := range trace {
		if level < floor {
			break
		}

		keptTimes = append(keptTimes, times[i])
		keptTrace = append(keptTrace, level)
	}

	slope, intercept, _ = linearFit(keptTimes, keptTrace)

	return slope, intercept
}

// TestTruncationReadsARingTimeLongAndTheFloorModelDoesNot is the measurement
// that justifies the estimator, and it is written as a comparison rather than as
// a tolerance on the new one alone: what matters is not that the floor model is
// accurate in the abstract but that it is accurate where the thing it replaced
// is not.
//
// The configuration is an ordinary one. A partial 30 dB above its own noise
// floor cannot show 45 dB of decay, so the truncation never fires, and the
// straight line is drawn through a trace whose last third has already flattened
// onto the floor. There is nothing pathological here — this is what a quiet
// partial in a real recording looks like.
func TestTruncationReadsARingTimeLongAndTheFloorModelDoesNot(t *testing.T) {
	t.Parallel()

	const (
		wantT60      = 0.30
		floorBelowDB = 30
	)

	times, trace := decayTrace(wantT60, floorBelowDB)

	slope, intercept := truncatedLogLinear(times, trace)

	truncated := t60From(slope)
	if truncated <= wantT60*1.10 {
		t.Fatalf("log-linear T60 = %.4f s against a true %.4f s: the premise of this "+
			"test is that truncation reads long, and it did not", truncated, wantT60)
	}

	fit, ok := decayFloorFit(times, trace, slope, intercept)
	if !ok {
		t.Fatal("decayFloorFit() declined a trace that is exactly its own model")
	}

	refined := t60From(fit.slopeDBPerSecond)
	if ratio := refined / wantT60; ratio < 0.99 || ratio > 1.01 {
		t.Errorf("floor-model T60 = %.4f s, want %.4f s (ratio %.4f)", refined, wantT60, ratio)
	}

	t.Logf("true %.4f s; log-linear %.4f s (%+.1f %%); floor model %.4f s (%+.1f %%)",
		wantT60, truncated, 100*(truncated/wantT60-1), refined, 100*(refined/wantT60-1))
}

// TestTheFloorModelAgreesWithTruncationWhereTruncationWorks is the other half of
// the claim. A partial standing 60 dB above its floor is one the old estimator
// handled correctly, and a replacement that moved those readings would be
// trading one bias for another rather than removing one.
func TestTheFloorModelAgreesWithTruncationWhereTruncationWorks(t *testing.T) {
	t.Parallel()

	const wantT60 = 0.30

	times, trace := decayTrace(wantT60, 60)

	slope, intercept := truncatedLogLinear(times, trace)

	truncated := t60From(slope)
	if ratio := truncated / wantT60; ratio < 0.98 || ratio > 1.02 {
		t.Fatalf("log-linear T60 = %.4f s, want %.4f s: the premise is that truncation "+
			"is right here", truncated, wantT60)
	}

	fit, ok := decayFloorFit(times, trace, slope, intercept)
	if !ok {
		t.Fatal("decayFloorFit() declined a clean exponential")
	}

	if ratio := t60From(fit.slopeDBPerSecond) / wantT60; ratio < 0.99 || ratio > 1.01 {
		t.Errorf("floor-model T60 = %.4f s, want %.4f s", t60From(fit.slopeDBPerSecond), wantT60)
	}
}

// TestTheFittedRangeIsTheEvidenceTheSlopeWasReadFrom pins DecayRangeDB against
// the quantity it is defined as, on traces built with that quantity known.
//
// It was proposed as the decay term's confidence weighting, so it had to mean
// what it says before the question of whether it discriminates could even be
// asked. It was then measured against subband ESPRIT, it does not discriminate,
// and the term weights by nothing — but the field is still reported, and this is
// what pins what it reports.
func TestTheFittedRangeIsTheEvidenceTheSlopeWasReadFrom(t *testing.T) {
	t.Parallel()

	// The window opens at DecayFitStartSeconds, by which time a partial with a
	// T60 of 0.30 s has already fallen 60 * 0.05 / 0.30 = 10 dB. That decay was
	// not observed, so it is not evidence, and the range excludes it: what is
	// reported is how far the partial fell *inside the window* before the floor
	// caught it.
	for _, floorBelowDB := range []float64{20, 30, 45, 60} {
		times, trace := decayTrace(0.30, floorBelowDB)

		slope, intercept := truncatedLogLinear(times, trace)

		fit, ok := decayFloorFit(times, trace, slope, intercept)
		if !ok {
			t.Fatalf("decayFloorFit() declined the %.0f dB trace", floorBelowDB)
		}

		if want := floorBelowDB - 10; math.Abs(fit.rangeDB-want) > 1 {
			t.Errorf("range = %.2f dB, want %.0f dB", fit.rangeDB, want)
		}
	}
}

// TestAnUnreachedFloorIsNotInfiniteConfidence is the numerical failure the cap
// exists for. A partial still standing above the recording's noise when the
// window closes leaves the floor unconstrained from below, so the fit drives it
// to zero and 10*log10(P0/N) runs away — values above 1e4 dB, and one above
// 1e12 dB, were observed on the licensed reference before the cap.
func TestAnUnreachedFloorIsNotInfiniteConfidence(t *testing.T) {
	t.Parallel()

	// A floor 200 dB down is never reached inside the window: the partial has
	// fallen 110 dB by the time it closes.
	times, trace := decayTrace(0.30, 200)

	slope, intercept := truncatedLogLinear(times, trace)

	fit, ok := decayFloorFit(times, trace, slope, intercept)
	if !ok {
		t.Fatal("decayFloorFit() declined a clean exponential")
	}

	if fit.rangeDB > 115 {
		t.Errorf("range = %.4g dB, want no more than the ~110 dB the trace covers", fit.rangeDB)
	}
}

// TestTheFloorModelDeclinesRatherThanInventingADecay covers the two inputs a
// fit loop will hand it that contain no decay at all. Reporting failure is what
// lets measureDecays keep its own guards in charge of what happens next.
func TestTheFloorModelDeclinesRatherThanInventingADecay(t *testing.T) {
	t.Parallel()

	times, trace := decayTrace(0.30, 30)

	if _, ok := decayFloorFit(times, trace, 0, 0); ok {
		t.Error("decayFloorFit() accepted a non-negative seed slope")
	}

	if _, ok := decayFloorFit(times[:4], trace[:4], -100, 0); ok {
		t.Error("decayFloorFit() accepted four points")
	}
}

// TestAQuietPartialIsNoLongerReportedAsRingingLongest is the end-to-end form:
// the same defect, seen through Extract, on a signal with a real noise floor
// rather than a synthetic one.
//
// The 512 Hz partial is the quietest and the fastest, so it is the one whose
// trace flattens onto the noise first, and under truncation it was the one whose
// ring time came back longest relative to the truth.
func TestAQuietPartialIsNoLongerReportedAsRingingLongest(t *testing.T) {
	t.Parallel()

	tones := wellSeparatedTones()

	features, err := Extract(
		synthesizeNoisy(tones, testSampleRate, 1.5, -70), testSampleRate, DefaultOptions(),
	)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if len(features.Partials) != len(tones) {
		t.Fatalf("partials = %d, want %d", len(features.Partials), len(tones))
	}

	for i, want := range tones {
		got := features.Partials[i]

		if ratio := got.T60Seconds / want.t60Seconds; ratio < 0.94 || ratio > 1.06 {
			t.Errorf("partial %d (%.0f Hz) T60 = %.4f s, want %.4f s (ratio %.3f)",
				i, want.frequencyHz, got.T60Seconds, want.t60Seconds, ratio)
		}

		// Every one of these stands well clear of the noise, so the confidence
		// the decay term reads must say so.
		if got.DecayRangeDB < 20 {
			t.Errorf("partial %d (%.0f Hz) decay range = %.1f dB, want a well-measured partial",
				i, want.frequencyHz, got.DecayRangeDB)
		}
	}
}
