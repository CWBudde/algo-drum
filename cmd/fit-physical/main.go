// Command fit-physical searches the physical Tom's parameter bank for the
// settings that come closest to a recorded hit.
//
// It renders candidates at the reference's own sample rate, reduces both to
// the same perceptual features, and minimizes their weighted distance with the
// Mayfly Optimization Algorithm. The search space is exactly the bank the
// product exposes, so anything it finds can be typed into the app or shared as
// a link — a fit that needed a hidden parameter would not be a preset.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cwbudde/algo-drum/internal/drum"
	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/algo-drum/internal/physical/match"
	"github.com/cwbudde/mayfly"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "fit-physical: %v\n", err)
		os.Exit(1)
	}
}

// assignmentFlag collects repeatable ID=value pairs.
type assignmentFlag map[string]float64

func (a assignmentFlag) String() string {
	parts := make([]string, 0, len(a))
	for name, value := range a {
		parts = append(parts, fmt.Sprintf("%s=%g", name, value))
	}

	return strings.Join(parts, ",")
}

func (a assignmentFlag) Set(text string) error {
	name, raw, found := strings.Cut(text, "=")
	if !found {
		return fmt.Errorf("%w: expected ID=value, got %q", errInvalidFitOption, text)
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", errInvalidFitOption, name, err)
	}

	a[strings.TrimSpace(name)] = value

	return nil
}

// engineeringFlag is -set: the same pinning as -fix, stated in the parameter's
// own unit rather than as a normalized position.
//
// Two flags rather than one because the two cannot be told apart from the value.
// SIZE is a head diameter in metres over an exponential 0.16–0.50 range, so
// `SIZE=0.2032` is a legal normalized position (0.203 m, as it happens) *and* a
// legal diameter (0.2032 m, normalized 0.2098). Guessing which was meant would
// be wrong about one in two, silently, and the failure mode is a fit against a
// drum of the wrong size that still converges and still reports a total.
//
// -fix is kept as the primary because a report prints normalized positions and
// a resumed search stores them; -set exists for the values that come from
// outside the model, where the instrument is known.
type engineeringFlag map[string]float64

func (e engineeringFlag) String() string {
	return assignmentFlag(e).String()
}

func (e engineeringFlag) Set(text string) error {
	return assignmentFlag(e).Set(text)
}

// resolveEngineering folds -set assignments into the -fix map, converting each
// through its own parameter's curve. A name given to both flags is an error
// rather than a precedence rule: the two would usually disagree, and silently
// preferring one is exactly the failure -set exists to prevent.
func resolveEngineering(fixed assignmentFlag, engineering engineeringFlag) error {
	specs := drum.PhysicalTomSpecs()

	for name, value := range engineering {
		index := slices.IndexFunc(specs, func(spec drum.ParamSpec) bool {
			return spec.ID == name || spec.Label == name
		})
		if index < 0 {
			return fmt.Errorf("%w: no parameter %q", errInvalidFitOption, name)
		}

		spec := specs[index]

		if _, both := fixed[name]; both {
			return fmt.Errorf("%w: %s is given to both -fix and -set", errInvalidFitOption, name)
		}

		// Out of range is refused rather than clamped. Unmap clamps, which is
		// right for a knob and wrong here: a caller who states a 0.60 m head on
		// a range that stops at 0.50 has made a mistake, and silently fitting a
		// 0.50 m drum would answer a question nobody asked.
		if value < spec.Min || value > spec.Max || math.IsNaN(value) {
			return fmt.Errorf("%w: %s = %v %s is outside the model's range %v-%v %s",
				errInvalidFitOption, name, value, spec.Unit, spec.Min, spec.Max, spec.Unit)
		}

		fixed[name] = spec.Unmap(value)
	}

	return nil
}

// pathsFlag collects a repeatable file path, keeping the order it was given in.
//
// -reference used to be a single string and reads the same way when it is given
// once. Repeating it is what asks for a joint fit: one bank against every take,
// each with its own free velocity. The order is preserved so the report and the
// summary name the takes the way the caller listed them — which for a velocity
// series is the file order, and is the thing the fitted velocities are then
// compared against.
type pathsFlag struct {
	paths []string
}

func (p *pathsFlag) String() string {
	return strings.Join(p.paths, " ")
}

func (p *pathsFlag) Set(text string) error {
	path := strings.TrimSpace(text)
	if path == "" {
		return fmt.Errorf("%w: empty reference path", errInvalidFitOption)
	}

	p.paths = append(p.paths, path)

	return nil
}

// duplicate names the first path given twice, or "" when they are all distinct.
//
// A repeated take would be scored twice and would weight that hit double in the
// mean, silently. Refused rather than deduplicated: a caller who listed a file
// twice meant something, and neither guess is safe.
//
// Checked here rather than in Set because flag wraps a Set error with %v, which
// severs the chain errors.Is walks — the caller would get an unidentifiable
// error instead of an invalid option.
func (p *pathsFlag) duplicate() string {
	seen := make(map[string]bool, len(p.paths))

	for _, path := range p.paths {
		if seen[path] {
			return path
		}

		seen[path] = true
	}

	return ""
}

// correctionFlag collects repeatable m,n=rate mode-decay corrections, in the
// order they were given.
type correctionFlag struct {
	entries []physical.ModeDecayCorrection
}

func (c *correctionFlag) String() string {
	parts := make([]string, 0, len(c.entries))
	for _, entry := range c.entries {
		parts = append(parts, fmt.Sprintf("%d,%d=%g",
			entry.AzimuthalOrder, entry.RadialOrder, entry.DecayRatePerSecond))
	}

	return strings.Join(parts, " ")
}

func (c *correctionFlag) Set(text string) error {
	mode, raw, found := strings.Cut(text, "=")
	if !found {
		return fmt.Errorf("%w: expected m,n=rate, got %q", errInvalidFitOption, text)
	}

	azimuthal, radial, found := strings.Cut(mode, ",")
	if !found {
		return fmt.Errorf("%w: expected m,n=rate, got %q", errInvalidFitOption, text)
	}

	entry := physical.ModeDecayCorrection{}

	if err := parseOrder(&entry.AzimuthalOrder, azimuthal, "azimuthal order"); err != nil {
		return err
	}

	if err := parseOrder(&entry.RadialOrder, radial, "radial order"); err != nil {
		return err
	}

	// The radial order starts at 1 — there is no n = 0 Bessel zero — and a
	// negative rate would be an energy source rather than a loss. Both are
	// rejected here so the flag fails on the command line instead of turning
	// every candidate into +Inf several minutes later.
	if entry.RadialOrder < 1 {
		return fmt.Errorf("%w: radial order %d: modes are numbered from 1",
			errInvalidFitOption, entry.RadialOrder)
	}

	rate, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", errInvalidFitOption, mode, err)
	}

	if rate < 0 || math.IsNaN(rate) {
		return fmt.Errorf("%w: rate %v at (%d,%d) is not a loss",
			errInvalidFitOption, rate, entry.AzimuthalOrder, entry.RadialOrder)
	}

	entry.DecayRatePerSecond = rate
	c.entries = append(c.entries, entry)

	return nil
}

