//go:build purego

package main

import (
	"math"

	"github.com/cwbudde/matplotlib-go/core"
	"github.com/cwbudde/matplotlib-go/geom"
	"github.com/cwbudde/matplotlib-go/optional"
	"github.com/cwbudde/matplotlib-go/ticker"
)

// Figures are sized to the paper's text width at 200 dpi.
const (
	figureWidth = 1320
	// Marker radii are given in pixels; ScatterAreaFromRadius converts via 72/dpi,
	// so passing 72 keeps the requested radius in image pixels.
	markerDPI = 72

	titleSize  = 11.0
	labelSize  = 11.0
	legendSize = 10.0
)

// The frequency decades every figure shares, so the four can be read against
// each other without re-reading the axis.
var (
	frequencyTicks = []float64{100, 200, 300, 500, 700, 1000, 1500}
	frequencyMin   = 80.0
	frequencyMax   = 1600.0
)

func newFigure(height int) (*core.Figure, *core.Axes) {
	fig := core.NewFigure(figureWidth, height)
	axes := fig.AddAxes(geom.Rect{
		Min: geom.Pt{X: 0.075, Y: 0.155},
		Max: geom.Pt{X: 0.985, Y: 0.90},
	})
	axes.AddXGrid()
	axes.AddYGrid()

	return fig, axes
}

func logFrequencyAxis(axes *core.Axes) error {
	if err := axes.SetXScale("log"); err != nil {
		return err
	}

	axes.SetXLim(frequencyMin, frequencyMax)
	axes.XAxis.Locator = ticker.FixedLocator{TicksList: frequencyTicks}
	axes.XAxis.Formatter = ticker.ScalarFormatter{Prec: 0}
	axes.SetXLabel("frequency (Hz)")

	return nil
}

// drawPartials is the picture the two coverage terms score: which reference
// partials the model accounted for, which it missed, and which it invented.
func drawPartials(rep *report, out string) error {
	ref, can := rep.Target.Partials, rep.Best.Features.Partials
	pairs := matchPartials(ref, can)

	fig, axes := newFigure(560)

	const floor = -46.0

	low, high := referenceSpan(ref)
	axes.AxVSpan(low, high, core.VSpanOptions{
		Color: optional.Of(fade(accent, 0.05)),
	})

	// The links come first so the stems and markers sit on top of them.
	for _, link := range pairs.links {
		_, err := axes.Plot(
			[]float64{ref[link[0]].FrequencyHz, can[link[1]].FrequencyHz},
			[]float64{ref[link[0]].LevelDB, can[link[1]].LevelDB},
			core.PlotOptions{
				Color:     optional.Of(rule),
				LineWidth: optional.Of(1.4),
			},
		)
		if err != nil {
			return err
		}
	}

	// MarkerCross is stroked rather than filled, so a white edge would erase it;
	// the filled markers keep the white edge that separates overlapping stems.
	draw := func(partials []partial, indices []int, colour, edge renderColor, marker core.MarkerType, label string) {
		if len(indices) == 0 {
			return
		}

		freq := make([]float64, 0, len(indices))
		level := make([]float64, 0, len(indices))
		base := make([]float64, 0, len(indices))

		for _, index := range indices {
			freq = append(freq, partials[index].FrequencyHz)
			level = append(level, partials[index].LevelDB)
			base = append(base, floor)
		}

		axes.VLines(freq, base, level, core.LineCollectionOptions{
			Color:     optional.Of(fade(colour, 0.5)),
			LineWidth: optional.Of(1.6),
		})
		_, _ = axes.Scatter(freq, level, core.ScatterOptions{
			Color:     optional.Of(colour),
			Marker:    optional.Of(marker),
			Size:      optional.Of(core.ScatterAreaFromRadius(7.0, markerDPI)),
			EdgeColor: optional.Of(edge),
			EdgeWidth: optional.Of(1.4),
			Label:     label,
		})
	}

	matchedRef, missingRef := pairs.split(len(ref), pairs.matchedRef)
	matchedCan, inventedCan := pairs.split(len(can), pairs.matchedCan)

	draw(ref, matchedRef, accent, white, core.MarkerCircle, "reference, matched")
	draw(ref, missingRef, accent, accent, core.MarkerCross, "reference, unmatched")
	draw(can, matchedCan, warm, white, core.MarkerSquare, "model, matched")
	draw(can, inventedCan, warm, white, core.MarkerTriangle, "model, no counterpart")

	if err := logFrequencyAxis(axes); err != nil {
		return err
	}

	axes.SetYLim(floor, 12)
	axes.SetYLabel("level (dB re. strongest)")

	legend := axes.AddLegend()
	legend.Location = core.LegendUpperRight
	legend.NumColumns = 2
	legend.FrameOn = false

	return fig.Save(out)
}

