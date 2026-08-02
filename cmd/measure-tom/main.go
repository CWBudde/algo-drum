// Command measure-tom turns recordings of a real drum into the derived tables
// docs/physical-measurement-protocol.md asks for: partial frequencies with
// levels, their ratios to the fundamental, per-partial T60 and the damping
// ratio that follows from it, the scatter across repeated hits, and — given a
// resonant-head-off and a resonant-head-on take — the (0,1) doublet that
// Cavity.StiffnessScale is fitted to.
//
// It exists because every number in internal/physical has so far been checked
// against the model or against a published measurement of a different
// instrument. The tables this writes are committable; the audio need not be.
//
// It measures with internal/physical/match — the same WAV loader, onset finder,
// partial detector and decay fitter cmd/fit-physical scores a candidate with —
// so a table produced here and a fit run later are reading the same instrument.
// There is deliberately no second FFT and no second peak picker in this tree.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/cwbudde/algo-drum/internal/physical/match"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "measure-tom: %v\n", err)
		os.Exit(1)
	}
}

// flagWasSet reports whether the named flag was given on the command line, as
// opposed to sitting at its default. FlagSet.Visit — unlike VisitAll — walks
// only the flags that were set, which is the standard way to tell a default
// apart from the same value typed out deliberately.
func flagWasSet(flags *flag.FlagSet, name string) bool {
	set := false

	flags.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})

	return set
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("measure-tom", flag.ContinueOnError)
	flags.SetOutput(stderr)

	defaults := match.DefaultOptions()

	channel := flags.String("channel", string(match.ChannelMono),
		"channel reduction: mono, left or right")
	outputPath := flags.String("o", "",
		"JSON output path, - for stdout, or empty for none")
	quiet := flags.Bool("quiet", false, "suppress the human-readable table")

	analysisSeconds := flags.Float64("analysis-seconds", defaults.AnalysisSeconds,
		"how much of each hit is measured, from the onset")
	maxPartials := flags.Int("max-partials", defaults.MaxPartials,
		"how many partials to retain")
	minHz := flags.Float64("min-hz", defaults.MinFrequencyHz, "lowest partial considered")
	maxHz := flags.Float64("max-hz", defaults.MaxFrequencyHz, "highest partial considered")
	floorDB := flags.Float64("floor-db", defaults.PartialFloorDB,
		"how far below the strongest partial a partial may sit")
	prominenceDB := flags.Float64("prominence-db", defaults.PeakProminenceDB,
		"how far a peak must clear its own skirt to count as a partial")
	decayStart := flags.Float64("decay-start", defaults.DecayFitStartSeconds,
		"where the per-partial decay fit begins, after the onset")
	decayEnd := flags.Float64("decay-end", defaults.DecayFitEndSeconds,
		"where the per-partial decay fit ends; shorten it in a live room")
	// 20 dB, matching match.DefaultOptions().GlidePartialWindowDB, which picks
	// the fundamental for the glide measurement by the same rule and states the
	// reason: wide enough to reach past a strongly radiating upper mode to the
	// fundamental below it, narrow enough that a shell resonance or a room mode
	// well down cannot claim the note. Two tools asking the same question of the
	// same recording should not answer it differently.
	//
	// It was 30 here, and on reference/tt08x08/lp/hd that cost two of the
	// sixteen takes: v04 and v05 have peaks 24.6 and 26.0 dB down, at 199.5 and
	// 159.7 Hz, which claimed the note from the 239 Hz fundamental the other
	// fourteen agree on. Everything keyed to the base followed them — the ratio
	// column, the base T60, the repeatability summary — and the summary read
	// **SD 182 cents, spread 709** for a drum whose tuning is in fact stable to
	// SD 2.9 cents across all sixteen hits. Two misidentifications, not a
	// property of the instrument.
	baseWindowDB := flags.Float64("base-window-db", 20,
		"the ratio base is the lowest partial within this much of the strongest")
	baseHz := flags.Float64("base-hz", 0,
		"take every ratio against the partial nearest this frequency instead")
	baseTolerance := flags.Float64("base-tolerance", 0.1,
		"how far from -base-hz a partial may sit and still be the base, as a fraction")

	espritDefaults := match.DefaultEspritOptions()

	highResolution := flags.Bool("high-resolution", false,
		"measure each take a second time with subband ESPRIT and compare the two tables")
	espritBands := flags.Float64("esprit-bands", espritDefaults.BandsPerOctave,
		"subbands per octave for -high-resolution; narrower bands resolve closer pairs")
	espritOrder := flags.Int("esprit-max-order", espritDefaults.MaxOrder,
		"most components ESTER may put in one subband")

	seriesMode := flags.Bool("series", false,
		"also read the takes as an ordered set: rank correlation of each quantity "+
			"against the take index, and the partials aligned into common modes")
	correspondenceMinTakes := flags.Int("correspondence-min-takes", 2,
		"omit modes seen in fewer takes than this from the printed correspondence table; "+
			"the JSON always carries every row")

	doublet := flags.Bool("doublet", false,
		"treat the two files as Fischer's pair: resonant head off first, on second")
	doubletMinRatio := flags.Float64("doublet-min-ratio", 1.02,
		"lowest upper/lower ratio accepted as the stiffened branch")
	doubletMaxRatio := flags.Float64("doublet-max-ratio", 1.45,
		"highest such ratio; the default stops below the 1.5 the (1,1) family sits at")

	if err := flags.Parse(args); err != nil {
		return err
	}

	paths := flags.Args()
	if len(paths) == 0 {
		return fmt.Errorf("no input files: pass one or more WAV recordings of single hits")
	}

	if *doublet && len(paths) != 2 {
		return fmt.Errorf("-doublet needs exactly two files (resonant head off, then on), got %d", len(paths))
	}

	options := defaults
	options.AnalysisSeconds = *analysisSeconds
	options.MaxPartials = *maxPartials
	options.MinFrequencyHz = *minHz
	options.MaxFrequencyHz = *maxHz
	options.PartialFloorDB = *floorDB
	options.PeakProminenceDB = *prominenceDB
	options.DecayFitStartSeconds = *decayStart
	options.DecayFitEndSeconds = *decayEnd
	// This tool reports Take.Decay, which is off by default because nothing that
	// scores a fit reads it. See match.Options.Diagnostics.
	options.Diagnostics = true

	base := BaseRule{
		WindowDB:   *baseWindowDB,
		ForcedHz:   *baseHz,
		Tolerance:  *baseTolerance,
		Autoselect: *baseHz <= 0,
	}

	// The subspace estimator is asked the same question the fast one is, so
	// where the two have a setting in common it comes from the same flag.
	var esprit *match.EspritOptions

	if *highResolution {
		espritOptions := espritDefaults
		espritOptions.MinFrequencyHz = options.MinFrequencyHz
		espritOptions.MaxFrequencyHz = options.MaxFrequencyHz
		espritOptions.StartSeconds = options.DecayFitStartSeconds
		espritOptions.EndSeconds = options.DecayFitEndSeconds
		espritOptions.FloorDB = options.PartialFloorDB
		espritOptions.BandsPerOctave = *espritBands
		espritOptions.MaxOrder = *espritOrder

		esprit = &espritOptions
	}

	report := Report{Options: options, Base: base}

	// FlagSet.Visit walks only the flags that were actually set, which is how a
	// typed "-channel mono" — a decision about a stereo capture — stays distinct
	// from a -channel nobody passed.
	chosenChannel := flagWasSet(flags, "channel")

	for _, path := range paths {
		take, err := measureTake(path, match.Channel(*channel), chosenChannel, options, base, esprit)
		if err != nil {
			return err
		}

		report.Takes = append(report.Takes, take)
	}

	if *doublet {
		report.Doublet = measureDoublet(report.Takes[0], report.Takes[1],
			*doubletMinRatio, *doubletMaxRatio)
	} else {
		report.Repeatability = measureRepeatability(report.Takes)
	}

	// -series is additive rather than an alternative to the scatter block: the
	// two reductions of the same takes disagree loudly on a ramp, and seeing
	// both is what makes it obvious which question was answered.
	if *seriesMode && !*doublet {
		report.Series = measureSeries(report.Takes)
		report.Correspondence = measureCorrespondence(report.Takes,
			correspondenceTolerance(options.MinSeparationHz))
	}

	if !*quiet && *outputPath != "-" {
		printReport(stdout, report, *correspondenceMinTakes)
	}

	if *outputPath == "" {
		return nil
	}

	return encodeJSON(*outputPath, report, stdout)
}

