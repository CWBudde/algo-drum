package main

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"

	"github.com/cwbudde/algo-drum/internal/physical/match"
)

type extraction struct {
	file     string
	partials []match.Partial
}

func extractAll(files []string, analysis, fitEnd float64) []extraction {
	options := match.DefaultOptions()
	options.AnalysisSeconds = analysis
	options.DecayFitEndSeconds = fitEnd

	var out []extraction

	for _, file := range files {
		ref, err := match.LoadReference(file, match.ChannelMono)
		if err != nil {
			continue
		}

		features, err := match.Extract(ref.Samples, ref.SampleRateHz, options)
		if err != nil {
			continue
		}

		out = append(out, extraction{filepath.Base(file), features.Partials})
	}

	return out
}

// stability matches partial-to-partial by frequency within 0.5% and reports the
// median |log2 T60 ratio| between two window settings. Band medians cannot be
// compared across runs because the partial sets differ.
func stability(a, b []extraction, lowCut float64) (float64, int) {
	var ratios []float64

	for i := range a {
		if i >= len(b) || a[i].file != b[i].file {
			continue
		}

		for _, pa := range a[i].partials {
			if pa.T60Seconds <= 0 || pa.FrequencyHz > lowCut {
				continue
			}

			for _, pb := range b[i].partials {
				if pb.T60Seconds <= 0 {
					continue
				}

				if math.Abs(pb.FrequencyHz/pa.FrequencyHz-1) < 0.005 {
					ratios = append(ratios, math.Abs(math.Log2(pb.T60Seconds/pa.T60Seconds)))

					break
				}
			}
		}
	}

	sort.Float64s(ratios)

	if len(ratios) == 0 {
		return math.NaN(), 0
	}

	return ratios[len(ratios)/2], len(ratios)
}

func reportStability(files []string) {
	fmt.Println("=== window-to-window stability, partial-matched within 0.5% ===")
	fmt.Println("median |log2 T60 ratio| as a percentage; low band is < 1 kHz")
	fmt.Println()

	type window struct {
		analysis, fitEnd float64
	}

	windows := []window{{1.2, 0.60}, {1.6, 0.90}, {2.0, 1.20}, {2.0, 1.60}, {2.0, 1.90}}

	runs := make([][]extraction, len(windows))
	for i, w := range windows {
		runs[i] = extractAll(files, w.analysis, w.fitEnd)
	}

	for i := range windows[:len(windows)-1] {
		lowMedian, lowN := stability(runs[i], runs[i+1], 1000)
		allMedian, allN := stability(runs[i], runs[i+1], math.Inf(1))

		fmt.Printf("  %.2f s -> %.2f s : low %5.1f%% (n=%3d)   all %5.1f%% (n=%3d)\n",
			windows[i].fitEnd, windows[i+1].fitEnd,
			100*(math.Pow(2, lowMedian)-1), lowN,
			100*(math.Pow(2, allMedian)-1), allN)
	}

	fmt.Println()
}
