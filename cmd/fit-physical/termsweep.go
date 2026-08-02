package main

// The per-term half of the step sweep: PLAN.md N20 step 1, "which term steps".
//
// N6 measured that this objective is piecewise-constant at the scale its own
// parameters live at — a second difference at h = 1e-4 normalized is measuring a
// partial entering or leaving the matched set, not curvature, and no component of
// the 5x5 angle/radius block produced a plateau anywhere in the swept grid. That
// is a statement about the *total*, and the total is nine terms in a trench coat.
// The nine do not quantize alike: unmatched and spurious are shares of a matched
// set and can only move when a partial crosses the matching threshold, while the
// spectral envelope involves no matching at all. So the total's staircase may be
// one term's or every term's, and N20's first question is which.
//
// The measurement is free. match.Distance returns all nine terms plus the total
// on every call, and evaluator.cost keeps one of them; so decomposing the sweep
// re-uses the same evaluations rather than adding any. Nothing here smooths
// anything, changes any weight, or moves any committed total — N20's steps 2 and
// 3 are separate questions and the item says in terms not to touch them first.

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/cwbudde/algo-drum/internal/physical/match"
)

// termCount is the number of distance terms match.Terms carries beside its total.
const termCount = 9

// termTotalSlot is where the total sits in a termVector: after the nine.
const termTotalSlot = termCount

// termVector is one evaluation of the objective, decomposed.
//
// Slots 0..8 are the nine terms in termFields order, each **divided by its
// adoption gate**, because that is the only unit the nine are comparable in — 14
// dB of spectral envelope error and 25 cents of glide error are the same-looking
// numbers and a factor of five apart in what they cost. Slot 9 is match.Terms's
// own Total, copied rather than re-summed from the nine.
//
// Copied rather than re-summed on purpose. The nine ratios do sum to the total,
// weight being 1/gate, but they sum in a different order than match.Distance adds
// them, so the two agree to floating point and not to the bit. This sweep's total
// column has to be the *same* float64 the scalar sweep would have produced, since
// that is what makes it a decomposition rather than a second measurement, so the
// total is taken from where the scalar sweep takes it.
type termVector [termCount + 1]float64

// newTermVector decomposes one distance into the vector the sweep differences.
func newTermVector(terms match.Terms) termVector {
	var vector termVector

	for slot, field := range termFields(terms) {
		vector[slot] = field.Ratio()
	}

	vector[termTotalSlot] = terms.Total

	return vector
}

// termSlotLabels names the ten slots, in their order.
//
// Read off termFields rather than written out again, so the labels cannot drift
// from the order newTermVector fills — the two would otherwise be a pair of lists
// that have to be kept in step by hand, and this file would mislabel every column
// the day a term is added.
func termSlotLabels() []string {
	fields := termFields(match.Terms{})
	labels := make([]string, 0, len(fields)+1)

	for _, field := range fields {
		labels = append(labels, field.Name)
	}

	return append(labels, "total")
}

// TermStepSweep is one component's step sweep, decomposed by term.
//
// Terms holds one StepSweep per slot of a termVector, labelled with the term
// rather than with the component — the component is the Label here — so each
// entry can be read with exactly the plateau vocabulary the scalar sweep uses.
type TermStepSweep struct {
	Label string `json:"label"`
	// Decomposed is false when the objective could not supply the nine terms, in
	// which case Terms holds the total alone. That is the case for the synthetic
	// scalar objectives the unit tests differentiate, and it is recorded rather
	// than inferred from the length.
	Decomposed bool        `json:"decomposed"`
	Terms      []StepSweep `json:"terms"`
}

