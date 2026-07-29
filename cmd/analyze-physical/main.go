// Command analyze-physical emits deterministic calibration metrics for the
// experimental physical drum without involving the browser audio pipeline.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/cwbudde/algo-drum/internal/physical"
	physicalanalysis "github.com/cwbudde/algo-drum/internal/physical/analysis"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "analyze-physical: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("analyze-physical", flag.ContinueOnError)
	flags.SetOutput(stderr)

	outputPath := flags.String("o", "-", "JSON output path, or - for stdout")
	suite := flags.Bool("suite", false, "generate the committed multi-condition reference suite")
	duration := flags.Float64("duration", 2, "render duration in seconds")
	velocity := flags.Float64("velocity", 0.8, "normalized strike velocity [0,1]")
	strikeRadius := flags.Float64("strike-radius", 0.45, "normalized strike radius [0,1]")
	pickupRadius := flags.Float64("pickup-radius", 0.32, "normalized microphone projection radius [0,1]")
	pickupAngle := flags.Float64("pickup-angle", 0.6, "microphone projection angle in radians")
	pickupDistance := flags.Float64("pickup-distance", 0.30, "microphone distance in metres")
	fftSize := flags.Int("fft-size", 16_384, "waveform FFT size")
	pitchFrame := flags.Int("pitch-frame", 4096, "pitch-track FFT frame size")
	pitchHop := flags.Int("pitch-hop", 1024, "pitch-track hop size")

	peakCount := flags.Int("peaks", 12, "number of spectral peaks to retain")

	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}

	var value any

	if *suite {
		report, err := physicalanalysis.GenerateReferenceSuite()
		if err != nil {
			return err
		}

		value = report
	} else {
		config := physical.DefaultPhysicalDrum()
		config.Strike.Radius01 = *strikeRadius
		config.Pickup.Radius01 = *pickupRadius
		config.Pickup.AngleRad = *pickupAngle
		config.Pickup.DistanceM = *pickupDistance
		options := physicalanalysis.DefaultOptions()
		options.DurationSeconds = *duration
		options.Velocity01 = *velocity
		options.FFTSize = *fftSize
		options.PitchFrameSize = *pitchFrame
		options.PitchHopSize = *pitchHop
		options.PeakCount = *peakCount

		report, err := physicalanalysis.Analyze(config, options)
		if err != nil {
			return err
		}

		value = report
	}

	return encodeJSON(*outputPath, value, stdout)
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
