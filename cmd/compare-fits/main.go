// Command compare-fits asks of the *search* the question cmd/measure-objective
// asks of the objective: does it agree with itself?
//
// measure-objective scores one observation of a drum against a second
// observation of the same drum and calls the disagreement the floor. Nothing did
// the same for the Mayfly search, and the search has the larger opportunity to
// disagree with itself: it is stochastic, it is stopped by hand, and its report
// prints eighteen parameters and sixteen per-take velocities to sixteen digits
// whether or not the objective determines any of them. Two runs over the same
// sixteen takes — 5,002 and 8,976 evaluations, totals 15.835 and 15.186 — were
// once compared by hand-parsing their checkpoints in a throwaway script, and
// that comparison produced the most important result of the session: the per-take
// fitted velocities correlated rho = +0.15 between the runs. A quantity two runs
// of the same search cannot agree on is not a measurement of the strike, and
// nothing may be read off it. A finding of that weight cannot live in a script
// that no longer exists.
//
//	go run ./cmd/compare-fits fits/fit-A-series.json fits/fit-B-series.json
//
// What it prints, and why each part is here:
//
//   - Per-term deltas, each shown raw and divided by its adoption gate. The
//     ratio is not decoration: weight = 1/gate, so a term's value over its gate
//     is exactly its additive contribution to the total, and a term below 1.0 is
//     inside the objective's measured disagreement with itself. A term that moved
//     between runs while staying under 1.0 has not moved in any sense that means
//     anything.
//   - Per-parameter deltas, flagging any *free* parameter that sits within 1 % of
//     a stop. physicalTom.damping landed at normalized 0.0084 in both runs of the
//     motivating session: a free parameter pinned to its stop says the shipped
//     range excludes the value the drum wants, and two runs agreeing on it turns
//     an accident into a result.
//   - Rank correlation of the per-take fitted velocities, which is the headline.
//     Also each run's velocities against the take index, because the vNN
//     labelling is a claim about how hard the takes were struck and correlating
//     against it is a test of that claim rather than an assumption of it.
//
// Rank rather than linear correlation throughout — see internal/physical/series,
// which owns that argument.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"text/tabwriter"

	"github.com/cwbudde/algo-drum/internal/physical/match"
	"github.com/cwbudde/algo-drum/internal/physical/series"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "compare-fits: %v\n", err)

		os.Exit(1)
	}
}

// errCompare reports a comparison this tool refuses to make.
var errCompare = errors.New("incomparable reports")

// report is a deliberately lenient view of a cmd/fit-physical -o report: it
// declares only the fields this comparison reads, and encoding/json discards the
// rest.
//
// The alternative — importing fit-physical's own report type — is not available
// (it is package main) and would be the wrong dependency even if it were: this
// tool would then fail to build every time the report grew a field, and would
// have to be revised in lockstep with a tool it only observes. tools/paper-figures
// declares its own structs over the same reports for the same reason. The cost is
// that a renamed field goes silently missing rather than failing to compile,
// which is why every field read here is also printed: a zeroed column is visible
// in the output in a way a dropped field never is.
//
// Terms and Weights are the shipped match types rather than local copies. Those
// are not report schema — they are the objective itself, and a local restatement
// of them could drift from the thing being compared.
type report struct {
	// path is where this was read from. Not a JSON field; it labels the columns.
	path string

	References []reference   `json:"references"`
	Weights    match.Weights `json:"weights"`
	Search     search        `json:"search"`
	Baseline   *stage        `json:"baseline"`
	Best       *stage        `json:"best"`
}

type reference struct {
	Path string `json:"path"`
}

type search struct {
	Variant     string `json:"variant"`
	Iterations  int    `json:"iterations"`
	Population  int    `json:"population"`
	Restarts    int    `json:"restarts"`
	Seed        int64  `json:"seed"`
	Evaluations int    `json:"evaluations"`
	Interrupted bool   `json:"interrupted"`
}

// stage is one scored point — the shipped defaults (baseline) or the fit (best).
type stage struct {
	Terms  match.Terms `json:"terms"`
	Params []param     `json:"params"`
	Takes  []take      `json:"takes"`
}

type param struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Normalized is the search's own 0-1 position, which is the only scale on
	// which "against a stop" means anything; Value is the same point in metres,
	// N/m or degrees, which is the only scale a person can judge.
	Normalized float64 `json:"normalized"`
	Value      float64 `json:"value"`
	Unit       string  `json:"unit"`
	Fixed      bool    `json:"fixed"`
}