// sweepTermSteps turns one component's per-step term vectors into per-term
// sweeps, and runs each through the same plateau test the scalar sweep uses.
//
// samples[step][slot], with a nil row for a step that could not be measured at
// all — a stencil point that was not a drum, or one that would have crossed a
// bound — so that a term is never credited with a sample its component did not
// have. The note for such a step is shared across all ten terms because the
// reason is: a rejected configuration is rejected for every term at once.
func sweepTermSteps(
	label string,
	decomposed bool,
	samples [][]float64,
	notes []string,
) TermStepSweep {
	labels := termSlotLabels()

	slots := len(labels)
	if !decomposed {
		slots = 1
	}

	sweep := TermStepSweep{Label: label, Decomposed: decomposed, Terms: make([]StepSweep, 0, slots)}

	for offset := range slots {
		slot := offset
		if !decomposed {
			slot = termTotalSlot
		}

		perTerm := StepSweep{Label: labels[slot]}

		for index, step := range hessianSteps {
			sample := StepSample{Step: step, Note: notes[index]}

			if samples[index] != nil {
				curvature := samples[index][slot]
				sample.Curvature = &curvature

				numerator := curvature * step * step
				sample.Numerator = &numerator

				if curvature == 0 {
					sample.Note = detentNote
				}
			}

			perTerm.Samples = append(perTerm.Samples, sample)
		}

		perTerm.PlateauFrom, perTerm.PlateauTo, perTerm.Available, perTerm.Note = findPlateau(perTerm.Samples)

		sweep.Terms = append(sweep.Terms, perTerm)
	}

	return sweep
}

// writeTermSweeps prints the per-term block: one row per term per component.
//
// Wide on purpose. The question this table answers is which of the nine terms is
// responsible for the total's staircase, and that is a comparison *across* the
// nine at one component — so the nine sit together, under the component, and the
// total sits with them as the row they have to explain.
func writeTermSweeps(out io.Writer, sweeps []TermStepSweep) {
	if len(sweeps) == 0 {
		return
	}

	_, _ = fmt.Fprintf(out, "\nper-term step sweep (each term over its adoption gate; "+
		"the total is the scalar sweep above)\n")

	header := make([]string, 0, len(hessianSteps))
	for _, step := range hessianSteps {
		header = append(header, fmt.Sprintf("%12g", step))
	}

	_, _ = fmt.Fprintf(out, "  %-10s%s\n", "h →", strings.Join(header, ""))

	for _, sweep := range sweeps {
		_, _ = fmt.Fprintf(out, "  %s\n", sweep.Label)

		for _, perTerm := range sweep.Terms {
			_, _ = fmt.Fprintf(out, "    %-8s%s | %s\n",
				perTerm.Label, formatSamples(perTerm.Samples), perTerm.Note)
		}
	}
}

// termVerdict is the one-line reading of a per-term sweep: which terms, if any,
// produced a plateau at any component.
//
// Stated as a count rather than as a conclusion. "Four of nine terms plateau
// somewhere" is a fact; "the staircase is in the matching terms" is a reading of
// it, and belongs in the document a person writes after looking, not in the tool.
func termVerdict(sweeps []TermStepSweep) string {
	if len(sweeps) == 0 {
		return ""
	}

	available := map[string]int{}
	labels := []string{}

	for _, sweep := range sweeps {
		for _, perTerm := range sweep.Terms {
			if !slices.Contains(labels, perTerm.Label) {
				labels = append(labels, perTerm.Label)
			}

			if perTerm.Available {
				available[perTerm.Label]++
			}
		}
	}

	plateaued := make([]string, 0, len(labels))

	for _, label := range labels {
		if available[label] > 0 {
			plateaued = append(plateaued,
				fmt.Sprintf("%s (%d/%d)", label, available[label], len(sweeps)))
		}
	}

	if len(plateaued) == 0 {
		return fmt.Sprintf("no term produced a plateau at any of the %d components: "+
			"the staircase is in every term, not in one of them", len(sweeps))
	}

	return fmt.Sprintf("%d of %d terms produced a plateau at one or more of the %d "+
		"components: %s", len(plateaued), len(labels), len(sweeps),
		strings.Join(plateaued, ", "))
}