func parseOrder(target *int, text, what string) error {
	order, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return fmt.Errorf("%w: %s: %w", errInvalidFitOption, what, err)
	}

	if order < 0 {
		return fmt.Errorf("%w: %s %d is negative", errInvalidFitOption, what, order)
	}

	*target = order

	return nil
}

// flagWasSet reports whether the named flag was actually given on the command
// line, as opposed to sitting at its default.
//
// FlagSet.Visit — unlike VisitAll — walks only the flags that were set, which is
// the standard way to draw that distinction. It matters wherever a default is a
// legitimate value in its own right: -channel mono typed out is a choice about
// how a stereo capture is reduced, while an absent -channel is nobody having
// thought about it, and the two must not be answered the same way.
func flagWasSet(flags *flag.FlagSet, name string) bool {
	set := false

	flags.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})

	return set
}

// Report is what the run writes out.
//
// References and Targets are parallel and in the order the takes were given, and
// both are lists even for a single-take run: a report that sometimes holds one
// reference and sometimes a list would need every reader to handle both, and the
// list of one is the same thing said once.
type Report struct {
	References []ReferenceInfo `json:"references"`
	Options    match.Options   `json:"options"`
	Weights    match.Weights   `json:"weights"`
	// Gates is Weights stated the other way round — the value of each term at
	// which a candidate stops being distinguishable from a second recording of
	// the reference. Recorded beside the weights it is the reciprocal of so that
	// a report can be read without the matching build of the code beside it, and
	// so that every termsVsGate number in it can be checked.
	Gates match.Weights `json:"gates"`
	// WeightsFingerprint identifies that weight set, so two reports can be
	// checked comparable before their totals are compared. A total means nothing
	// across a gate edit; see weightsFingerprint for the incident this is here
	// to prevent repeating.
	WeightsFingerprint string `json:"weightsFingerprint"`
	// ObjectiveFloor is what -floor was told, and zero when it was told nothing.
	// It is the objective's own disagreement with itself on *this* reference set,
	// below which a total is not distinguishable from noise — a measurement
	// cmd/measure-objective makes, a property of one drum at one tuning, and
	// deliberately not a constant anywhere in this tool.
	ObjectiveFloor float64          `json:"objectiveFloor,omitempty"`
	Search         SearchInfo       `json:"search"`
	Baseline       Candidate        `json:"baseline"`
	Best           *Candidate       `json:"best,omitempty"`
	Targets        []match.Features `json:"targets"`
}

// ReferenceInfo records what was measured, so a report can be read a year
// later without the file beside it.
type ReferenceInfo struct {
	Path         string  `json:"path"`
	Channel      string  `json:"channel"`
	SampleRateHz float64 `json:"sampleRateHz"`
	Channels     int     `json:"channels"`
	BitDepth     int     `json:"bitDepth"`
	Frames       int     `json:"frames"`
}