// drawTerms shows every term as a multiple of its own threshold, which is the
// only view in which the nine are comparable — and the view the adoption gate
// is actually read in.
func drawTerms(rep *report, out string) error {
	type entry struct {
		label     string
		threshold float64
		gated     bool
		baseline  float64
		fitted    float64
	}

	base, best := rep.Baseline.Terms, rep.Best.Terms
	entries := []entry{
		{"partial freq.", 25, true, base.PartialFrequency, best.PartialFrequency},
		{"partial decay", 0.35, true, base.PartialDecay, best.PartialDecay},
		{"spectral env.", 4, true, base.SpectralEnvelope, best.SpectralEnvelope},
		{"partial level", 3, false, base.PartialLevel, best.PartialLevel},
		{"amplitude env.", 3, false, base.Envelope, best.Envelope},
		{"glide", 40, false, base.Glide, best.Glide},
		{"attack balance", 6, false, base.AttackBalance, best.AttackBalance},
		{"unmatched", 0.5, false, base.Unmatched, best.Unmatched},
		{"spurious", 0.5, false, base.Spurious, best.Spurious},
	}

	fig, axes := newFigure(520)

	positions := make([]float64, len(entries))
	labels := make([]string, len(entries))
	baselineBars := make([]float64, len(entries))
	fittedBars := make([]float64, len(entries))
	fittedColours := make([]renderColor, len(entries))
	peak := 0.0

	for index, e := range entries {
		positions[index] = float64(index)
		labels[index] = e.label
		baselineBars[index] = e.baseline / e.threshold
		fittedBars[index] = e.fitted / e.threshold
		fittedColours[index] = muted

		if e.gated {
			fittedColours[index] = accent
		}

		peak = math.Max(peak, math.Max(baselineBars[index], fittedBars[index]))
	}

	left := make([]float64, len(entries))
	right := make([]float64, len(entries))

	for index := range entries {
		left[index] = positions[index] - 0.19
		right[index] = positions[index] + 0.19
	}

	if _, err := axes.Bar(left, baselineBars, core.BarOptions{
		Color: optional.Of(fade(muted, 0.32)),
		Width: optional.Of(0.36),
		Label: "shipped default",
	}); err != nil {
		return err
	}

	if _, err := axes.Bar(right, fittedBars, core.BarOptions{
		Colors: fittedColours,
		Width:  optional.Of(0.36),
		Label:  "fitted",
	}); err != nil {
		return err
	}

	axes.AxHLine(1, core.HLineOptions{
		Color:     optional.Of(warm),
		LineWidth: optional.Of(1.6),
		Dashes:    []float64{7, 4},
	})
	axes.Text(float64(len(entries))-0.6, 1.12, "threshold", core.TextOptions{
		FontSize: legendSize,
		Color:    warm,
		HAlign:   core.TextAlignRight,
	})

	if err := axes.SetYScale("log"); err != nil {
		return err
	}

	axes.SetXLim(-0.6, float64(len(entries))-0.4)
	axes.SetYLim(0.02, peak*2)
	axes.SetYLabel("multiples of threshold")
	axes.XAxis.Locator = ticker.FixedLocator{TicksList: positions}
	axes.XAxis.Formatter = ticker.FixedFormatter{Labels: labels}

	legend := axes.AddLegend()
	legend.Location = core.LegendUpperRight
	legend.FrameOn = false

	return fig.Save(out)
}

