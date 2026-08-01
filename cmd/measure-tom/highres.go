package main

import (
	"fmt"
	"io"
	"math"
	"slices"
	"text/tabwriter"

	"github.com/cwbudde/algo-drum/internal/physical/match"
)

// The second estimator, and the comparison between the two.
//
// PLAN.md's N2 requires that the subspace estimator be compared partial by
// partial against the table the fast one produces *before* anything is refitted
// against either. This is that comparison, in the tool that already writes the
// committable tables, so that the comparison is a measurement session rather
// than a script somebody ran once.
//
// What it is for: the fast estimator cannot resolve two modes closer together
// than match.Options.MinSeparationHz, and does not report that it could not. So
// the interesting output below is not the frequency agreement — which is
// expected to be good — but the merged column, which counts the places where
// one partial in the fast table is two modes in the signal.

// HighResolution is one take measured again with subband ESPRIT, and how the
// two measurements compare.
type HighResolution struct {
	Options  match.EspritOptions `json:"options"`
	Partials []HighResolutionRow `json:"partials"`

	Agreement Agreement `json:"agreement"`
}

// HighResolutionRow is Row plus what only the subspace estimator can say.
type HighResolutionRow struct {
	Row

	// Order is how many components the subband was fitted with, Support how
	// many model orders this one survived, and EsterOrder what ESTER would
	// have chosen for the band — reported so that the disagreement between the
	// two order-selection rules is in the committed table.
	Order        int     `json:"order"`
	Support      int     `json:"support"`
	EsterOrder   int     `json:"esterOrder"`
	BandCentreHz float64 `json:"bandCentreHz"`
}

// Agreement is the partial-by-partial comparison of the two estimators.
type Agreement struct {
	// MatchWindowHz is the window a fast partial and a subspace component are
	// paired within. It is the fast estimator's own separation guard, because
	// that is precisely the distance inside which it cannot tell two things
	// apart, and so the distance inside which a disagreement is not evidence of
	// an error in either.
	MatchWindowHz float64 `json:"matchWindowHz"`

	Paired []Pairing `json:"paired"`

	// Merged lists the fast partials that stand for more than one subspace
	// component. This is the count N2's first defect predicts is non-zero.
	Merged []Merge `json:"merged,omitempty"`

	// The partials each estimator reports and the other does not.
	OnlyFastHz           []float64 `json:"onlyFastHz,omitempty"`
	OnlyHighResolutionHz []float64 `json:"onlyHighResolutionHz,omitempty"`

	// Medians over the paired partials. Medians rather than means because a
	// single mispairing at the edge of the window would otherwise decide them.
	MedianFrequencyCents float64 `json:"medianAbsFrequencyCents"`
	MedianT60Percent     float64 `json:"medianAbsT60Percent"`

	// LevelOffsetDB is the median signed level difference, and is not an error.
	// Each estimator states its levels relative to its own strongest partial,
	// and the two do not agree on which partial that is — the fast one cannot
	// see some of what the subspace one finds, and the reporting floor then
	// falls in a different place. The offset is removed before MedianLevelDB is
	// taken, so that a difference in the normalising partial is not counted as
	// sixteen partials disagreeing.
	LevelOffsetDB float64 `json:"levelOffsetDB"`
	MedianLevelDB float64 `json:"medianAbsLevelDB"`
}

// Pairing is one partial as the two estimators each saw it.
type Pairing struct {
	FastHz           float64 `json:"fastHz"`
	HighResolutionHz float64 `json:"highResolutionHz"`
	Cents            float64 `json:"cents"`

	FastT60           float64 `json:"fastT60Seconds"`
	HighResolutionT60 float64 `json:"highResolutionT60Seconds"`
	// T60Percent is the fast estimator's ring time relative to the subspace
	// one, as a signed percentage. A truncated log-linear fit over a decay that
	// is not a single exponential is expected to be short.
	T60Percent float64 `json:"t60Percent"`

	FastLevelDB           float64 `json:"fastLevelDB"`
	HighResolutionLevelDB float64 `json:"highResolutionLevelDB"`

	// FastFitQuality is carried so that the question "does R² predict where the
	// two estimators disagree?" can be answered from the report. If it does
	// not, R² is not the guard it is being used as.
	//
	// It does not — see docs/physical-objective-validation.md §5c — and
	// FastDecayRangeDB is the candidate that replaced it, carried here for the
	// same question so that the replacement is measured on the same evidence
	// rather than argued for.
	FastFitQuality   float64 `json:"fastFitQuality"`
	FastDecayRangeDB float64 `json:"fastDecayRangeDB"`
}