// SearchInfo records enough to repeat the run.
type SearchInfo struct {
	Variant         string  `json:"variant"`
	Iterations      int     `json:"iterations"`
	Population      int     `json:"population"`
	Restarts        int     `json:"restarts"`
	Seed            int64   `json:"seed"`
	Evaluations     int     `json:"evaluations"`
	DurationSeconds float64 `json:"durationSeconds"`
	Quality         string  `json:"quality"`
	Contact         string  `json:"contact,omitempty"`
	MalletGrams     float64 `json:"malletGrams,omitempty"`
	// LossScale is 1 for every run that stays inside the shipped ranges, and
	// anything else marks a report whose bank the product cannot express.
	LossScale float64 `json:"lossScale,omitempty"`
	// SearchBlind marks a report whose search included the parameters the
	// objective is measured to be blind to. Such a run is evidence about the
	// objective and not a bank to ship, so a report carrying it should not be
	// read as a fit result. See blindParameters.
	SearchBlind bool `json:"searchBlind,omitempty"`
	// ModeCorrections records any -mode-correction overrides, for the same
	// reason LossScale is here: they describe a drum whose correction table the
	// product does not ship, so the bank in such a report is not one either.
	ModeCorrections string `json:"modeCorrections,omitempty"`
	// SeedErrorCents records what the analytic pre-solve achieved for each
	// seeded restart, best first. It is the honest caption for a seeded run: a
	// low number here says the starting point matched the reference's partial
	// frequencies, and nothing whatever about the fit that followed.
	SeedErrorCents []float64 `json:"seedErrorCents,omitempty"`
	SeedWidth      float64   `json:"seedWidth,omitempty"`
	// Interrupted marks a report the search did not finish producing. It also
	// qualifies Evaluations, which counts mayfly's calls rather than rendered
	// candidates: after an interrupt the objective refuses work without
	// measuring it, so the tail of that count is the search winding up.
	Interrupted bool               `json:"interrupted,omitempty"`
	FixedParams map[string]float64 `json:"fixedParams,omitempty"`
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("fit-physical", flag.ContinueOnError)
	flags.SetOutput(stderr)

	references := &pathsFlag{}
	flags.Var(references, "reference",
		"reference WAV file (required; repeatable — every take given is fitted "+
			"jointly by one bank, each at its own velocity)")

	channel := flags.String("channel", string(match.ChannelMono), "channel reduction: mono, left or right")
	outputPath := flags.String("o", "-", "JSON report path, or - for stdout")
	// Defaulted from the analysis span rather than written out, because the two
	// are not independent: a candidate rendered for less than the objective
	// measures is scored partly on silence, and the reference is not. They were
	// separate literals until PLAN N17 and had already drifted apart by 0.4 s.
	duration := flags.Float64("duration", match.DefaultOptions().AnalysisSeconds,
		"candidate render duration in seconds; must cover the analysis span")
	quality := flags.String("quality", string(physical.QualityDraft),
		"mode budget during the search: draft, standard or high")
	contact := flags.String("contact", "",
		"excitation model: prescribed, hertzian, or empty for the configured default")
	malletGrams := flags.Float64("mallet-g", 0,
		"stick mass in grams, or 0 for the configured default")
	lossScale := flags.Float64("loss-scale", 1,
		"extra multiplier on every head loss rate, to search past DAMP's own range")
	seededRestarts := flags.Int("seeded-restarts", 0,
		"start this many restarts from an analytic solve for the reference's partial frequencies")
	seedWidth := flags.Float64("seed-width", 0.25,
		"half-width of the box a seeded restart searches around its seed")
	variant := flags.String("variant", "ma", "mayfly variant: ma, desma, olce, eobbma, gsasma, mpma or aoblmoa")
	iterations := flags.Int("iterations", 150, "mayfly iterations per restart")
	population := flags.Int("pop", 20, "mayfly males, and as many females")
	restarts := flags.Int("restarts", 0, "independent seeded runs, best of; 0 picks one per available core")
	seed := flags.Int64("seed", 1, "base RNG seed; restart n uses seed+n")
	reportOnly := flags.Bool("report-only", false, "measure the shipped defaults against the reference and stop")
	velocity := flags.Float64("velocity", defaultVelocity,
		"strike velocity for -report-only, in the search's own 0-1 units; ignored when searching")
	progressEvery := flags.Int("progress", 500,
		"print a progress line every N objective evaluations; 0 silences it")
	checkpointPath := flags.String("checkpoint", "",
		"file to save finished restarts and the best point to, and to resume from")
	inspect := flags.Bool("inspect", false,
		"describe the -checkpoint file's best point and stop, without searching")
	wavPath := flags.String("wav", "",
		"also render the fitted bank to this mono WAV file, for listening to")
	wavDuration := flags.Duration("wav-duration", 3*time.Second,
		"render duration for -wav; the search's own -duration is far too short to judge a tail by")
	wavTake := flags.Int("wav-take", 1,
		"which take's fitted velocity -wav renders at, counting from 1 in the "+
			"order -reference was given; a joint fit has one velocity per take "+
			"and no single one of them is the drum")

	fixed := assignmentFlag{}
	flags.Var(fixed, "fix", "freeze one parameter at a normalized position, as ID=value (repeatable)")

	engineering := engineeringFlag{}
	flags.Var(engineering, "set",
		"freeze one parameter at a value in its own unit — metres, N/m, degrees — "+
			"as ID=value (repeatable); use for what the instrument is known to be")

	corrections := &correctionFlag{}
	flags.Var(corrections, "mode-correction",
		"add a decay rate to one mode's loss law, as m,n=perSecond (repeatable); "+
			"a run with this set measures a correction table the product does not ship")

	searchBlind := flags.Bool("search-blind", false,
		"also search the parameters the objective is measured to be blind to; "+
			"a run with this set is evidence about the objective, not a bank to ship")

	// No default, and there will never be one. The floor is a property of a
	// reference set — 6.32 median on reference/tt08x08/lp/hd is that drum at that
	// tuning through this estimator, and it does not transfer to another set — so
	// baking any number in would hand every future run a threshold measured on a
	// drum it is not aiming at. That is the exact mistake this repository has made
	// twice with gates; see match.DefaultWeights.
	floor := flags.Float64("floor", 0,
		"the objective's own disagreement with itself on this reference set, from "+
			"cmd/measure-objective; printed and recorded beside the totals, since a "+
			"total below it is not distinguishable from the objective's noise")

	schema := flags.Bool("schema", false,
		"print the shape of the JSON report and exit")

	if err := flags.Parse(args); err != nil {
		return err
	}

	// Before every other check, because -schema answers a question about the
	// report format rather than about a run: requiring a -reference to be told
	// where a field lives would make the flag useless exactly when it is wanted.
	if *schema {
		writeSchema(stdout)

		return nil
	}

	if math.IsNaN(*floor) || *floor < 0 {
		return fmt.Errorf("%w: floor %v", errInvalidFitOption, *floor)
	}

	if err := resolveEngineering(fixed, engineering); err != nil {
		return err
	}

	if flags.NArg() > 0 {
		return fmt.Errorf("%w: unexpected argument %q", errInvalidFitOption, flags.Arg(0))
	}

	if len(references.paths) == 0 {
		return fmt.Errorf("%w: -reference is required", errInvalidFitOption)
	}

	if repeated := references.duplicate(); repeated != "" {
		return fmt.Errorf("%w: reference %q given twice", errInvalidFitOption, repeated)
	}

	if *wavTake < 1 || *wavTake > len(references.paths) {
		return fmt.Errorf("%w: wav take %d, with %d reference(s) given",
			errInvalidFitOption, *wavTake, len(references.paths))
	}

	if *duration <= 0 {
		return fmt.Errorf("%w: duration %v", errInvalidFitOption, *duration)
	}

	// A candidate shorter than the analysis span is scored on its own silence:
	// every partial's decay is fitted through a window that runs off the end of
	// the render, so the model is rewarded for stopping rather than for ringing
	// like the reference. Refused rather than clamped, because the two numbers
	// disagreeing means one of them was meant to be something else.
	if analysis := match.DefaultOptions().AnalysisSeconds; *duration < analysis {
		return fmt.Errorf("%w: duration %v s is shorter than the %v s the objective analyses",
			errInvalidFitOption, *duration, analysis)
	}

	if *iterations <= 0 || *population < 4 {
		return fmt.Errorf("%w: iterations %d, population %d (population must be at least 4)",
			errInvalidFitOption, *iterations, *population)
	}

	if *restarts < 0 {
		return fmt.Errorf("%w: restarts %d", errInvalidFitOption, *restarts)
	}

	if *progressEvery < 0 {
		return fmt.Errorf("%w: progress %d", errInvalidFitOption, *progressEvery)
	}

	switch physical.ContactModel(*contact) {
	case "", physical.ContactPrescribed, physical.ContactHertzian:
	default:
		return fmt.Errorf("%w: contact %q is neither %q nor %q",
			errInvalidFitOption, *contact, physical.ContactPrescribed, physical.ContactHertzian)
	}

	// Bounded by physical.Validate's own range for the mass, so an out-of-range
	// value is rejected here rather than turning every candidate into +Inf.
	if *malletGrams < 0 || *malletGrams > 1000 {
		return fmt.Errorf("%w: mallet mass %v g", errInvalidFitOption, *malletGrams)
	}

	// Bounded well inside what the loss law can carry: at zero the heads never
	// stop and every decay measure degenerates, and past a hundredfold the
	// modes are gone before the first analysis window closes.
	if *lossScale <= 0.01 || *lossScale > 100 {
		return fmt.Errorf("%w: loss scale %v", errInvalidFitOption, *lossScale)
	}

	// Velocity is the one dimension of the search space -fix cannot reach, and
	// it is not a gain: it sets the contact duration, the size of the nonlinear
	// glide and the balance between the modal and stochastic layers. Re-scoring
	// an archived report's bank at the default 1.0 when the run that produced it
	// recorded, say, 0.921 therefore measures a different drum, and biases the
	// level, attack-balance and envelope terms — which is exactly what a
	// re-score under a changed objective is supposed to hold still. Zero is
	// rejected rather than clamped for the same reason the other numeric flags
	// are: it is not a quiet strike but no strike at all, and every measure taken
	// from the silence that follows is meaningless.
	if math.IsNaN(*velocity) || *velocity <= 0 || *velocity > 1 {
		return fmt.Errorf("%w: velocity %v is not inside (0, 1]", errInvalidFitOption, *velocity)
	}

	if *seededRestarts < 0 {
		return fmt.Errorf("%w: seeded restarts %d", errInvalidFitOption, *seededRestarts)
	}

	// Half a cube is the whole cube, so anything at or past it is not a box and
	// is rejected rather than silently ignored.
	if *seededRestarts > 0 && (*seedWidth <= 0 || *seedWidth >= 0.5) {
		return fmt.Errorf("%w: seed width %v is not inside (0, 0.5)",
			errInvalidFitOption, *seedWidth)
	}

	if *restarts == 0 {
		*restarts = max(1, runtime.NumCPU()-1)
	}

	// QUAL is pinned rather than fitted. It buys mode count with CPU, so a
	// search left free would always spend its way to High and then ship a tom
	// that costs twice what the app budgeted for — a decision about the
	// product, not about the sound of this drum.
	if err := pinQuality(fixed, *quality); err != nil {
		return err
	}

	bank, free, err := resolveFixed(fixed, !*reportOnly, *searchBlind)
	if err != nil {
		return err
	}

	// flagWasSet is what separates "-channel mono" — a decision — from a -channel
	// nobody typed. See match.LoadReferenceExplicit for why that difference is
	// worth refusing to start a run over: the reference this guard was written
	// for was stereo, every total archived against it was its right channel,
	// and the defaulted flag silently fitted the average of the two instead. The
	// not-chosen case is reported as an invalid option because that is what it
	// is — a missing flag, not a broken file — while a genuinely unreadable
	// reference keeps its own error.
	options := match.DefaultOptions()

	var (
		info         = make([]ReferenceInfo, len(references.paths))
		targets      = make([]match.Features, len(references.paths))
		sampleRateHz float64
	)

	for index, path := range references.paths {
		reference, err := match.LoadReferenceExplicit(path, match.Channel(*channel),
			flagWasSet(flags, "channel"))
		if err != nil {
			if errors.Is(err, match.ErrChannelNotChosen) {
				return fmt.Errorf("%w: %w", errInvalidFitOption, err)
			}

			return err
		}

		// Every take is rendered from one buffer at one rate, and a resampler
		// is not allowed anywhere in the measurement path on either side. A
		// series recorded at two rates is therefore refused rather than
		// converted — and in practice means two different sessions have been
		// listed as one drum, which is the more useful thing to be told.
		if index == 0 {
			sampleRateHz = reference.SampleRateHz
		} else if reference.SampleRateHz != sampleRateHz {
			return fmt.Errorf("%w: %s is %g Hz but %s is %g Hz",
				errInvalidFitOption, path, reference.SampleRateHz,
				references.paths[0], sampleRateHz)
		}

		target, err := match.Extract(reference.Samples, reference.SampleRateHz, options)
		if err != nil {
			return fmt.Errorf("measure reference %s: %w", path, err)
		}

		info[index] = ReferenceInfo{
			Path:         path,
			Channel:      *channel,
			SampleRateHz: reference.SampleRateHz,
			Channels:     reference.Channels,
			BitDepth:     reference.BitDepth,
			Frames:       len(reference.Samples),
		}
		targets[index] = target
	}

	base := &evaluator{
		references:      targets,
		referencePaths:  references.paths,
		options:         options,
		weights:         match.DefaultWeights(),
		bank:            bank,
		free:            free,
		sampleRateHz:    sampleRateHz,
		durationSeconds: *duration,
		contact:         physical.ContactModel(*contact),
		malletMassKg:    *malletGrams / 1000,
		lossScale:       *lossScale,
		corrections:     corrections.entries,
		// Rendered at the reference's own rate, so no resampler ever enters
		// the measurement path on either side.
		buffer:     make([]float64, int(*duration*sampleRateHz)),
		velocities: make([]float64, len(targets)),
		rendered:   make([]match.Features, len(targets)),
	}

	report := Report{
		References:         info,
		Options:            options,
		Weights:            base.weights,
		Gates:              match.AdoptionGates(),
		WeightsFingerprint: weightsFingerprint(base.weights),
		ObjectiveFloor:     *floor,
		Targets:            targets,
		Search: SearchInfo{
			Variant:         *variant,
			Iterations:      *iterations,
			Population:      *population,
			Restarts:        *restarts,
			Seed:            *seed,
			DurationSeconds: *duration,
			Quality:         *quality,
			Contact:         *contact,
			MalletGrams:     *malletGrams,
			LossScale:       *lossScale,
			SearchBlind:     *searchBlind,
			ModeCorrections: corrections.String(),
			FixedParams:     fixed,
		},
	}

	baseline := newEvaluator(base)

	// The shipped defaults, at the velocity the product's own audition uses —
	// unless -report-only asked for another one. A search keeps quoting its
	// baseline at the default no matter what -velocity says, because that cost
	// goes into the checkpoint fingerprint: letting a flag the search itself
	// ignores move it would make two otherwise identical runs refuse to resume
	// each other.
	baselineVelocity := defaultVelocity
	if *reportOnly {
		baselineVelocity = *velocity
	}

	report.Baseline, err = baseline.describe(baseline.position(baselineVelocity))
	if err != nil {
		return fmt.Errorf("measure the shipped defaults: %w", err)
	}

	for index, target := range targets {
		_, _ = fmt.Fprintf(stderr,
			"reference: %-40s %3d partials, fundamental %.2f Hz, glide %.1f cents\n",
			references.paths[index], len(target.Partials), fundamentalHz(target), target.GlideCents)
	}

	if len(targets) > 1 {
		_, _ = fmt.Fprintf(stderr,
			"joint fit: one bank against %d takes, %d free parameters and %d velocities\n",
			len(targets), len(free), len(targets))
	}

	_, _ = fmt.Fprintf(stderr, "baseline:  %s\n", summarize(report.Baseline.Terms))

	// The pre-solve is analytic and takes a second or two, so it runs before the
	// checkpoint is opened and its outcome goes into the fingerprint: a resume
	// that reproduced different seeds would be resuming a different search.
	var (
		seeds    []seedCandidate
		relevant []bool
	)

	if !*reportOnly && *seededRestarts > 0 {
		// Seeded from the first take alone. The pre-solve matches modal
		// frequencies, and a mode's frequency is a property of the drum rather
		// than of how hard it was hit — the glide the strike does add is what
		// N5 is trying to measure, so folding sixteen takes' worth of it into
		// one seed would blur exactly the signal. It is a starting point, not a
		// result: the unseeded restarts are what would find it wrong.
		presolve := rand.New(rand.NewSource(*seed))
		relevant = frequencyRelevant(targets[0], bank, free,
			options.PartialFloorDB, sampleRateHz, presolve)
		seeds = frequencySeeds(targets[0], bank, free, min(*seededRestarts, *restarts),
			options.PartialFloorDB, sampleRateHz, presolve, defaultSeedBudget)

		named := make([]string, 0, len(free))
		specs := drum.PhysicalTomSpecs()

		for i, index := range free {
			if relevant[i] {
				named = append(named, specs[index].Label)
			}
		}

		_, _ = fmt.Fprintf(stderr, "  seeding %d of %d free parameters: %s\n",
			len(named), len(free), strings.Join(named, " "))

		for index, candidate := range seeds {
			_, _ = fmt.Fprintf(stderr, "  seed %d: %.1f cents from the reference's partials\n",
				index+1, candidate.errorCents)

			report.Search.SeedErrorCents = append(report.Search.SeedErrorCents, candidate.errorCents)
		}

		report.Search.SeedWidth = *seedWidth
	}

	// Only meaningful when something is seeded, so an unseeded run does not
	// carry a width none of its restarts ever used into the fingerprint.
	fingerprintSeedWidth := 0.0
	if len(seeds) > 0 {
		fingerprintSeedWidth = *seedWidth
	}

	if !*reportOnly {
		// The baseline goes into the fingerprint, so it has to be measured
		// before the checkpoint is opened — which it is, just above.
		checkpoint, err := loadStore(*checkpointPath, Fingerprint{
			SeededRestarts:  len(seeds),
			SeedWidth:       fingerprintSeedWidth,
			Reference:       strings.Join(references.paths, "\n"),
			Channel:         *channel,
			Contact:         *contact,
			MalletGrams:     *malletGrams,
			LossScale:       *lossScale,
			SearchBlind:     *searchBlind,
			ModeCorrections: corrections.String(),
			Quality:         *quality,
			Variant:         *variant,
			DurationSeconds: *duration,
			Iterations:      *iterations,
			Population:      *population,
			Restarts:        *restarts,
			Seed:            *seed,
			Fixed:           fixed,
			BaselineCost:    report.Baseline.Terms.Total,
		})
		if err != nil {
			return err
		}

		if resumed := len(checkpoint.completed()); resumed > 0 {
			_, _ = fmt.Fprintf(stderr, "resuming:  %d of %d restarts already finished\n",
				resumed, *restarts)
		}

		// -inspect reads a checkpoint and stops. The point is to see a running
		// fit's best candidate broken down by term without interrupting it:
		// checkpoints are written atomically, so a reader gets one whole version
		// or another, and the search never learns it was read. The fingerprint
		// guard still applies, which is what stops this describing one run's
		// point against another run's reference.
		if *inspect {
			snapshot := checkpoint.best()
			if snapshot == nil {
				return fmt.Errorf("%w: %s holds no best point yet",
					errInvalidFitOption, *checkpointPath)
			}

			candidate, err := base.describe(snapshot.Position)
			if err != nil {
				return err
			}

			report.Best = &candidate
			report.Search.Evaluations = snapshot.Evaluations
			report.Search.Interrupted = true

			_, _ = fmt.Fprintf(stderr, "inspected: %s after %d evaluations\n",
				summarize(candidate.Terms), snapshot.Evaluations)

			// Same tail as a finished run, -wav included. A checkpoint holds the
			// bank, so the point it describes can be listened to as well as read
			// — which matters here more than the convenience suggests: twice now
			// a defect in this metric has been found by hearing something the
			// distance called good, and a run one has to finish before it can be
			// auditioned is one nobody auditions.
			return finish(stdout, stderr, report, *wavPath, *wavDuration, *wavTake, *outputPath)
		}

		// An interrupt asks the search to wind up, not the process to die: the
		// objective starts refusing work, every restart returns the best it
		// actually found, and the report and checkpoint are written as usual. A
		// second signal takes the default behaviour and kills it outright.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		offspring := *population - *population%2
		progress := newTracker(stderr, *progressEvery,
			expectedEvaluations(*restarts, *iterations, *population, offspring), time.Now())
		progress.checkpoint = checkpoint

		best, evaluations, err := search(
			ctx, base, *variant, *iterations, *population, *restarts, *seed,
			seeds, relevant, *seedWidth, stderr, progress, checkpoint,
		)
		if err != nil {
			return err
		}

		if err := checkpoint.flush(); err != nil {
			return err
		}

		if ctx.Err() != nil {
			report.Search.Interrupted = true

			_, _ = fmt.Fprintf(stderr,
				"interrupted; the report below is the best point found so far\n")
		}

		report.Best = &best
		report.Search.Evaluations = evaluations

		_, _ = fmt.Fprintf(stderr, "best:      %s\n", summarize(best.Terms))
	}

	return finish(stdout, stderr, report, *wavPath, *wavDuration, *wavTake, *outputPath)
}

