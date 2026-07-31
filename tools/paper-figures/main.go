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
	outDir := flag.String("o", "docs/paper/figures", "directory to write the PNGs into")

	flag.Parse()

	if *reportPath == "" {
		fmt.Fprintln(os.Stderr, "usage: paper-figures -report <fit.json> [-o docs/paper/figures]")
		os.Exit(2)
	}

	if err := run(*reportPath, *outDir); err != nil {
		fmt.Fprintln(os.Stderr, "paper-figures:", err)
		os.Exit(1)
	}
}

func run(reportPath, outDir string) error {
	parsed, err := loadReport(reportPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	figures := []struct {
		name string
		draw func(*report, string) error
	}{
		{"partials.png", drawPartials},
		{"terms.png", drawTerms},
		{"decay.png", drawDecay},
		{"bands.png", drawBands},
	}

	for _, figure := range figures {
		if err := figure.draw(parsed, filepath.Join(outDir, figure.name)); err != nil {
			return fmt.Errorf("%s: %w", figure.name, err)
		}
	}

	fmt.Printf("wrote %d figures to %s from %s (total %.3f)\n",
		len(figures), outDir, reportPath, parsed.Best.Terms.Total)

	return nil
}

// renderColor keeps the figure code readable without importing render there.
type renderColor = render.Color
