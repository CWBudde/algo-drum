//go:build purego

package main

import (
	"fmt"
	"math"

	"github.com/cwbudde/matplotlib-go/core"
	"github.com/cwbudde/matplotlib-go/optional"
	"github.com/cwbudde/matplotlib-go/ticker"
)

// The model figures reach further up than the fit figures do: the attack layer
// stands in for modes an octave and a half above the highest resolved one, and a
// figure of the hybrid split that stopped at the modal ceiling would not show
// the thing it is about.
var (
	wideTicks = []float64{100, 200, 500, 1000, 2000, 5000, 10000}
	wideMin   = 100.0
	wideMax   = 12000.0
)

// radiationFloorDB bounds the microphone-weight figure from below.
//
// The weight of a mode of azimuthal order m carries a multipole cancellation of
// 1/(2^m m!), which at the highest retained order is 17 orders of magnitude.
// Those weights reach the double-precision noise around 1e-19 and change sign
// there, so plotting them unbounded would draw arithmetic rather than physics.
const radiationFloorDB = -120.0

func wideFrequencyAxis(axes *core.Axes) error {
	if err := axes.SetXScale("log"); err != nil {
		return err
	}

	axes.SetXLim(wideMin, wideMax)
	axes.XAxis.Locator = ticker.FixedLocator{TicksList: wideTicks}
	axes.XAxis.Formatter = ticker.ScalarFormatter{Prec: 0}
	axes.SetXLabel("frequency (Hz)")

	return nil
}