// finish writes what a run leaves behind: the summary, the optional WAV, and
// the JSON report. Shared so that -inspect leaves the same things behind as a
// completed run, rather than a subset of them.
func finish(
	stdout, stderr io.Writer,
	report Report,
	wavPath string,
	wavDuration time.Duration,
	wavTake int,
	outputPath string,
) error {
	writeSummary(stdout, report)

	// The baseline is the fallback for the same reason writeSummary uses it: with
	// -report-only there is no fitted bank, and the shipped one is what the run
	// measured.
	if wavPath != "" {
		exported := report.Baseline
		if report.Best != nil {
			exported = *report.Best
		}

		take := exported.Takes[wavTake-1]

		peak, err := exportCandidate(wavPath, exported, take.Velocity01, wavDuration)
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintf(stderr,
			"wrote %s: %.2fs at velocity %.3f, the fit for %s; source peak %.6g\n",
			wavPath, wavDuration.Seconds(), take.Velocity01, take.Path, peak)
	}

	return writeReport(outputPath, report)
}

// defaultVelocity is the audition level the VoiceEditor triggers at, and the
// obvious level at which to quote a baseline.
const defaultVelocity = 1.0

func pinQuality(fixed assignmentFlag, quality string) error {
	specs := drum.PhysicalTomSpecs()

	index := -1

	for i, spec := range specs {
		if spec.ID == "physicalTom.quality" {
			index = i
		}
	}

	if index < 0 {
		return fmt.Errorf("%w: the parameter table has no quality entry", errInvalidFitOption)
	}

	spec := specs[index]

	tier := -1

	for i, choice := range spec.Choices {
		if strings.EqualFold(choice, quality) {
			tier = i
		}
	}

	if tier < 0 {
		return fmt.Errorf("%w: quality %q is not one of %v",
			errInvalidFitOption, quality, spec.Choices)
	}

	if _, already := fixed[spec.ID]; !already {
		fixed[spec.ID] = float64(tier) / float64(len(spec.Choices)-1)
	}

	return nil
}