// Merge is one fast partial that the subspace estimator says is several modes.
type Merge struct {
	FastHz     float64 `json:"fastHz"`
	FastT60    float64 `json:"fastT60Seconds"`
	FastR2     float64 `json:"fastFitQuality"`
	Components []Row   `json:"components"`
	// SpreadHz is the widest gap between the components, so that a merge of two
	// modes 1 Hz apart is not read as the same finding as one of two 12 Hz
	// apart.
	SpreadHz float64 `json:"spreadHz"`
}

// measureHighResolution runs the subspace estimator over a take and compares it
// against the fast table already measured.
func measureHighResolution(reference match.Reference, fast []Row, baseHz float64,
	options match.Options, esprit match.EspritOptions,
) (*HighResolution, error) {
	found, err := match.ExtractHighResolution(reference.Samples, reference.SampleRateHz,
		options, esprit)
	if err != nil {
		return nil, err
	}

	partials := make([]HighResolutionRow, 0, len(found))

	for _, component := range found {
		row := rows([]match.Partial{component.Partial}, baseHz)[0]

		partials = append(partials, HighResolutionRow{
			Row:          row,
			Order:        component.Order,
			Support:      component.Support,
			EsterOrder:   component.EsterOrder,
			BandCentreHz: component.BandCentreHz,
		})
	}

	return &HighResolution{
		Options:   esprit,
		Partials:  partials,
		Agreement: compare(fast, partials, options.MinSeparationHz),
	}, nil
}

// compare pairs the two tables inside the fast estimator's separation guard.
func compare(fast []Row, high []HighResolutionRow, windowHz float64) Agreement {
	agreement := Agreement{MatchWindowHz: windowHz}

	claimed := make([]bool, len(high))

	var cents, ringTimes, levels []float64

	for _, partial := range fast {
		var inWindow []int

		for index, component := range high {
			if math.Abs(component.FrequencyHz-partial.FrequencyHz) <= windowHz {
				inWindow = append(inWindow, index)
			}
		}

		if len(inWindow) == 0 {
			agreement.OnlyFastHz = append(agreement.OnlyFastHz, partial.FrequencyHz)

			continue
		}

		// The fast partial stands for whichever component is loudest, since
		// that is the one its peak was found at.
		loudest := inWindow[0]
		for _, index := range inWindow {
			if high[index].LevelDB > high[loudest].LevelDB {
				loudest = index
			}
		}

		for _, index := range inWindow {
			claimed[index] = true
		}

		counterpart := high[loudest]

		pairing := Pairing{
			FastHz:                partial.FrequencyHz,
			HighResolutionHz:      counterpart.FrequencyHz,
			Cents:                 1200 * math.Log2(partial.FrequencyHz/counterpart.FrequencyHz),
			FastT60:               partial.T60Seconds,
			HighResolutionT60:     counterpart.T60Seconds,
			FastLevelDB:           partial.LevelDB,
			HighResolutionLevelDB: counterpart.LevelDB,
			FastFitQuality:        partial.FitQuality,
			FastDecayRangeDB:      partial.DecayRangeDB,
		}

		if counterpart.T60Seconds > 0 {
			pairing.T60Percent = 100 * (partial.T60Seconds - counterpart.T60Seconds) /
				counterpart.T60Seconds
			ringTimes = append(ringTimes, math.Abs(pairing.T60Percent))
		}

		cents = append(cents, math.Abs(pairing.Cents))
		levels = append(levels, pairing.FastLevelDB-pairing.HighResolutionLevelDB)

		agreement.Paired = append(agreement.Paired, pairing)

		if len(inWindow) > 1 {
			agreement.Merged = append(agreement.Merged, mergeOf(partial, high, inWindow))
		}
	}

	for index, component := range high {
		if !claimed[index] {
			agreement.OnlyHighResolutionHz = append(agreement.OnlyHighResolutionHz,
				component.FrequencyHz)
		}
	}

	agreement.MedianFrequencyCents = median(cents)
	agreement.MedianT60Percent = median(ringTimes)

	agreement.LevelOffsetDB = median(levels)

	spread := make([]float64, len(levels))
	for index, difference := range levels {
		spread[index] = math.Abs(difference - agreement.LevelOffsetDB)
	}

	agreement.MedianLevelDB = median(spread)

	return agreement
}