type take struct {
	Path       string      `json:"path"`
	Velocity01 float64     `json:"velocity01"`
	Terms      match.Terms `json:"terms"`
}

// stopTolerance is how close to 0 or 1 a free parameter has to sit before it is
// called pinned. One percent of the range: wide enough that a genuine optimum
// near the edge is still flagged (which is the point — the flag asks a question
// about the range, it does not accuse the search), narrow enough that an interior
// optimum never trips it.
const stopTolerance = 0.01

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("compare-fits", flag.ContinueOnError)
	flags.SetOutput(stderr)

	// The escape hatch exists because "these totals are not comparable" is
	// occasionally exactly what one wants to look at — a re-gating is judged by
	// seeing the old and new totals side by side. It is a flag rather than the
	// default because the failure it guards is silent: mismatched reports print
	// the same nine rows in the same nine columns and read as a result.
	allowIncomparable := flags.Bool("allow-incomparable", false,
		"compare reports scored under different weights or against different "+
			"references; the totals are then not a comparison of two fits but of two objectives")

	if err := flags.Parse(args); err != nil {
		return err
	}

	paths := flags.Args()
	if len(paths) < 2 {
		return fmt.Errorf(
			"need at least two fit reports: compare-fits fits/a.json fits/b.json",
		)
	}

	reports := make([]*report, 0, len(paths))

	for _, path := range paths {
		loaded, err := load(path)
		if err != nil {
			return err
		}

		reports = append(reports, loaded)
	}

	if err := comparable(reports, stderr, *allowIncomparable); err != nil {
		return err
	}

	printHeader(stdout, reports)
	printTerms(stdout, reports)
	printParams(stdout, reports)

	return printVelocities(stdout, reports)
}

func load(path string) (*report, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	loaded := &report{path: path}
	if err := json.Unmarshal(raw, loaded); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	if loaded.Best == nil {
		return nil, fmt.Errorf("%s: no best point: an unfinished run has nothing to compare", path)
	}

	if len(loaded.Best.Takes) == 0 {
		return nil, fmt.Errorf("%s: no per-take results", path)
	}

	return loaded, nil
}

// comparable refuses the comparisons that cannot mean anything.
//
// Two refusals and one warning, and the split is deliberate:
//
//   - Different weights: refuse. A total is a property of a weight set. Tightening
//     one gate raises its weight, so the *same* raw disagreement scores higher, and
//     a run under tighter gates looks worse than an identical run under looser
//     ones. Quoting two such totals against each other has already caused a wrong
//     conclusion in this repository, which is why this is the one check that gets a
//     hard stop rather than a printed caveat.
//   - Different references: refuse. The per-take pairing is by file path, and the
//     partial tables, gates and totals are properties of one drum at one tuning.
//     The check is over the *set* of paths and not their order, on purpose: the
//     take order is a claim rather than a measurement, so making the comparison
//     depend on it would build the claim into the tool.
//   - Different baseline totals: warn. With the same references and weights the
//     baseline is the shipped default bank measured the same way twice, so it is
//     deterministic and a difference means the extraction options differed. That is
//     disqualifying for the per-term table but not for the velocity correlation,
//     which is what this tool is mostly for — so it is loud rather than fatal.
func comparable(reports []*report, stderr io.Writer, allow bool) error {
	first := reports[0]

	for _, other := range reports[1:] {
		if first.Weights != other.Weights {
			if !allow {
				return fmt.Errorf(
					"%w: %s and %s were scored under different weights, so their totals "+
						"measure different objectives and not different fits; pass "+
						"-allow-incomparable to print them anyway",
					errCompare, first.path, other.path,
				)
			}

			_, _ = fmt.Fprintf(stderr, "WARNING: %s and %s use different weights; "+
				"every total and every gate ratio below is on its own scale\n", first.path, other.path)
		}

		if !sameReferences(first, other) {
			if !allow {
				return fmt.Errorf(
					"%w: %s and %s targeted different references, so nothing below pairs up; "+
						"pass -allow-incomparable to print them anyway",
					errCompare, first.path, other.path,
				)
			}

			_, _ = fmt.Fprintf(stderr, "WARNING: %s and %s targeted different references\n",
				first.path, other.path)
		}

		if first.Baseline != nil && other.Baseline != nil &&
			first.Baseline.Terms.Total != other.Baseline.Terms.Total {
			_, _ = fmt.Fprintf(stderr,
				"WARNING: baseline totals differ (%.6f vs %.6f). Same references under the "+
					"same weights measure the shipped defaults identically, so the extraction "+
					"options differed between these runs and the per-term table below compares "+
					"two measurements rather than two fits\n",
				first.Baseline.Terms.Total, other.Baseline.Terms.Total)
		}
	}

	return nil
}

