package main

import (
	"fmt"
	"io"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/cwbudde/algo-drum/internal/physical/series"
)

// The takes read as an ordered set, rather than as a bag of repeats.
//
// measureRepeatability answers "how far apart are these takes", which is the
// right question for hits at one nominal dynamic and the wrong one for a
// deliberate velocity ramp: there a clean monotone trend reads as enormous
// spread, and the tool would report the instrument as unstable for doing
// exactly what the player asked of it. Everything here answers the other
// question — do the takes go somewhere, and do they all contain the same modes.
//
// **The take order is a claim, not a measurement.** The v01…v16 labelling was
// played by hand and written on the files afterwards; nothing in the audio
// carries it. A correlation reported here is therefore evidence *about* that
// claim, and must never be turned round and used as an assumption of it. That
// is also why it is rank correlation: see the reasoning on series.Spearman.

// Series is the trend of each measured quantity against the take index.
type Series struct {
	Takes int `json:"takes"`
	// Order is the take order the trends were taken against, echoed because the
	// coefficients below are meaningless without knowing what claim they are
	// evidence about. It is the order the files were named on the command line.
	Order  []string `json:"order"`
	Trends []Trend  `json:"trends"`
}

// Trend is one quantity's rank correlation against the take index.
type Trend struct {
	Quantity string `json:"quantity"`
	// Pairs is how many takes contributed. It is below Series.Takes whenever a
	// take had no reading of the quantity — a fundamental that was never
	// identified, a decay the fit did not converge on — and the missing takes
	// are dropped rather than interpolated, keeping the remaining indices.
	// Dropping preserves the order the coefficient is about; filling would
	// invent takes that were never played.
	Pairs int `json:"pairs"`
	// Spearman is present only when Measured; a zero coefficient and an
	// unmeasurable one are different findings and are not both reported as 0.
	Spearman float64 `json:"spearman,omitempty"`
	Measured bool    `json:"measured"`
	Note     string  `json:"note,omitempty"`
}

// Correspondence aligns the per-take partial tables into common rows, so that a
// mode can be seen to be present in some takes and absent in others.
//
// Nothing in the per-take tables carries this: they are sixteen independent
// lists of peaks, and asking whether the 255.7 Hz peak in v10 is "the same
// partial" as the one in v12 was, until this existed, done by reading two tables
// side by side. That is how the finding this table was written for stayed
// invisible — on reference/tt08x08/lp/hd a partial at 255.7 Hz is absent from
// all nine of v01–v09 and present in six of the seven of v10–v16 at a level
// consistent to sd 0.8 dB, while the 358 Hz partial one row up is present in
// twelve takes at levels scattering over 14.8 dB. A mode that switches on above
// a strike level is a nonlinear signature and not a positional one, so the loud
// takes are not the quiet takes scaled up; no per-take table can show that.
type Correspondence struct {
	// ToleranceHz is how far a partial may sit from a row's running centre and
	// still join it. See correspondenceTolerance for the choice.
	ToleranceHz float64 `json:"toleranceHz"`
	// Takes labels the columns, in the order the files were given.
	Takes []string  `json:"takes"`
	Modes []ModeRow `json:"modes"`
}

// ModeRow is one aligned mode across every take.
type ModeRow struct {
	MeanHz   float64 `json:"meanFrequencyHz"`
	SpreadHz float64 `json:"frequencySpreadHz"`
	Present  int     `json:"presentInTakes"`

	// The level statistics are over the takes the mode is present in, and only
	// those: a mode absent from a take has no level there, and counting the
	// absence as the detection floor would turn "not detected" into a
	// measurement of how quiet it was.
	MeanLevelDB   float64 `json:"meanLevelDB"`
	LevelSDDB     float64 `json:"levelSDDB"`
	LevelSpreadDB float64 `json:"levelSpreadDB"`

	// LevelDB is one cell per take, in Correspondence.Takes order, relative to
	// that take's own fundamental rather than to its strongest partial — so a
	// cell is a balance against the note, and survives both the per-file peak
	// normalisation and a take whose loudest mode is not its lowest. A nil
	// entry is a mode the detector did not find in that take.
	LevelDB []*float64 `json:"levelDB"`
}

// correspondenceTolerance is how far a partial may sit from a row's running
// centre and still be called the same mode.
//
// It is half of match.Options.MinSeparationHz, which is not a coincidence and
// not a taste: within one take the peak picker guarantees no two partials closer
// than MinSeparationHz, so a row admitting ±MinSeparationHz/2 of its centre is
// exactly wide enough that it can never legitimately hold two partials of the
// same take. Any wider and the rule and the detector would be contradicting each
// other about what counts as one mode.
//
// Rejected: a relative tolerance in cents, which is the right unit for a tuning
// spread and the wrong one here. The detector's separation guard is absolute, so
// a cents window is simultaneously too tight to hold the 1141–1150 Hz row on
// this reference and wide enough at 1.2 kHz to chain that row into the 1172 Hz
// one above it.
func correspondenceTolerance(minSeparationHz float64) float64 { return minSeparationHz / 2 }