// search runs independent mayfly runs concurrently and keeps the best.
//
// The concurrency is between runs rather than inside one, because mayfly calls
// the objective sequentially. Multi-start is what this landscape wants anyway:
// the detent in every knob's mapping puts a small flat spot at each default,
// and a single swarm can settle into one.
func search(
	ctx context.Context,
	base *evaluator,
	variant string,
	iterations, population, restarts int,
	seed int64,
	seeds []seedCandidate,
	relevant []bool,
	seedWidth float64,
	stderr io.Writer,
	progress *tracker,
	checkpoint *store,
) (Candidate, int, error) {
	// Restarts a previous run finished are not re-run. Their positions are
	// replayed through describe rather than stored as candidates, so the report
	// is written by this build's measurement code and a resumed report says the
	// same thing an uninterrupted one would.
	done := checkpoint.completed()

	type outcome struct {
		candidate   Candidate
		evaluations int
		convergence []float64
		err         error
	}

	results := make([]outcome, restarts)

	var group sync.WaitGroup

	for run := range restarts {
		group.Add(1)

		go func() {
			defer group.Done()

			// Since mayfly v0.2.0 an oversized NC is a returned error rather
			// than a panic from inside the library, so this recover no longer
			// has a known trigger. It stays as a backstop: the objective runs
			// third-party numerics on adversarially-chosen parameters, and one
			// restart dying should never take the other seven with it.
			defer func() {
				if recovered := recover(); recovered != nil {
					results[run] = outcome{err: fmt.Errorf(
						"%w: restart %d panicked inside the optimizer: %v",
						errInvalidFitOption, run+1, recovered,
					)}
				}
			}()

			local := newEvaluator(base)

			if record, ok := done[run]; ok {
				candidate, err := local.describe(record.Position)
				if err != nil {
					results[run] = outcome{err: err}

					return
				}

				_, _ = fmt.Fprintf(stderr, "  restart %d/%d: total %.3f from the checkpoint\n",
					run+1, restarts, candidate.Terms.Total)

				results[run] = outcome{
					candidate:   candidate,
					evaluations: record.Evaluations,
					convergence: record.Convergence,
				}

				return
			}

			// Seeded restarts come first, so an -seeded-restarts below the
			// restart count leaves the remainder searching the whole cube. That
			// mix is the point: a seed is a hypothesis about where to look, and
			// the unseeded restarts are what would find it wrong.
			//
			// The box is applied here and nowhere else, so the position that
			// reaches the objective, the progress tracker, the checkpoint and
			// the report is always an ordinary bank position. A stored point
			// therefore means the same thing whether its restart was seeded.
			warp := boxAround(nil, nil, 0)
			if run < len(seeds) {
				warp = boxAround(seeds[run].position, relevant, seedWidth)
			}

			// Progress is reported from inside the objective because mayfly has
			// no per-iteration hook: without this the run says nothing until a
			// whole restart finishes.
			objective := func(raw []float64) float64 {
				position := warp(raw)

				// Cancellation is cooperative for the same reason: Optimize
				// cannot be interrupted, so an abandoned evaluation returns the
				// cost of a configuration that is not a drum. mayfly keeps its
				// incumbent, so the restart still reports the best it genuinely
				// found — the run unwinds in seconds without discarding it.
				if ctx.Err() != nil {
					return math.Inf(1)
				}

				cost := local.cost(position)
				progress.observe(cost, position)

				return cost
			}

			config, err := mayfly.NewBuilder(variant).
				ForProblem(objective, local.dimensions(), 0, 1).
				WithIterations(iterations).
				WithPopulation(population, population).
				WithConfig(func(settings *mayfly.Config) {
					settings.Rand = rand.New(rand.NewSource(seed + int64(run)))
					// One offspring per pair, which is the paper's ratio. The
					// library's fixed default of 20 would exceed the population
					// for any -pop below 10; v0.2.0 rejects that rather than
					// indexing past the end, but rejecting is still not what we
					// want here.
					settings.NC = population - population%2
				}).
				Build()
			if err != nil {
				results[run] = outcome{err: err}

				return
			}

			result, err := mayfly.Optimize(config)
			if err != nil {
				results[run] = outcome{err: err}

				return
			}

			candidate, err := local.describe(warp(result.GlobalBest.Position))
			if err != nil {
				results[run] = outcome{err: err}

				return
			}

			// A restart the interrupt cut short is recorded but not marked
			// complete: it saw fewer real evaluations than the run asked for, so
			// a later resume must run it again rather than adopt its answer.
			complete := ctx.Err() == nil

			state := "after"
			if !complete {
				state = "interrupted after"
			}

			_, _ = fmt.Fprintf(stderr, "  restart %d/%d: total %.3f %s %d evaluations\n",
				run+1, restarts, result.GlobalBest.Cost, state, result.FuncEvalCount)

			if err := checkpoint.recordRestart(RestartRecord{
				Run:         run,
				Seed:        seed + int64(run),
				Complete:    complete,
				Cost:        result.GlobalBest.Cost,
				Position:    result.GlobalBest.Position,
				Evaluations: result.FuncEvalCount,
				Convergence: result.ConvergenceCurve,
			}); err != nil {
				results[run] = outcome{err: err}

				return
			}

			results[run] = outcome{
				candidate:   candidate,
				evaluations: result.FuncEvalCount,
				convergence: result.ConvergenceCurve,
			}
		}()
	}

	group.Wait()

	best := Candidate{}
	evaluations := 0
	found := false

	for _, result := range results {
		if result.err != nil {
			return Candidate{}, 0, result.err
		}

		evaluations += result.evaluations

		if !found || result.candidate.Terms.Total < best.Terms.Total {
			best, found = result.candidate, true
			best.Convergence = result.convergence
		}
	}

	// The running snapshot is a candidate in its own right. It is usually
	// beaten by the restart that produced it — a swarm's incumbent is its own
	// global best — but after an interrupt it can be the only thing left, and a
	// point that measured better is better whatever produced it.
	if snapshot := checkpoint.best(); snapshot != nil && (!found || snapshot.Cost < best.Terms.Total) {
		candidate, err := newEvaluator(base).describe(snapshot.Position)
		if err != nil {
			return Candidate{}, 0, err
		}

		if !found || candidate.Terms.Total < best.Terms.Total {
			_, _ = fmt.Fprintf(stderr, "  best point came from the checkpoint: total %.3f\n",
				candidate.Terms.Total)

			best, found = candidate, true
		}
	}

	if !found {
		return Candidate{}, 0, fmt.Errorf("%w: no restart produced a result", errInvalidFitOption)
	}

	return best, evaluations, nil
}