func sameReferences(first, second *report) bool {
	left := make([]string, 0, len(first.References))
	for _, item := range first.References {
		left = append(left, item.Path)
	}

	right := make([]string, 0, len(second.References))
	for _, item := range second.References {
		right = append(right, item.Path)
	}

	slices.Sort(left)
	slices.Sort(right)

	return slices.Equal(left, right)
}

func printHeader(out io.Writer, reports []*report) {
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	_, _ = fmt.Fprintln(writer, "report\ttakes\tvariant\tit\tpop\trestarts\tseed\tevals\tstate\ttotal")

	for _, item := range reports {
		state := "finished"
		if item.Search.Interrupted {
			state = "interrupted"
		}

		_, _ = fmt.Fprintf(writer, "%s\t%d\t%s\t%d\t%d\t%d\t%d\t%d\t%s\t%.3f\n",
			item.path, len(item.Best.Takes), item.Search.Variant, item.Search.Iterations,
			item.Search.Population, item.Search.Restarts, item.Search.Seed,
			item.Search.Evaluations, state, item.Best.Terms.Total)
	}

	_ = writer.Flush()
}

// termOf names the nine terms once, exactly as cmd/measure-objective does, so
// that this table and that one cannot fall out of step.
type termOf struct {
	name string
	unit string
	of   func(match.Terms) float64
	gate func(match.Weights) float64
}

func terms() []termOf {
	return []termOf{
		{
			"partial frequency", "cents", func(t match.Terms) float64 { return t.PartialFrequency },
			func(w match.Weights) float64 { return w.PartialFrequency },
		},
		{
			"partial level", "dB", func(t match.Terms) float64 { return t.PartialLevel },
			func(w match.Weights) float64 { return w.PartialLevel },
		},
		{
			"partial decay", "logratio", func(t match.Terms) float64 { return t.PartialDecay },
			func(w match.Weights) float64 { return w.PartialDecay },
		},
		{
			"spectral envelope", "dB", func(t match.Terms) float64 { return t.SpectralEnvelope },
			func(w match.Weights) float64 { return w.SpectralEnvelope },
		},
		{
			"envelope", "dB", func(t match.Terms) float64 { return t.Envelope },
			func(w match.Weights) float64 { return w.Envelope },
		},
		{
			"glide", "cents", func(t match.Terms) float64 { return t.Glide },
			func(w match.Weights) float64 { return w.Glide },
		},
		{
			"attack balance", "dB", func(t match.Terms) float64 { return t.AttackBalance },
			func(w match.Weights) float64 { return w.AttackBalance },
		},
		{
			"unmatched share", "", func(t match.Terms) float64 { return t.Unmatched },
			func(w match.Weights) float64 { return w.Unmatched },
		},
		{
			"spurious share", "", func(t match.Terms) float64 { return t.Spurious },
			func(w match.Weights) float64 { return w.Spurious },
		},
	}
}