// measureSeries correlates each measured quantity against the take index.
func measureSeries(takes []Take) *Series {
	if len(takes) < 2 {
		return nil
	}

	out := &Series{Takes: len(takes)}

	for _, take := range takes {
		out.Order = append(out.Order, takeLabel(take.Path))
	}

	// Peak amplitude is in the list precisely because it is the one that does
	// *not* work, and a reader who does not see it will reach for it. Every file
	// in reference/tt08x08/lp/hd is peak-normalised to 0.88–1.00, so it carries
	// no strike information: rho = +0.16 against the take index, which is noise.
	// Crest factor over the same takes is +0.91 and attack balance +0.85.
	out.Trends = []Trend{
		trend("base frequency (Hz)", takes, func(take Take) (float64, bool) {
			return take.BaseHz, take.BaseHz > 0
		}),
		trend("base T60 (s)", takes, func(take Take) (float64, bool) {
			t60 := baseT60(take)

			return t60, t60 > 0
		}),
		trend("attack balance (dB)", takes, func(take Take) (float64, bool) {
			return take.AttackBalanceDB, true
		}),
		trend("peak amplitude", takes, func(take Take) (float64, bool) {
			return take.Health.PeakAmplitude, take.Health.PeakAmplitude > 0
		}),
		trend("crest factor", takes, func(take Take) (float64, bool) {
			return take.Health.CrestFactor, take.Health.CrestFactor > 0
		}),
	}

	return out
}

// trend correlates one quantity against the index of the take it was read from.
//
// The index is the take's position in the full list, not its position among the
// takes that had a reading: the claim under test is about the order of the files
// as given, and renumbering the survivors would test a different one. Rank
// correlation is unharmed by the resulting gaps.
func trend(quantity string, takes []Take, read func(Take) (float64, bool)) Trend {
	var indices, values []float64

	for index, take := range takes {
		value, ok := read(take)
		if !ok {
			continue
		}

		indices = append(indices, float64(index))
		values = append(values, value)
	}

	result := Trend{Quantity: quantity, Pairs: len(values)}

	coefficient, err := series.Spearman(indices, values)
	if err != nil {
		result.Note = err.Error()

		return result
	}

	result.Spearman = coefficient
	result.Measured = true

	return result
}

// alignedPartial is one detected partial carrying the take it came from, so the
// clustering below can work on one frequency-sorted list rather than on sixteen.
type alignedPartial struct {
	take    int
	hz      float64
	levelDB float64
}

// measureCorrespondence aligns every take's partials into common rows.
//
// The sweep is over one list sorted by frequency, so the result does not depend
// on the order the files were listed in — which matters more here than anywhere
// else in this tool, because a table that rearranged itself when the shell
// expanded the glob differently would be evidence of nothing. Ties in frequency
// are broken by level and then by take index; two takes agreeing to the last bit
// of a float is not something the peak picker produces, and the tie-break exists
// so that the sort is total rather than because it is ever exercised.
//
// A partial joins the current row when it is within toleranceHz of that row's
// running mean, rather than of its nearest member: single-link chaining lets a
// row walk arbitrarily far from where it started, and on this reference a
// nearest-member rule joins the 1141 Hz row to the 1172 Hz one through the
// single 1157.7 Hz partial in v15.
//
// When a row already holds a partial from the incoming partial's take, the
// incoming one opens a new row and the incumbent keeps the row it seeded. The
// sweep is ascending, so the incumbent is always the lower of the two, and
// "lower keeps the row" makes membership a function of the sorted list alone.
// Rejected: keeping whichever of the two is nearer the row mean, which is more
// even-handed and needs an eviction that can cascade into rows already closed.
func measureCorrespondence(takes []Take, toleranceHz float64) *Correspondence {
	if len(takes) < 2 {
		return nil
	}

	out := &Correspondence{ToleranceHz: toleranceHz}

	var flat []alignedPartial

	for index, take := range takes {
		out.Takes = append(out.Takes, takeLabel(take.Path))

		base := baseLevelDB(take)

		for _, row := range take.Partials {
			flat = append(flat, alignedPartial{
				take:    index,
				hz:      row.FrequencyHz,
				levelDB: row.LevelDB - base,
			})
		}
	}

	slices.SortFunc(flat, func(left, right alignedPartial) int {
		if order := cmpFloat(left.hz, right.hz); order != 0 {
			return order
		}

		if order := cmpFloat(left.levelDB, right.levelDB); order != 0 {
			return order
		}

		return left.take - right.take
	})

	var (
		rows    [][]alignedPartial
		current []alignedPartial
		sum     float64
	)

	flush := func() {
		if len(current) > 0 {
			rows = append(rows, current)
		}

		current, sum = nil, 0
	}

	for _, partial := range flat {
		mean := sum / float64(max(len(current), 1))

		if len(current) > 0 && (math.Abs(partial.hz-mean) > toleranceHz || holds(current, partial.take)) {
			flush()
		}

		current = append(current, partial)
		sum += partial.hz
	}

	flush()

	for _, row := range rows {
		out.Modes = append(out.Modes, modeRow(row, len(takes)))
	}

	return out
}

