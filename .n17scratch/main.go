// Sizes the analysis and decay-fit windows for a reference set (PLAN N17).
//
// For each candidate window end it re-extracts every take and reports, per
// partial, the ring time against the evidence the window actually contains:
// the fall in dB across the fit span. A T60 much longer than its window is not
// automatically wrong -- it is wrong when the trace barely fell.
package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/cwbudde/algo-drum/internal/physical/match"
)

type row struct {
	file    string
	freq    float64
	t60     float64
	rangeDB float64
	r2      float64
	levelDB float64
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	idx := int(p * float64(len(sorted)-1))
	return sorted[idx]
}

func main() {
	files, err := filepath.Glob(os.Args[1])
	if err != nil || len(files) == 0 {
		fmt.Println("no files", err)
		os.Exit(1)
	}
	sort.Strings(files)

	// (analysisSeconds, decayFitEndSeconds) pairs to try.
	type window struct{ analysis, fitEnd float64 }
	windows := []window{
		{1.2, 0.60}, // shipped
		{1.6, 0.90},
		{2.0, 1.20},
		{2.0, 1.60},
	}

	fmt.Printf("%d takes\n\n", len(files))

	reportStability(files)

	for _, w := range windows {
		options := match.DefaultOptions()
		options.AnalysisSeconds = w.analysis
		options.DecayFitEndSeconds = w.fitEnd
		fitSpan := w.fitEnd - options.DecayFitStartSeconds

		var rows []row
		for _, file := range files {
			ref, err := match.LoadReference(file, match.ChannelMono)
			if err != nil {
				fmt.Println("load", file, err)
				continue
			}
			features, err := match.Extract(ref.Samples, ref.SampleRateHz, options)
			if err != nil {
				fmt.Println("extract", file, err)
				continue
			}
			for _, p := range features.Partials {
				if p.T60Seconds > 0 {
					rows = append(rows, row{
						filepath.Base(file), p.FrequencyHz,
						p.T60Seconds, p.DecayRangeDB, p.FitQuality, p.LevelDB,
					})
				}
			}
		}

		// How far does the fitted exponential fall across the fit span?
		// 60 dB per T60, so fall = 60 * span / T60.
		var falls, t60s []float64
		beyondSpan, beyondFile := 0, 0
		const fileSeconds = 2.08

		for _, r := range rows {
			fall := 60 * fitSpan / r.t60
			falls = append(falls, fall)
			t60s = append(t60s, r.t60)
			if r.t60 > fitSpan {
				beyondSpan++
			}
			if r.t60 > fileSeconds {
				beyondFile++
			}
		}
		sort.Float64s(falls)
		sort.Float64s(t60s)

		fmt.Printf("=== analysis %.2f s, fit %.2f..%.2f s (span %.2f s) ===\n",
			w.analysis, options.DecayFitStartSeconds, w.fitEnd, fitSpan)
		fmt.Printf("  partials with a decay: %d\n", len(rows))
		fmt.Printf("  T60 > fit span : %d (%.1f%%)\n", beyondSpan, 100*float64(beyondSpan)/float64(len(rows)))
		fmt.Printf("  T60 > file     : %d (%.1f%%)\n", beyondFile, 100*float64(beyondFile)/float64(len(rows)))
		fmt.Printf("  T60      p05=%.3f p50=%.3f p95=%.3f max=%.3f s\n",
			percentile(t60s, 0.05), percentile(t60s, 0.50), percentile(t60s, 0.95), t60s[len(t60s)-1])
		fmt.Printf("  implied fall across span: p05=%.1f p25=%.1f p50=%.1f dB\n",
			percentile(falls, 0.05), percentile(falls, 0.25), percentile(falls, 0.50))

		// The worst offenders, so the bound is chosen against real cases.
		sort.Slice(rows, func(i, j int) bool { return rows[i].t60 > rows[j].t60 })
		fmt.Printf("  longest ring times:\n")
		for i := 0; i < 6 && i < len(rows); i++ {
			r := rows[i]
			fmt.Printf("    %-10s %7.1f Hz  T60 %7.3f s  fall %5.1f dB  range %5.1f dB  R2 %.2f  level %6.1f dB\n",
				r.file, r.freq, r.t60, 60*fitSpan/r.t60, r.rangeDB, r.r2, r.levelDB)
		}
		fmt.Println()
	}
}