// mergeOf describes one fast partial that several subspace components fell
// inside.
func mergeOf(partial Row, high []HighResolutionRow, inWindow []int) Merge {
	merge := Merge{
		FastHz:  partial.FrequencyHz,
		FastT60: partial.T60Seconds,
		FastR2:  partial.FitQuality,
	}

	lowest, highest := math.Inf(1), math.Inf(-1)

	for _, index := range inWindow {
		merge.Components = append(merge.Components, high[index].Row)
		lowest = min(lowest, high[index].FrequencyHz)
		highest = max(highest, high[index].FrequencyHz)
	}

	merge.SpreadHz = highest - lowest

	return merge
}

// median returns the middle value, or zero for an empty sample.
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := slices.Clone(values)
	slices.Sort(sorted)

	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}

	return (sorted[middle-1] + sorted[middle]) / 2
}

func printHighResolution(writer io.Writer, high HighResolution) {
	_, _ = fmt.Fprintln(writer, "   -- subband ESPRIT")

	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)

	_, _ = fmt.Fprintln(table,
		"   #\tf (Hz)\tlevel (dB)\tf/f0\tT60 (s)\tζ (%)\tfit\tord\tsup\tESTER\tband")

	for index, row := range high.Partials {
		_, _ = fmt.Fprintf(table, "   %d\t%.2f\t%.1f\t%.3f\t%s\t%s\t%.2f\t%d\t%d\t%d\t%.0f\n",
			index+1, row.FrequencyHz, row.LevelDB, row.RatioToBase,
			optional(row.T60Seconds, "%.3f"), optional(row.DampingRatioPercent, "%.2f"),
			row.FitQuality, row.Order, row.Support, row.EsterOrder, row.BandCentreHz)
	}

	_ = table.Flush()

	agreement := high.Agreement

	_, _ = fmt.Fprintf(writer,
		"   -- agreement (paired within %.1f Hz): %d paired, %d merged, %d only fast, %d only ESPRIT\n",
		agreement.MatchWindowHz, len(agreement.Paired), len(agreement.Merged),
		len(agreement.OnlyFastHz), len(agreement.OnlyHighResolutionHz))

	_, _ = fmt.Fprintf(writer,
		"      median |Δf| %.1f cents, median |ΔT60| %.1f %%, level offset %+.1f dB with median |Δ| %.1f dB\n",
		agreement.MedianFrequencyCents, agreement.MedianT60Percent,
		agreement.LevelOffsetDB, agreement.MedianLevelDB)

	for _, merge := range agreement.Merged {
		_, _ = fmt.Fprintf(writer,
			"      ! %.2f Hz (T60 %.3f s, R²=%.2f) is %d modes spanning %.2f Hz:",
			merge.FastHz, merge.FastT60, merge.FastR2, len(merge.Components), merge.SpreadHz)

		for _, component := range merge.Components {
			_, _ = fmt.Fprintf(writer, " %.2f Hz/%.3f s", component.FrequencyHz, component.T60Seconds)
		}

		_, _ = fmt.Fprintln(writer)
	}
}