// drawDecay puts the ring times against the constant-Q law a membrane obeys,
// which is what makes "the model rings too long" a reading rather than an
// impression.
func drawDecay(rep *report, out string) error {
	ref, can := rep.Target.Partials, rep.Best.Features.Partials

	fig, axes := newFigure(500)

	// Anchored on the reference fundamental, so the line is a statement about
	// the reference's own consistency and not a fitted trend.
	anchorF, anchorT := ref[0].FrequencyHz, ref[0].T60Seconds

	const steps = 96

	grid := make([]float64, steps)
	law := make([]float64, steps)

	for i := range steps {
		ratio := float64(i) / float64(steps-1)
		grid[i] = frequencyMin * math.Pow(frequencyMax/frequencyMin, ratio)
		law[i] = anchorT * anchorF / grid[i]
	}

	if _, err := axes.Plot(grid, law, core.PlotOptions{
		Color:     optional.Of(muted),
		LineWidth: optional.Of(1.6),
		Dashes:    []float64{8, 5},
		Label:     "constant Q:  T60 ∝ 1/f",
	}); err != nil {
		return err
	}

	scatter := func(partials []partial, colour renderColor, marker core.MarkerType, label string) {
		freq := make([]float64, len(partials))
		t60 := make([]float64, len(partials))
		sizes := make([]float64, len(partials))

		for index, p := range partials {
			freq[index] = p.FrequencyHz
			t60[index] = p.T60Seconds
			// Marker area follows the decay fit's R², so a partial whose
			// envelope is not an exponential does not read as a firm datum.
			sizes[index] = core.ScatterAreaFromRadius(4.5+4.0*p.FitQuality, markerDPI)
		}

		_, _ = axes.Scatter(freq, t60, core.ScatterOptions{
			Color:     optional.Of(colour),
			Marker:    optional.Of(marker),
			Sizes:     sizes,
			EdgeColor: optional.Of(white),
			EdgeWidth: optional.Of(1.0),
			Label:     label,
		})
	}

	scatter(ref, accent, core.MarkerCircle, "reference")
	scatter(can, warm, core.MarkerSquare, "model")

	if err := logFrequencyAxis(axes); err != nil {
		return err
	}

	if err := axes.SetYScale("log"); err != nil {
		return err
	}

	axes.SetYLim(0.08, 3)
	axes.SetYLabel("T60 (s)")

	legend := axes.AddLegend()
	legend.Location = core.LegendUpperRight
	legend.FrameOn = false

	return fig.Save(out)
}

// drawBands is the spectral envelope window by window — the term the adoption
// gate is furthest from, shown in the form that says where it is furthest.
func drawBands(rep *report, out string) error {
	centres := rep.Target.BandCentresHz
	if len(centres) == 0 {
		return errNoBands
	}

	candidate := map[string]window{}
	for _, candWindow := range rep.Best.Features.Windows {
		candidate[candWindow.Name] = candWindow
	}

	fig := core.NewFigure(figureWidth, 420)
	panels := len(rep.Target.Windows)

	for index, refWindow := range rep.Target.Windows {
		panel := fig.AddSubplot(1, panels, index+1)
		panel.AddXGrid()
		panel.AddYGrid()

		if _, err := panel.Plot(centres, refWindow.BandDB, core.PlotOptions{
			Color:     optional.Of(accent),
			LineWidth: optional.Of(1.8),
			Label:     "reference",
		}); err != nil {
			return err
		}

		if other, ok := candidate[refWindow.Name]; ok && len(other.BandDB) == len(centres) {
			if _, err := panel.Plot(centres, other.BandDB, core.PlotOptions{
				Color:     optional.Of(warm),
				LineWidth: optional.Of(1.8),
				Label:     "model",
			}); err != nil {
				return err
			}
		}

		if err := panel.SetXScale("log"); err != nil {
			return err
		}

		panel.SetXLim(60, 12500)
		panel.SetYLim(-45, 55)
		panel.SetTitle(refWindow.Name)
		panel.SetXLabel("Hz")

		if index == 0 {
			panel.SetYLabel("band level (dB)")

			legend := panel.AddLegend()
			legend.Location = core.LegendLowerLeft
			legend.FrameOn = false
		}
	}

	return fig.Save(out)
}