// drawModes is the instrument's own mode map: every retained oscillator, where
// it sits and how long it rings, against the constant-Q law the loss
// coefficients are calibrated to hold.
//
// It is the model-side counterpart of decay.png, drawn on the same axes, so the
// tilt that figure measures against the recording can be read against what the
// bank does on its own.
func drawModes(data *modelData, out string) error {
	fig, axes := newFigure(560)

	batter, ok := data.head("batter")
	if !ok {
		return errNoBatterHead
	}

	// The law, not a fit: zeta = d1/c follows from the k¹ loss coefficient and
	// the wave speed, with no reference to the modes plotted against it.
	if zeta := batter.dampingRatio(); zeta > 0 {
		const steps = 96

		grid := make([]float64, steps)
		law := make([]float64, steps)

		for index := range steps {
			ratio := float64(index) / float64(steps-1)
			grid[index] = frequencyMin * math.Pow(frequencyMax/frequencyMin, ratio)
			law[index] = math.Log(1000) / (2 * math.Pi * zeta * grid[index])
		}

		if _, err := axes.Plot(grid, law, core.PlotOptions{
			Color:     optional.Of(muted),
			LineWidth: optional.Of(1.6),
			Dashes:    []float64{8, 5},
			Label:     fmt.Sprintf("constant Q:  ζ = d₁/c = %.2f %%", 100*zeta),
		}); err != nil {
			return err
		}
	}

	scatter := func(head string, colour renderColor, marker core.MarkerType, label string) error {
		modes := data.byFrequency(head)
		if len(modes) == 0 {
			return nil
		}

		freq := make([]float64, 0, len(modes))
		t60 := make([]float64, 0, len(modes))

		for _, mode := range modes {
			if !finitePositive(mode.FrequencyHz) || !finitePositive(mode.T60Seconds) {
				continue
			}

			freq = append(freq, mode.FrequencyHz)
			t60 = append(t60, mode.T60Seconds)
		}

		_, err := axes.Scatter(freq, t60, core.ScatterOptions{
			Color:     optional.Of(colour),
			Marker:    optional.Of(marker),
			Size:      optional.Of(core.ScatterAreaFromRadius(4.5, markerDPI)),
			EdgeColor: optional.Of(white),
			EdgeWidth: optional.Of(0.8),
			Label:     label,
		})

		return err
	}

	if err := scatter("batter", accent, core.MarkerCircle, "batter head"); err != nil {
		return err
	}

	if err := scatter("resonant", warm, core.MarkerSquare, "resonant head"); err != nil {
		return err
	}

	// The corrected fundamentals are the whole point of the figure, so they are
	// named rather than left for the reader to find among a hundred markers.
	corrected := make([]float64, 0, 2)
	correctedT60 := make([]float64, 0, 2)

	for _, mode := range data.Modes {
		if mode.DecayCorrectionPerSecond != 0 && finitePositive(mode.T60Seconds) {
			corrected = append(corrected, mode.FrequencyHz)
			correctedT60 = append(correctedT60, mode.T60Seconds)
		}
	}

	if len(corrected) > 0 {
		if _, err := axes.Scatter(corrected, correctedT60, core.ScatterOptions{
			Color:     optional.Of(fade(warm, 0)),
			Marker:    optional.Of(core.MarkerCircle),
			Size:      optional.Of(core.ScatterAreaFromRadius(9.0, markerDPI)),
			EdgeColor: optional.Of(warm),
			EdgeWidth: optional.Of(1.8),
			Label:     "(0,1), with its measured decay correction",
		}); err != nil {
			return err
		}
	}

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

// drawLoss separates the loss law into the channels it is made of, which is what
// makes "the residual is the shape of the law" a checkable claim rather than a
// characterisation.
func drawLoss(data *modelData, out string) error {
	modes := data.byFrequency("batter")
	if len(modes) == 0 {
		return errNoBatterHead
	}

	fig := core.NewFigure(figureWidth, 470)

	rates := fig.AddSubplot(1, 2, 1)
	rates.AddXGrid()
	rates.AddYGrid()

	freq := make([]float64, 0, len(modes))
	structural := make([]float64, 0, len(modes))
	radiation := make([]float64, 0, len(modes))
	total := make([]float64, 0, len(modes))
	ratio := make([]float64, 0, len(modes))

	for _, mode := range modes {
		if !finitePositive(mode.FrequencyHz) {
			continue
		}

		freq = append(freq, mode.FrequencyHz)
		structural = append(structural, mode.StructuralDecayPerSecond)
		radiation = append(radiation, math.Max(mode.RadiationDecayPerSecond, 1e-6))
		total = append(total, mode.DecayRatePerSecond)
		ratio = append(ratio, 100*mode.DampingRatio)
	}

	lines := []struct {
		values []float64
		colour renderColor
		dashes []float64
		label  string
	}{
		{total, accent, nil, "total γ"},
		{structural, muted, []float64{6, 3}, "structural  d₀ + d₁k + d₂k²"},
	}

	for _, line := range lines {
		if _, err := rates.Plot(freq, line.values, core.PlotOptions{
			Color:     optional.Of(line.colour),
			LineWidth: optional.Of(1.8),
			Dashes:    line.dashes,
			Label:     line.label,
		}); err != nil {
			return err
		}
	}

	// Radiation loss is scattered rather than joined. It depends on the mode's
	// azimuthal order as well as its frequency — the efficiency is raised to
	// m+1 — so neighbouring modes of different order sit at genuinely different
	// rates, and a line through them in frequency order would draw that spread
	// as if it were noise on one curve.
	if _, err := rates.Scatter(freq, radiation, core.ScatterOptions{
		Color:  optional.Of(fade(warm, 0.75)),
		Marker: optional.Of(core.MarkerCircle),
		Size:   optional.Of(core.ScatterAreaFromRadius(2.6, markerDPI)),
		Label:  "radiation  d_rad·α², per mode",
	}); err != nil {
		return err
	}

	// The structural law continued past the highest resolved mode, with the
	// attack layer's three bands on it. This is the claim that the layer is an
	// extrapolation of the mode series rather than an effect bolted beside it:
	// its releases are this curve read at those three frequencies.
	if batter, ok := data.head("batter"); ok && batter.WaveSpeedMPerS > 0 {
		const steps = 120

		top := freq[len(freq)-1]
		grid := make([]float64, steps)
		law := make([]float64, steps)

		for index := range steps {
			fraction := float64(index) / float64(steps-1)
			grid[index] = top * math.Pow(wideMax/top, fraction)
			law[index] = batter.structuralDecayAt(grid[index])
		}

		if _, err := rates.Plot(grid, law, core.PlotOptions{
			Color:     optional.Of(fade(muted, 0.55)),
			LineWidth: optional.Of(1.4),
			Dashes:    []float64{2, 3},
			Label:     "the same law, past the modal ceiling",
		}); err != nil {
			return err
		}

		bandFreq := make([]float64, 0, len(data.AttackBands))
		bandRate := make([]float64, 0, len(data.AttackBands))

		for _, band := range data.AttackBands {
			if !finitePositive(band.T60Seconds) {
				continue
			}

			bandFreq = append(bandFreq, band.CentreHz)
			bandRate = append(bandRate, math.Log(1000)/band.T60Seconds)
		}

		if _, err := rates.Scatter(bandFreq, bandRate, core.ScatterOptions{
			Color:     optional.Of(warm),
			Marker:    optional.Of(core.MarkerDiamond),
			Size:      optional.Of(core.ScatterAreaFromRadius(7.0, markerDPI)),
			EdgeColor: optional.Of(white),
			EdgeWidth: optional.Of(1.2),
			Label:     "attack bands, released at that law",
		}); err != nil {
			return err
		}
	}

	if err := panelFrequencyAxis(rates); err != nil {
		return err
	}

	if err := rates.SetYScale("log"); err != nil {
		return err
	}

	rates.SetYLim(3e-2, 3e3)
	rates.SetYLabel("decay rate (1/s)")
	rates.SetTitle("loss channels")

	rateLegend := rates.AddLegend()
	rateLegend.Location = core.LegendUpperLeft
	rateLegend.FrameOn = false

	shape := fig.AddSubplot(1, 2, 2)
	shape.AddXGrid()
	shape.AddYGrid()

	if _, err := shape.Plot(freq, ratio, core.PlotOptions{
		Color:     optional.Of(accent),
		LineWidth: optional.Of(1.8),
		Label:     "ζ = γ/ω, per retained mode",
	}); err != nil {
		return err
	}

	// The attack layer on the same axis. Its bands are not modes and have no
	// gamma/omega of their own to measure — these are the derived releases read
	// back as a damping ratio, which is the check that the layer continues the
	// series rather than sitting at a rate of its own.
	bandFreq := make([]float64, 0, len(data.AttackBands))
	bandRatio := make([]float64, 0, len(data.AttackBands))

	for _, band := range data.AttackBands {
		if !finitePositive(band.T60Seconds) || !finitePositive(band.CentreHz) {
			continue
		}

		rate := math.Log(1000) / band.T60Seconds
		bandFreq = append(bandFreq, band.CentreHz)
		bandRatio = append(bandRatio, 100*rate/(2*math.Pi*band.CentreHz))
	}

	if _, err := shape.Scatter(bandFreq, bandRatio, core.ScatterOptions{
		Color:     optional.Of(warm),
		Marker:    optional.Of(core.MarkerDiamond),
		Size:      optional.Of(core.ScatterAreaFromRadius(7.0, markerDPI)),
		EdgeColor: optional.Of(white),
		EdgeWidth: optional.Of(1.2),
		Label:     "attack bands, read back the same way",
	}); err != nil {
		return err
	}

	if batter, ok := data.head("batter"); ok && batter.dampingRatio() > 0 {
		shape.AxHLine(100*batter.dampingRatio(), core.HLineOptions{
			Color:     optional.Of(rule),
			LineWidth: optional.Of(1.6),
			Dashes:    []float64{7, 4},
		})
		shape.Text(
			wideMin*1.1,
			100*batter.dampingRatio()+0.14,
			"d₁/c",
			core.TextOptions{FontSize: legendSize, Color: muted, HAlign: core.TextAlignLeft},
		)
	}

	if err := panelFrequencyAxis(shape); err != nil {
		return err
	}

	shape.SetYLim(0, 4)
	shape.SetYLabel("damping ratio ζ (%)")
	shape.SetTitle("the shape the law holds")

	shapeLegend := shape.AddLegend()
	shapeLegend.Location = core.LegendUpperRight
	shapeLegend.FrameOn = false

	return fig.Save(out)
}

// drawRadiation is the partial balance the microphone model produces, by
// azimuthal order, with and without the evanescent near-field term.
//
// The two panels are the reason that term exists. Far-field pressure from a
// compact source is proportional to volume acceleration, and volume
// displacement is exactly zero for every m > 0 mode, so a distant microphone
// hears the axisymmetric series and a long way down everything else — which is
// correct, and is not what a close-miked tom sounds like. The near field decays
// as exp(-z·d/R) rather than propagating, so at three centimetres it restores
// the m > 0 modes and at three metres it is gone.
func drawRadiation(data *modelData, out string) error {
	panels := []struct {
		title string
		pick  func(modelMode) float64
	}{
		{
			"far field only  (s_nf = 0)",
			func(mode modelMode) float64 { return mode.FarFieldRadiationWeight },
		},
		{
			"shipped close microphone  (30 mm, s_nf = 1)",
			func(mode modelMode) float64 { return mode.RadiationWeight },
		},
	}

	fig := core.NewFigure(figureWidth, 470)

	for index, spec := range panels {
		reference := data.fundamentalWeight(spec.pick)
		if reference <= 0 {
			return errNoFundamental
		}

		axes := fig.AddSubplot(1, len(panels), index+1)
		axes.AddXGrid()
		axes.AddYGrid()

		if err := drawRadiationPanel(axes, data, spec.pick, reference, index == 0); err != nil {
			return err
		}

		axes.SetTitle(spec.title)

		if index == 0 {
			axes.SetYLabel("microphone weight (dB re. the (0,1))")
		}
	}

	return fig.Save(out)
}

// radiationOrders is how many azimuthal orders are named individually before the
// rest are grouped. Grouping the tail is not a simplification: past this order
// every mode is below the plot's floor, and six more legend entries would say
// only that.
const radiationOrders = 4

func drawRadiationPanel(
	axes *core.Axes,
	data *modelData,
	pick func(modelMode) float64,
	reference float64,
	withLegend bool,
) error {
	type series struct {
		freq   []float64
		weight []float64
	}

	grouped := make(map[int]*series)

	for _, mode := range data.Modes {
		order := min(mode.AzimuthalOrder, radiationOrders+1)

		bucket, ok := grouped[order]
		if !ok {
			bucket = &series{}
			grouped[order] = bucket
		}

		level := radiationFloorDB
		if magnitude := math.Abs(pick(mode)); magnitude > 0 {
			level = math.Max(20*math.Log10(magnitude/reference), radiationFloorDB)
		}

		bucket.freq = append(bucket.freq, mode.FrequencyHz)
		bucket.weight = append(bucket.weight, level)
	}

	// MarkerCross and MarkerPlus are stroked rather than filled, so a white edge
	// would erase them; they take their own colour as the edge instead.
	markers := []core.MarkerType{
		core.MarkerCircle,
		core.MarkerSquare,
		core.MarkerTriangle,
		core.MarkerDiamond,
		core.MarkerCross,
		core.MarkerPlus,
	}

	for order := 0; order <= radiationOrders+1; order++ {
		bucket, ok := grouped[order]
		if !ok {
			continue
		}

		label := fmt.Sprintf("m = %d", order)
		if order == radiationOrders+1 {
			label = fmt.Sprintf("m > %d", radiationOrders)
		}

		marker := markers[min(order, len(markers)-1)]
		colour := blend(accent, warm, float64(order)/float64(radiationOrders+1))
		edge := white

		if marker == core.MarkerCross || marker == core.MarkerPlus {
			edge = colour
		}

		if _, err := axes.Scatter(bucket.freq, bucket.weight, core.ScatterOptions{
			Color:     optional.Of(colour),
			Marker:    optional.Of(marker),
			Size:      optional.Of(core.ScatterAreaFromRadius(4.5, markerDPI)),
			EdgeColor: optional.Of(edge),
			EdgeWidth: optional.Of(0.9),
			Label:     label,
		}); err != nil {
			return err
		}
	}

	if err := logFrequencyAxis(axes); err != nil {
		return err
	}

	axes.SetYLim(radiationFloorDB, 8)

	if withLegend {
		legend := axes.AddLegend()
		legend.Location = core.LegendLowerLeft
		legend.NumColumns = 3
		legend.FrameOn = false
	}

	return nil
}

// drawCavity is the air spring's whole audible effect: what the enclosed air
// does to the axisymmetric fundamental, at the uncoupled limit, at the shipped
// fitted stiffness, and at the rigid-enclosure ceiling the formula predicts.
func drawCavity(data *modelData, out string) error {
	grid := data.CavityResponse.FrequencyHz
	if len(grid) == 0 {
		return errNoCavityResponse
	}

	fig, axes := newFigure(500)

	colours := []renderColor{muted, accent, warm}
	dashes := [][]float64{{3, 3}, nil, {8, 4}}

	for index, curve := range data.CavityResponse.Curves {
		if len(curve.MagnitudeDB) != len(grid) {
			return fmt.Errorf(
				"cavity curve at scale %v has %d points against %d frequencies",
				curve.StiffnessScale,
				len(curve.MagnitudeDB),
				len(grid),
			)
		}

		label := fmt.Sprintf("s = %g", curve.StiffnessScale)

		switch curve.StiffnessScale {
		case 0:
			label += "  (air spring off)"
		case 1:
			label += "  (rigid enclosure)"
		default:
			label += "  (shipped)"
		}

		if curve.LowerBranchHz > 0 && curve.UpperBranchHz > 0 {
			label += fmt.Sprintf(
				":  %.0f / %.0f Hz,  ×%.2f",
				curve.LowerBranchHz,
				curve.UpperBranchHz,
				curve.UpperBranchHz/curve.LowerBranchHz,
			)
		}

		if _, err := axes.Plot(grid, curve.MagnitudeDB, core.PlotOptions{
			Color:     optional.Of(colours[min(index, len(colours)-1)]),
			LineWidth: optional.Of(1.8),
			Dashes:    dashes[min(index, len(dashes)-1)],
			Label:     label,
		}); err != nil {
			return err
		}
	}

	if err := axes.SetXScale("log"); err != nil {
		return err
	}

	// Stopping at 800 Hz rather than at the end of the sweep. Above the
	// axisymmetric family the three curves lie on top of each other — which is
	// the swept area being exactly zero for m > 0, and is worth showing — but a
	// further octave of identical thicket would only shrink the doublet the
	// figure is about.
	axes.SetXLim(grid[0], 800)
	axes.XAxis.Locator = ticker.FixedLocator{
		TicksList: []float64{60, 100, 150, 200, 300, 400, 600, 800},
	}
	axes.XAxis.Formatter = ticker.ScalarFormatter{Prec: 0}
	axes.SetXLabel("frequency (Hz)")
	axes.SetYLim(-70, 6)
	axes.SetYLabel("radiated magnitude (dB re. peak)")

	legend := axes.AddLegend()
	legend.Location = core.LegendLowerLeft
	legend.FrameOn = false

	return fig.Save(out)
}

// drawBandwidth is why the instrument is a hybrid.
//
// A membrane's mode count grows as the square of frequency, so a fixed
// oscillator budget buys bandwidth only as its square root. The staircase is
// what the budget actually reaches; the quadratic is what would have to be
// resolved to keep going; the shaded bands are what the stochastic layer covers
// instead.
func drawBandwidth(data *modelData, out string) error {
	modes := data.byFrequency("")
	if len(modes) == 0 {
		return errNoBatterHead
	}

	batter, ok := data.head("batter")
	if !ok {
		return errNoBatterHead
	}

	fig, axes := newFigure(520)

	for index, band := range data.AttackBands {
		// Each band is about an octave wide at the shipped Q, so the shading is
		// drawn at that width rather than as a line: what the layer covers is a
		// span, and three lines would read as three partials.
		axes.AxVSpan(band.CentreHz/math.Sqrt2, band.CentreHz*math.Sqrt2, core.VSpanOptions{
			Color: optional.Of(fade(warm, 0.10)),
		})

		if index == 0 {
			axes.Text(
				band.CentreHz/math.Sqrt2,
				2200,
				"stochastic attack layer",
				core.TextOptions{FontSize: legendSize, Color: warm, HAlign: core.TextAlignLeft},
			)
		}
	}

	// The Weyl estimate, anchored on nothing: N(f) = (ak)^2/4 with k = 2*pi*f/c
	// is a property of the head's geometry and wave speed alone.
	const steps = 200

	grid := make([]float64, steps)
	weyl := make([]float64, steps)

	for index := range steps {
		fraction := float64(index) / float64(steps-1)
		grid[index] = wideMin * math.Pow(wideMax/wideMin, fraction)
		wavenumber := 2 * math.Pi * grid[index] / batter.WaveSpeedMPerS
		weyl[index] = math.Max(batter.RadiusM*wavenumber*batter.RadiusM*wavenumber/4, 1)
	}

	if _, err := axes.Plot(grid, weyl, core.PlotOptions{
		Color:     optional.Of(muted),
		LineWidth: optional.Of(1.8),
		Dashes:    []float64{8, 5},
		Label:     "modes a membrane has:  N(f) ≈ (ak)²/4",
	}); err != nil {
		return err
	}

	staircaseX := make([]float64, 0, len(modes))
	staircaseY := make([]float64, 0, len(modes))

	for index, mode := range modes {
		staircaseX = append(staircaseX, mode.FrequencyHz)
		staircaseY = append(staircaseY, float64(index+1))
	}

	axes.Step(staircaseX, staircaseY, core.StepOptions{
		Color:     optional.Of(accent),
		LineWidth: optional.Of(2.0),
		Label:     "modes this instrument resolves",
	})

	// The three ceilings fall within a factor of two of each other, so their
	// labels are staggered in height: side by side at this scale they overprint,
	// which is itself the figure's point but not legibly.
	for index, tier := range data.Tiers {
		axes.AxVLine(tier.TopModeHz, core.VLineOptions{
			Color:     optional.Of(rule),
			LineWidth: optional.Of(1.3),
			Dashes:    []float64{3, 3},
		})
		axes.Text(
			tier.TopModeHz*1.04,
			1.25*math.Pow(2.6, float64(index)),
			fmt.Sprintf("%s (%d) → %.0f Hz", tier.Quality, tier.SlotBudget, tier.TopModeHz),
			core.TextOptions{FontSize: legendSize, Color: muted, HAlign: core.TextAlignLeft},
		)
	}

	if err := wideFrequencyAxis(axes); err != nil {
		return err
	}

	if err := axes.SetYScale("log"); err != nil {
		return err
	}

	axes.SetYLim(1, 4000)
	axes.SetYLabel("modes below f")

	legend := axes.AddLegend()
	legend.Location = core.LegendUpperLeft
	legend.FrameOn = false

	return fig.Save(out)
}

// panelFrequencyAxis is logFrequencyAxis for a subplot, which gets the wider
// range because the loss law is what the attack layer is read past the top of.
func panelFrequencyAxis(axes *core.Axes) error {
	if err := axes.SetXScale("log"); err != nil {
		return err
	}

	axes.SetXLim(frequencyMin, wideMax)
	axes.XAxis.Locator = ticker.FixedLocator{TicksList: wideTicks}
	axes.XAxis.Formatter = ticker.ScalarFormatter{Prec: 0}
	axes.SetXLabel("frequency (Hz)")

	return nil
}

func blend(from, to renderColor, ratio float64) renderColor {
	ratio = math.Min(math.Max(ratio, 0), 1)
	from.R += (to.R - from.R) * ratio
	from.G += (to.G - from.G) * ratio
	from.B += (to.B - from.B) * ratio

	return from
}

func finitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}