func fundamentalHz(features match.Features) float64 {
	if len(features.Partials) == 0 {
		return 0
	}

	return features.Partials[0].FrequencyHz
}

// summarize is the one-line form of a score, and every term carries its ×gate
// beside its raw value.
//
// The raw value alone is close to unreadable: nine numbers in seven units, of
// which "spectrum 14.3 dB" is a catastrophe and "glide 25.2¢" is comfortably
// inside tolerance, and nothing on the line said so. The multiplier is the same
// number in every term, it is that term's whole contribution to the total, and
// the nine of them add up to the total printed at the front — so a reader can
// see at a glance which term is paying for the fit.
func summarize(terms match.Terms) string {
	fields := termFields(terms)
	parts := make([]string, 0, len(fields))

	for _, field := range fields {
		parts = append(parts, field.String())
	}

	return fmt.Sprintf("total %.3f (%s)", terms.Total, strings.Join(parts, ", "))
}

// writeSummary prints the table a person actually reads.
func writeSummary(stdout io.Writer, report Report) {
	_, _ = fmt.Fprintf(stdout, "\n%-8s %-10s %12s %12s\n", "PARAM", "LABEL", "BASELINE", "FITTED")

	best := report.Best
	if best == nil {
		best = &report.Baseline
	}

	for index, param := range report.Baseline.Params {
		fitted := best.Params[index]

		note := ""

		switch {
		case fitted.Fixed:
			note = " (fixed)"
		case fitted.Pinned:
			note = " (PINNED at the " + fitted.PinnedAt + " stop)"
		}

		_, _ = fmt.Fprintf(stdout, "%-8d %-10s %12.4g %12.4g%s\n",
			index, param.Label, param.Value, fitted.Value, note)
	}

	_, _ = fmt.Fprintf(stdout, "%-8s %-10s %12.4g %12.4g\n",
		"-", "VEL", report.Baseline.Takes[0].Velocity01, best.Takes[0].Velocity01)

	writePinned(stdout, *best)
	writeTakes(stdout, *best)

	// The first take's partials, and only the first. A joint fit measures the
	// same drum sixteen times and printing sixteen partial tables would bury the
	// bank they were all fitted from; the per-take numbers that do differ are in
	// the table above and in the report's own takes[].
	_, _ = fmt.Fprintf(stdout, "\nreference partials (Hz / dB / T60 s) — %s:\n",
		report.References[0].Path)

	for _, partial := range report.Targets[0].Partials {
		_, _ = fmt.Fprintf(stdout, "  %8.2f %7.1f %6.2f\n",
			partial.FrequencyHz, partial.LevelDB, partial.T60Seconds)
	}

	_, _ = fmt.Fprintf(stdout, "\nfitted partials (Hz / dB / T60 s):\n")
	for _, partial := range best.Takes[0].Features.Partials {
		_, _ = fmt.Fprintf(stdout, "  %8.2f %7.1f %6.2f\n",
			partial.FrequencyHz, partial.LevelDB, partial.T60Seconds)
	}

	writeTerms(stdout, report)

	_, _ = fmt.Fprintf(stdout, "\nbaseline %s\n", summarize(report.Baseline.Terms))

	if report.Best != nil {
		_, _ = fmt.Fprintf(stdout, "fitted   %s\n", summarize(report.Best.Terms))
	}
}