func printReport(writer io.Writer, report Report, correspondenceMinTakes int) {
	for index, take := range report.Takes {
		if index > 0 {
			_, _ = fmt.Fprintln(writer)
		}

		printTake(writer, take)
	}

	if report.Repeatability != nil {
		_, _ = fmt.Fprintln(writer)
		printRepeatability(writer, *report.Repeatability)
	}

	if report.Series != nil {
		_, _ = fmt.Fprintln(writer)
		printSeries(writer, *report.Series)
	}

	if report.Correspondence != nil {
		_, _ = fmt.Fprintln(writer)
		printCorrespondence(writer, *report.Correspondence, correspondenceMinTakes)
	}

	if report.Doublet != nil {
		_, _ = fmt.Fprintln(writer)
		printDoublet(writer, *report.Doublet)
	}
}

func printTake(writer io.Writer, take Take) {
	_, _ = fmt.Fprintf(writer, "== %s (%s, %.0f Hz, %d-bit, %d ch)\n",
		take.Path, take.Channel, take.Format.SampleRateHz,
		take.Format.BitDepth, take.Format.Channels)

	floor := "unmeasurable"
	if take.Health.PreOnsetFloorDB != nil {
		floor = fmt.Sprintf("%.1f dB", *take.Health.PreOnsetFloorDB)
	}

	_, _ = fmt.Fprintf(writer,
		"   peak %.3f  crest %.2f  clipped %d  DC %+.5f  pre-roll %.2f s  floor %s  analysed %.2f s\n",
		take.Health.PeakAmplitude, take.Health.CrestFactor, take.Health.ClippedSamples,
		take.Health.DCOffset, take.Health.PreOnsetSeconds, floor, take.Health.AnalyzedSeconds)

	for _, warning := range take.Health.Warnings {
		_, _ = fmt.Fprintf(writer, "   ! %s\n", warning)
	}

	// An unmeasured glide prints as "unmeasurable" rather than as +0.0 cents:
	// the fundamental decayed before the late probe could be placed on it, and
	// a zero there would be indistinguishable from a drum that does not bend.
	glide := "unmeasurable"
	if take.GlideMeasured {
		glide = fmt.Sprintf("%+.1f cents", take.GlideCents)
	}

	_, _ = fmt.Fprintf(writer, "   base %.2f Hz  glide %s  attack balance %+.1f dB  RT60 %.2f s\n",
		take.BaseHz, glide, take.AttackBalanceDB, take.Decay.RT60)

	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)

	_, _ = fmt.Fprintln(table, "   #\tf (Hz)\tlevel (dB)\tf/f0\tT60 (s)\tζ (%)\tR²")

	for index, row := range take.Partials {
		_, _ = fmt.Fprintf(table, "   %d\t%.2f\t%.1f\t%.3f\t%s\t%s\t%.2f\n",
			index+1, row.FrequencyHz, row.LevelDB, row.RatioToBase,
			optional(row.T60Seconds, "%.3f"), optional(row.DampingRatioPercent, "%.2f"),
			row.FitQuality)
	}

	_ = table.Flush()

	if take.HighResolution != nil {
		printHighResolution(writer, *take.HighResolution)
	}
}