// printTerms is the per-term table: each report's raw value in its own unit, and
// the same value over its adoption gate.
//
// The gate column is what makes the table readable. The raw units are not
// commensurable — 76 cents of partial frequency error and 0.62 of decay log ratio
// cannot be compared as numbers — while the ratios are, by construction, and they
// sum to the total. A row where both ratios are under 1.0 is a row where the two
// runs disagree by less than one observation of the drum disagrees with another,
// and the delta on it is noise regardless of how large it looks raw.
func printTerms(out io.Writer, reports []*report) {
	gates := match.AdoptionGates()

	_, _ = fmt.Fprintln(out, "\nper-term, raw and over the adoption gate (a term at its gate contributes 1.0):")

	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	_, _ = fmt.Fprint(writer, "term\tunit\tgate")

	for i := range reports {
		_, _ = fmt.Fprintf(writer, "\traw%d\txgate%d", i+1, i+1)
	}

	_, _ = fmt.Fprintf(writer, "\t%s/gate\tnote\n", spreadLabel(len(reports)))

	for _, definition := range terms() {
		gate := definition.gate(gates)

		_, _ = fmt.Fprintf(writer, "%s\t%s\t%.3f", definition.name, definition.unit, gate)

		ratios := make([]float64, 0, len(reports))

		for _, item := range reports {
			value := definition.of(item.Best.Terms)
			ratio := value / gate
			ratios = append(ratios, ratio)

			_, _ = fmt.Fprintf(writer, "\t%.3f\t%.3f", value, ratio)
		}

		note := ""
		if slices.Max(ratios) < 1 {
			note = "inside the objective's own noise"
		}

		_, _ = fmt.Fprintf(writer, "\t%+.3f\t%s\n", spread(ratios), note)
	}

	_, _ = fmt.Fprint(writer, "total\t\t")

	for _, item := range reports {
		_, _ = fmt.Fprintf(writer, "\t\t%.3f", item.Best.Terms.Total)
	}

	_, _ = fmt.Fprint(writer, "\t\t\n")

	_ = writer.Flush()
}

// printParams is the per-parameter table, keyed by ID so that reports whose
// parameter lists differ in order or membership (a different -fix set) still line
// up. Order follows the first report, which is the order fit-physical prints.
func printParams(out io.Writer, reports []*report) {
	_, _ = fmt.Fprintln(out, "\nper-parameter, normalized position (* = fixed, PINNED = free but at a stop):")

	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	_, _ = fmt.Fprint(writer, "parameter\tlabel\tunit")

	for i := range reports {
		_, _ = fmt.Fprintf(writer, "\tnorm%d\tvalue%d", i+1, i+1)
	}

	_, _ = fmt.Fprintf(writer, "\t%s\tnote\n", spreadLabel(len(reports)))

	for _, first := range reports[0].Best.Params {
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s", first.ID, first.Label, first.Unit)

		positions := make([]float64, 0, len(reports))
		pinned, everywhere, free := false, true, false

		for _, item := range reports {
			found, ok := lookup(item.Best.Params, first.ID)
			if !ok {
				_, _ = fmt.Fprint(writer, "\t-\t-")

				everywhere = false

				continue
			}

			positions = append(positions, found.Normalized)

			marker := ""

			if found.Fixed {
				marker = "*"
			} else {
				free = true

				if atStop(found.Normalized) {
					pinned = true
				}
			}

			_, _ = fmt.Fprintf(writer, "\t%.4f%s\t%.4g", found.Normalized, marker, found.Value)
		}

		note := ""

		switch {
		case !everywhere:
			note = "absent from a report"
		case pinned && free:
			// Both halves matter. That a free parameter reached its stop says the
			// range is wrong; that every run reached the same stop says the range
			// is wrong reproducibly, which is a result about the model rather than
			// a quirk of one search.
			note = "PINNED: free, at a stop — the shipped range excludes the optimum"
		}

		_, _ = fmt.Fprintf(writer, "\t%+.4f\t%s\n", spread(positions), note)
	}

	_ = writer.Flush()
}

// spread is signed for two reports and a range for more.
//
// With two reports "the second is 0.18 gates worse than the first" is the
// question being asked, and a magnitude would throw away the direction. With
// three or more there is no direction to report, so the column becomes the range
// — how far apart the runs landed — which is the quantity that still means
// something when there is no privileged pair.
func spread(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	if len(values) == 2 {
		return values[1] - values[0]
	}

	return slices.Max(values) - slices.Min(values)
}

func spreadLabel(reports int) string {
	if reports == 2 {
		return "delta"
	}

	return "range"
}

func lookup(params []param, id string) (param, bool) {
	for _, item := range params {
		if item.ID == id {
			return item, true
		}
	}

	return param{}, false
}

func atStop(normalized float64) bool {
	return normalized <= stopTolerance || normalized >= 1-stopTolerance
}