// writeTerms prints the nine terms against the gates they are read against.
//
// The ×gate column is the point of the table and the raw column is the caption.
// A term at its gate is at the level where a candidate stops being
// distinguishable from a second microphone on the same drum, so 1.00 is the
// target for every row and the reader needs no memory of what a decibel of
// spectral envelope error means. The column sums to the total printed on its
// last row, which is the identity GateRatios documents, and printing both makes
// that checkable rather than asserted.
func writeTerms(stdout io.Writer, report Report) {
	best := report.Best
	if best == nil {
		best = &report.Baseline
	}

	_, _ = fmt.Fprintf(stdout,
		"\nterms against their adoption gates (×gate = value / gate; the total is their plain sum):\n")
	_, _ = fmt.Fprintf(stdout, "%-12s %8s %10s %8s %10s %8s\n",
		"TERM", "GATE", "BASELINE", "xGATE", "FITTED", "xGATE")

	baseline, fitted := termFields(report.Baseline.Terms), termFields(best.Terms)

	for index, field := range fitted {
		digits := 1
		if field.Unit == "" {
			digits = 3
		}

		_, _ = fmt.Fprintf(stdout, "%s %8.4g %10.*f %8.2f %10.*f %8.2f\n",
			padRunes(field.Name+" "+field.Unit, 12), field.Gate,
			digits, baseline[index].Value, baseline[index].Ratio(),
			digits, field.Value, field.Ratio())
	}

	_, _ = fmt.Fprintf(stdout, "%-12s %8s %10.3f %8.2f %10.3f %8.2f\n",
		"total", "-",
		report.Baseline.Terms.Total, report.Baseline.TermsVsGate.Total,
		best.Terms.Total, best.TermsVsGate.Total)

	// The fingerprint travels with every total this tool prints, not only with
	// the JSON, because the comparison that goes wrong is the one made between
	// two terminal scrollbacks.
	_, _ = fmt.Fprintf(stdout, "\nweight set %s — totals from any other weight set are not comparable with these.\n",
		report.WeightsFingerprint)

	if report.ObjectiveFloor > 0 {
		_, _ = fmt.Fprintf(stdout,
			"objective floor on this reference set: %.3f (given by -floor). "+
				"A total below it is not distinguishable from the objective's own noise.\n",
			report.ObjectiveFloor)

		return
	}

	_, _ = fmt.Fprintf(stdout,
		"no -floor given, so these totals have nothing to be read against: "+
			"run cmd/measure-objective on this reference set and pass what it measures.\n")
}