func holds(row []alignedPartial, take int) bool {
	for _, partial := range row {
		if partial.take == take {
			return true
		}
	}

	return false
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func modeRow(row []alignedPartial, takes int) ModeRow {
	frequencies := make([]float64, 0, len(row))
	levels := make([]float64, 0, len(row))
	cells := make([]*float64, takes)

	for _, partial := range row {
		frequencies = append(frequencies, partial.hz)
		levels = append(levels, partial.levelDB)

		level := partial.levelDB
		cells[partial.take] = &level
	}

	return ModeRow{
		MeanHz:        mean(frequencies),
		SpreadHz:      spread(frequencies),
		Present:       len(row),
		MeanLevelDB:   mean(levels),
		LevelSDDB:     standardDeviation(levels),
		LevelSpreadDB: spread(levels),
		LevelDB:       cells,
	}
}

// baseLevelDB is the level the take's own fundamental was detected at, which
// every cell of that take's column is taken against. Zero when the base was
// never identified, which leaves the column as the raw balance against the
// strongest partial — wrong to mix with the others, and visible as such, because
// the take's base frequency is already reported as absent.
func baseLevelDB(take Take) float64 {
	for _, row := range take.Partials {
		if row.FrequencyHz == take.BaseHz {
			return row.LevelDB
		}
	}

	return 0
}

// takeLabel is the file's stem, which is what a column heading has room for and
// what the takes are referred to by everywhere else — v08, not the whole path.
func takeLabel(path string) string {
	base := filepath.Base(path)

	return strings.TrimSuffix(base, filepath.Ext(base))
}

func printSeries(writer io.Writer, out Series) {
	_, _ = fmt.Fprintf(writer, "== series trend across %d takes, in the order given\n", out.Takes)
	_, _ = fmt.Fprintf(writer, "   order: %s\n", strings.Join(out.Order, " "))
	_, _ = fmt.Fprintln(writer,
		"   Spearman rho of each quantity against the take index. This is the reduction for a")
	_, _ = fmt.Fprintln(writer,
		"   deliberate ramp; the repeatability block above is the one for repeats at a fixed")
	_, _ = fmt.Fprintln(writer,
		"   dynamic, and on a ramp its spread is the trend and not jitter.")
	_, _ = fmt.Fprintln(writer,
		"   The take order is a claim about how the files were played, not a measurement:")
	_, _ = fmt.Fprintln(writer,
		"   read rho as evidence about that claim, never as licence to assume it.")

	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)

	_, _ = fmt.Fprintln(table, "   quantity\ttakes\trho")

	for _, item := range out.Trends {
		value := "-"
		if item.Measured {
			value = fmt.Sprintf("%+.2f", item.Spearman)
		}

		_, _ = fmt.Fprintf(table, "   %s\t%d\t%s\n", item.Quantity, item.Pairs, value)
	}

	_ = table.Flush()

	for _, item := range out.Trends {
		if item.Note != "" {
			_, _ = fmt.Fprintf(writer, "   ! %s: %s\n", item.Quantity, item.Note)
		}
	}
}

// printCorrespondence prints the aligned table. minPresent hides rows seen in
// fewer takes than that: with sixteen takes of sixteen partials the full table
// is mostly modes detected once, and those say more about the detection floor
// than about the drum. The JSON carries every row regardless, so the filter is a
// property of the printout and never of the record.
func printCorrespondence(writer io.Writer, out Correspondence, minPresent int) {
	_, _ = fmt.Fprintf(writer,
		"== partial correspondence across %d takes (rows aligned within %.1f Hz of the row mean)\n",
		len(out.Takes), out.ToleranceHz)
	_, _ = fmt.Fprintln(writer,
		"   cells are dB relative to that take's own fundamental; blank = the mode was not detected.")
	_, _ = fmt.Fprintf(writer,
		"   rows present in fewer than %d takes are omitted here and kept in the JSON.\n", minPresent)

	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)

	header := "   f (Hz)\tn\tmean\tsd\tspread"
	for _, label := range out.Takes {
		header += "\t" + label
	}

	_, _ = fmt.Fprintln(table, header)

	for _, mode := range out.Modes {
		if mode.Present < minPresent {
			continue
		}

		_, _ = fmt.Fprintf(table, "   %.1f\t%d\t%.1f\t%.1f\t%.1f",
			mode.MeanHz, mode.Present, mode.MeanLevelDB, mode.LevelSDDB, mode.LevelSpreadDB)

		for _, cell := range mode.LevelDB {
			if cell == nil {
				_, _ = fmt.Fprint(table, "\t")

				continue
			}

			_, _ = fmt.Fprintf(table, "\t%.1f", *cell)
		}

		_, _ = fmt.Fprintln(table)
	}

	_ = table.Flush()
}