// printVelocities is the headline check.
//
// The per-take strike velocity is a nuisance parameter: it exists because a joint
// fit cannot assume how hard each take was struck, not because anybody wants to
// know. Two independent searches over the same recordings, both converging to
// comparable totals, should nonetheless recover the same *ordering* of strike
// strengths if the objective determines it at all. When they do not — the
// motivating session measured rho = +0.15 over sixteen takes — the objective is
// indifferent to that dimension and the search fills it with whatever the swarm
// happened to be holding. Nothing may then be read off the velocities: not "take
// 13 was the hardest hit", not a velocity-to-loudness curve, and not a claim that
// the vNN order is a ramp.
//
// The correlation against the take index tests the file labelling itself, and is
// reported per run rather than pooled: it is evidence about the recording session,
// and pooling it across runs would hide the case where one run sees a trend and
// the other does not.
func printVelocities(out io.Writer, reports []*report) error {
	order := make([]string, 0, len(reports[0].Best.Takes))
	for _, item := range reports[0].Best.Takes {
		order = append(order, item.Path)
	}

	velocities := make([][]float64, len(reports))

	for i, item := range reports {
		byPath := make(map[string]float64, len(item.Best.Takes))
		for _, entry := range item.Best.Takes {
			byPath[entry.Path] = entry.Velocity01
		}

		velocities[i] = make([]float64, 0, len(order))

		for _, path := range order {
			value, ok := byPath[path]
			if !ok {
				return fmt.Errorf("%w: %s has no take for %s", errCompare, item.path, path)
			}

			velocities[i] = append(velocities[i], value)
		}
	}

	_, _ = fmt.Fprintln(out, "\nper-take fitted velocity (0-1), in the first report's take order:")

	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	_, _ = fmt.Fprint(writer, "take")

	for i := range reports {
		_, _ = fmt.Fprintf(writer, "\tvel%d", i+1)
	}

	_, _ = fmt.Fprintln(writer)

	for index, path := range order {
		_, _ = fmt.Fprintf(writer, "%s", path)

		for i := range reports {
			_, _ = fmt.Fprintf(writer, "\t%.4f", velocities[i][index])
		}

		_, _ = fmt.Fprintln(writer)
	}

	_ = writer.Flush()

	_, _ = fmt.Fprintln(out, "\nvelocity agreement (Spearman rank correlation):")

	for i := range reports {
		for j := i + 1; j < len(reports); j++ {
			rho, err := series.Spearman(velocities[i], velocities[j])
			_, _ = fmt.Fprintf(out, "  report %d vs report %d:  %s\n", i+1, j+1, format(rho, err))
		}
	}

	_, _ = fmt.Fprintln(out, "\nvelocity against take index (is the vNN order a strike ramp?):")

	indices := series.Indices(len(order))

	for i := range reports {
		rho, err := series.Spearman(velocities[i], indices)
		_, _ = fmt.Fprintf(out, "  report %d:  %s\n", i+1, format(rho, err))
	}

	printVelocityVerdict(out, reports, velocities)

	return nil
}

// agreementThreshold is where this tool stops calling the velocities a
// measurement. It is a reading aid and not a gate: there is no measured
// reproducibility floor for a search the way measure-objective provides one for
// the objective, so the number below is a judgement — under half the variance in
// common (rho^2 < 0.25) is not an agreement anyone should build on. Report the
// rho; the verdict only saves the reader from computing its square.
const agreementThreshold = 0.5

func printVelocityVerdict(out io.Writer, reports []*report, velocities [][]float64) {
	worst, measured := math.Inf(1), false

	for i := range reports {
		for j := i + 1; j < len(reports); j++ {
			rho, err := series.Spearman(velocities[i], velocities[j])
			if err != nil {
				continue
			}

			worst, measured = math.Min(worst, rho), true
		}
	}

	if !measured {
		return
	}

	if worst < agreementThreshold {
		_, _ = fmt.Fprintf(out,
			"\nVERDICT: the runs' per-take velocities correlate at rho = %+.2f, which is not agreement.\n"+
				"The velocity is then a nuisance parameter the objective does not identify: the\n"+
				"search is filling those dimensions with noise, and nothing may be read off them\n"+
				"— not the ordering of the takes, not a velocity curve, not the vNN labelling.\n",
			worst)

		return
	}

	_, _ = fmt.Fprintf(out,
		"\nVERDICT: the runs' per-take velocities correlate at rho = %+.2f. The objective\n"+
			"identifies the strike ordering; the velocities are evidence, within that rho.\n",
		worst)
}

func format(rho float64, err error) string {
	if err != nil {
		return "n/a: " + err.Error()
	}

	return fmt.Sprintf("rho = %+.3f", rho)
}