// padRunes left-aligns to a width counted in runes.
//
// fmt's %-*s counts bytes, and the unit column holds ¢ — two bytes, one column
// — so the table would step left by one on exactly the two rows that use it.
func padRunes(text string, width int) string {
	if count := len([]rune(text)); count < width {
		return text + strings.Repeat(" ", width-count)
	}

	return text
}

// writePinned says, in words, that the search wanted something outside the
// shipped range.
//
// The flag is on every pinned parameter in the report already and that was not
// enough: a fit prints eighteen parameter rows and the one that matters is a
// parenthetical on one of them. The motivating case is DAMP landing at
// normalized 0.0084 on the tt08x08/lp/hd series, in two independent runs, which
// was the most actionable result either produced and which the tool did not
// mention — it was inside the stop and outside the tolerance the flag then used.
//
// A pinned parameter is not a converged one. The search stopped there because it
// ran out of range, so the number is a bound rather than a fit, and the finding
// is about the range: either the model's mapping is wrong, or the product's
// limits exclude the drum being fitted.
func writePinned(stdout io.Writer, best Candidate) {
	var pinned []ParamValue

	for _, param := range best.Params {
		if param.Pinned {
			pinned = append(pinned, param)
		}
	}

	if len(pinned) == 0 {
		return
	}

	_, _ = fmt.Fprintf(stdout,
		"\nWARNING: %d free parameter(s) pinned against a stop — a bound, not a fit:\n",
		len(pinned))

	for _, param := range pinned {
		_, _ = fmt.Fprintf(stdout, "  %-10s %s stop, normalized %.4f = %.4g %s\n",
			param.Label, param.PinnedAt, param.Normalized, param.Value, param.Unit)
	}

	_, _ = fmt.Fprintf(stdout,
		"  The search wanted to go further and the range would not let it. "+
			"Read this as evidence the shipped range is wrong, not as convergence.\n")
}

// writeTakes prints one line per take, and then says what the fitted velocities
// imply about the order the takes were given in.
//
// The order check is the point of the section. A velocity series is named v01…
// v16 by the pack that shipped it, in what it says is increasing strike order,
// and those files were played by hand — so the labelling is a claim, not data.
// Nothing in the fit uses it: each take carries its own free velocity and the
// takes never see each other. That makes the fitted velocities an independent
// read on the ordering, and the inversions counted below are how far the names
// and the drum disagree.
//
// A count is reported rather than a re-ordering. Which of the two is wrong is
// not something this tool can know — a genuinely non-monotone series and a
// mislabelled one look identical from here — and quietly renaming files to make
// a number look better is the opposite of a measurement.
func writeTakes(stdout io.Writer, best Candidate) {
	if len(best.Takes) < 2 {
		return
	}

	_, _ = fmt.Fprintf(stdout, "\n%-6s %-40s %8s %8s\n", "TAKE", "REFERENCE", "VEL", "TOTAL")

	for index, take := range best.Takes {
		_, _ = fmt.Fprintf(stdout, "%-6d %-40s %8.3f %8.3f\n",
			index+1, take.Path, take.Velocity01, take.Terms.Total)
	}

	inversions, ties := 0, 0

	for i := 1; i < len(best.Takes); i++ {
		switch {
		case best.Takes[i].Velocity01 < best.Takes[i-1].Velocity01:
			inversions++
		case best.Takes[i].Velocity01 == best.Takes[i-1].Velocity01:
			ties++
		}
	}

	// Every take at one velocity is a baseline, not a fit — position broadcasts
	// a single velocity across the series — and calling that monotone would be
	// reading an ordering out of a constant.
	if ties == len(best.Takes)-1 {
		_, _ = fmt.Fprintf(stdout,
			"\nevery take is struck at the same velocity, so this says nothing about the order.\n")

		return
	}

	if inversions == 0 {
		_, _ = fmt.Fprintf(stdout,
			"\nfitted velocities rise monotonically across the %d takes as listed.\n",
			len(best.Takes))

		return
	}

	_, _ = fmt.Fprintf(stdout,
		"\nfitted velocities fall at %d of %d steps in the order listed — "+
			"the series is not monotone in strike strength, or is not labelled in that order.\n",
		inversions, len(best.Takes)-1)
}

// ensureDir creates the directory a run's artifacts are about to be written to.
// The defaults put reports and checkpoints under fits/ and WAVs under renders/,
// neither of which is committed (both are gitignored), so on a fresh clone the
// first run would otherwise fail on a missing directory rather than on anything
// the user did wrong.
func ensureDir(path string) error {
	directory := filepath.Dir(path)
	if directory == "" || directory == "." {
		return nil
	}

	return os.MkdirAll(directory, 0o755)
}

func writeReport(path string, report Report) error {
	if path == "-" {
		return nil
	}

	if err := ensureDir(path); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(report); err != nil {
		return err
	}

	return file.Close()
}