func printRepeatability(writer io.Writer, repeat Repeatability) {
	_, _ = fmt.Fprintf(writer,
		"== repeatability: scatter across %d takes, the reduction for repeats at one dynamic\n",
		repeat.Takes)
	_, _ = fmt.Fprintf(writer, "   base %.2f Hz, SD %.1f cents, spread %.1f cents\n",
		repeat.MeanBaseHz, repeat.BaseSDCents, repeat.BaseSpreadCents)
	_, _ = fmt.Fprintf(writer, "   base T60 %.3f s, SD %.1f %%\n",
		repeat.MeanBaseT60, repeat.BaseT60SDPct)
	_, _ = fmt.Fprintf(writer, "   peak spread %.1f dB, attack balance SD %.1f dB\n",
		repeat.PeakSpreadDB, repeat.AttackBalanceSDDB)
	_, _ = fmt.Fprintln(writer,
		"   plot base frequency against take peak before calling any of this jitter:")
	_, _ = fmt.Fprintln(writer,
		"   a spread that tracks the level is the tension nonlinearity, working as designed.")
	_, _ = fmt.Fprintln(writer,
		"   if these takes are a velocity ramp rather than repeats, none of the above is jitter:")
	_, _ = fmt.Fprintln(writer,
		"   run -series, which reduces the same takes as an ordered set instead.")
}

