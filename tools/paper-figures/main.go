//go:build purego

// Command paper-figures renders the figures in docs/paper from a fit report.
//
// The report written by cmd/fit-physical carries the reference and candidate
// feature sets in full, so every figure here is reproducible from a committed
// artefact — the recording itself is not redistributable, and a figure pipeline
// that needed it could not be re-run by anyone else.
//
//	just paper-figures fit-v4-hertzian.json
//
// The comb figure is the one exception and is not produced here: it is measured
// from the two channels of the recording directly.
//
// Built only under the `purego` tag. Saving a PNG needs matplotlib-go's agg
// backend, which links FreeType through cgo unless that tag selects its pure-Go
// rasteriser instead — and requiring FreeType headers to run `go build ./...`
// would be a poor trade for a command that regenerates four committed images.
// `just lint` passes the tag so this package stays linted.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/cwbudde/matplotlib-go/backends/agg"
	"github.com/cwbudde/matplotlib-go/render"
)

// The paper's palette, matching docs/paper/style.typ so the figures sit in the
// text rather than beside it.
var (
	muted  = render.Color{R: 0.35, G: 0.39, B: 0.43, A: 1}
	accent = render.Color{R: 0.00, G: 0.44, B: 0.48, A: 1}
	warm   = render.Color{R: 0.71, G: 0.33, B: 0.12, A: 1}
	rule   = render.Color{R: 0.79, G: 0.82, B: 0.84, A: 1}
	white  = render.Color{R: 1, G: 1, B: 1, A: 1}
)

func fade(c render.Color, alpha float64) render.Color {
	c.A = alpha

	return c
}

func main() {
	reportPath := flag.String("report", "", "fit report JSON written by cmd/fit-physical -o")
	modelPath := flag.String(
		"model-data",
		"",
		"model artefact JSON written by cmd/analyze-physical -paper-data",
	)
	outDir := flag.String("o", "docs/paper/figures", "directory to write the PNGs into")
	// A second fit of the same drum is a second set of figures, not a replacement
	// for the first: the paper reports each run where it was made and does not
	// re-render a chapter's figures from a run that chapter does not describe.
	suffix := flag.String("suffix", "", "insert before .png, so terms.png becomes terms<suffix>.png")
	only := flag.String("only", "", "comma-separated figure basenames to draw (default: all)")

	flag.Parse()

	// Either input alone is a complete job: the fit figures and the model figures
	// answer different questions and are regenerated on different occasions — the
	// former when a fit is re-run, the latter when the model changes.
	if *reportPath == "" && *modelPath == "" {
		fmt.Fprintln(os.Stderr,
			"usage: paper-figures [-report <fit.json>] [-model-data <model.json>] "+
				"[-o docs/paper/figures]")
		os.Exit(2)
	}

	if err := run(*reportPath, *modelPath, *outDir, *suffix, selection(*only)); err != nil {
		fmt.Fprintln(os.Stderr, "paper-figures:", err)
		os.Exit(1)
	}
}

// selection turns -only into a membership test. A nil set means everything,
// which is what an absent flag has to mean for the recipes that predate it.
func selection(only string) map[string]bool {
	if only == "" {
		return nil
	}

	chosen := map[string]bool{}

	for _, name := range strings.Split(only, ",") {
		if name = strings.TrimSpace(name); name != "" {
			chosen[strings.TrimSuffix(name, ".png")] = true
		}
	}

	return chosen
}

func run(reportPath, modelPath, outDir, suffix string, only map[string]bool) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// A figure's basename is its identity — -only names it, -suffix distinguishes
	// the run it was drawn from — so the two are applied at the same point.
	path := func(name string) (string, bool) {
		if only != nil && !only[name] {
			return "", false
		}

		return filepath.Join(outDir, name+suffix+".png"), true
	}

	written := 0

	if reportPath != "" {
		parsed, err := loadReport(reportPath)
		if err != nil {
			return err
		}

		figures := []struct {
			name string
			draw func(*report, string) error
		}{
			{"partials", drawPartials},
			{"terms", drawTerms},
			{"decay", drawDecay},
			{"bands", drawBands},
		}

		drawn := 0

		for _, figure := range figures {
			out, wanted := path(figure.name)
			if !wanted {
				continue
			}

			if err := figure.draw(parsed, out); err != nil {
				return fmt.Errorf("%s: %w", figure.name, err)
			}

			drawn++
		}

		written += drawn

		fmt.Printf("wrote %d fit figures from %s (total %.3f)\n",
			drawn, reportPath, parsed.Best.Terms.Total)
	}

	if modelPath != "" {
		parsed, err := loadModelData(modelPath)
		if err != nil {
			return err
		}

		figures := []struct {
			name string
			draw func(*modelData, string) error
		}{
			{"modes", drawModes},
			{"loss", drawLoss},
			{"radiation", drawRadiation},
			{"cavity", drawCavity},
			{"bandwidth", drawBandwidth},
		}

		drawn := 0

		for _, figure := range figures {
			out, wanted := path(figure.name)
			if !wanted {
				continue
			}

			if err := figure.draw(parsed, out); err != nil {
				return fmt.Errorf("%s: %w", figure.name, err)
			}

			drawn++
		}

		written += drawn

		fmt.Printf("wrote %d model figures from %s (%d modes)\n",
			drawn, modelPath, len(parsed.Modes))
	}

	fmt.Printf("%d figures in %s\n", written, outDir)

	return nil
}

// renderColor keeps the figure code readable without importing render there.
type renderColor = render.Color
