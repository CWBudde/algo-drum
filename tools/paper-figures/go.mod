// Separate module on purpose.
//
// Rendering a PNG pulls matplotlib-go and its graphics dependency tree, and the
// AGG backend links FreeType through cgo unless the `purego` tag selects its
// pure-Go rasteriser. Neither belongs in the engine's module: `go mod tidy`
// ignores custom build tags, so a purego-only package inside the main module
// makes go.mod oscillate, and requiring FreeType headers to run
// `go build ./...` would be a poor trade for a command that regenerates four
// committed images.
module github.com/cwbudde/algo-drum/tools/paper-figures

go 1.25.0

require github.com/cwbudde/matplotlib-go v0.3.2

require (
	codeberg.org/go-fonts/dejavu v0.4.0 // indirect
	github.com/cwbudde/agg_go v0.4.0 // indirect
	github.com/cwbudde/algo-fft v0.6.11 // indirect
	github.com/cwbudde/mathtext v0.5.3 // indirect
	github.com/cwbudde/qhull-go v0.1.0 // indirect
	golang.org/x/image v0.42.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
)