func printDoublet(writer io.Writer, doublet Doublet) {
	_, _ = fmt.Fprintln(writer, "== (0,1) doublet — Fischer's protocol")
	_, _ = fmt.Fprintf(writer, "   single head  %s: %.2f Hz\n", doublet.SingleHeadPath, doublet.SingleHz)
	_, _ = fmt.Fprintf(writer, "   two heads    %s: lower %.2f Hz (%.1f dB), upper %.2f Hz (%.1f dB)\n",
		doublet.DoubleHeadPath, doublet.LowerHz, doublet.LowerLevelDB,
		doublet.UpperHz, doublet.UpperLevelDB)
	_, _ = fmt.Fprintf(writer, "   split ratio upper/lower = %.3f   upper/single = %.3f   lower/single = %.3f\n",
		doublet.SplitRatio, doublet.UpperOverSingle, doublet.LowerOverSingle)
	_, _ = fmt.Fprintf(writer, "   (window %.2f-%.2f×; Fischer's snare 1.16, shipped StiffnessScale 0.083 → 1.15)\n",
		doublet.SearchMinRatio, doublet.SearchMaxRatio)

	for _, warning := range doublet.Warnings {
		_, _ = fmt.Fprintf(writer, "   ! %s\n", warning)
	}

	_, _ = fmt.Fprintln(writer, "   candidates above the lower branch:")

	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)

	_, _ = fmt.Fprintln(table, "   \tf (Hz)\tlevel (dB)\tf/lower")

	for _, row := range doublet.Candidates {
		_, _ = fmt.Fprintf(table, "   \t%.2f\t%.1f\t%.3f\n",
			row.FrequencyHz, row.LevelDB, row.FrequencyHz/doublet.LowerHz)
	}

	_ = table.Flush()
}

// optional formats a measurement that may not exist. A dash rather than 0.000,
// because a T60 the fit did not converge on is absent, not instantaneous.
func optional(value float64, format string) string {
	if value <= 0 {
		return "-"
	}

	return fmt.Sprintf(format, value)
}

func encodeJSON(path string, value any, stdout io.Writer) error {
	var (
		writer = stdout
		output *os.File
	)

	if path != "-" {
		var err error

		output, err = os.Create(path)
		if err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}

		writer = output
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(value)

	var closeErr error
	if output != nil {
		closeErr = output.Close()
	}

	if encodeErr != nil {
		return fmt.Errorf("encode JSON: %w", encodeErr)
	}

	if closeErr != nil {
		return fmt.Errorf("close %s: %w", path, closeErr)
	}

	return nil
}
