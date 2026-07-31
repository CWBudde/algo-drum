// Command render-physical renders the experimental double-headed physical tom
// through its batter-side pickup to a mono 16-bit PCM WAV file for offline
// auditioning.
package main

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/algo-drum/internal/wavio"
)

const maxRenderDuration = 30 * time.Second

var errInvalidRenderOption = errors.New("invalid physical render option")

func main() {
	defaults := physical.DefaultPhysicalDrum()
	outputPath := flag.String("o", "physical-drum.wav", "output mono PCM WAV file")
	duration := flag.Duration("duration", 3*time.Second, "render duration")
	velocity := flag.Float64("velocity", 0.8, "normalized strike velocity [0,1]")
	strikeRadius := flag.Float64(
		"strike-radius",
		defaults.Strike.Radius01,
		"normalized strike radius [0,1]",
	)
	hardness := flag.Float64(
		"hardness",
		defaults.Strike.Hardness01,
		"normalized mallet hardness [0,1]",
	)

	flag.Parse()

	config := physical.DefaultPhysicalDrum()
	config.Strike.Radius01 = *strikeRadius
	config.Strike.Hardness01 = *hardness

	samples, err := render(config, *duration, *velocity)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render-physical: %v\n", err)
		os.Exit(1)
	}

	output, err := os.Create(*outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render-physical: create %s: %v\n", *outputPath, err)
		os.Exit(1)
	}

	peak, encodeErr := wavio.WriteMonoPCM16(output, samples, int(config.SampleRateHz))
	closeErr := output.Close()

	if encodeErr != nil {
		fmt.Fprintf(os.Stderr, "render-physical: encode %s: %v\n", *outputPath, encodeErr)
		os.Exit(1)
	}

	if closeErr != nil {
		fmt.Fprintf(os.Stderr, "render-physical: close %s: %v\n", *outputPath, closeErr)
		os.Exit(1)
	}

	fmt.Printf(
		"wrote %s: %.2fs, %d Hz, %d-mode budget, source peak %.6g\n",
		*outputPath,
		duration.Seconds(),
		int(config.SampleRateHz),
		config.Quality.ModeLimit(),
		peak,
	)
}

func render(config physical.PhysicalDrum, duration time.Duration, velocity float64) ([]float64, error) {
	if duration <= 0 || duration > maxRenderDuration {
		return nil, fmt.Errorf(
			"%w: duration %s outside (0,%s]",
			errInvalidRenderOption,
			duration,
			maxRenderDuration,
		)
	}

	model, err := physical.NewDoubleHead(config)
	if err != nil {
		return nil, err
	}

	if err := model.Trigger(velocity); err != nil {
		return nil, err
	}

	sampleCount := int(math.Round(duration.Seconds() * config.SampleRateHz))
	samples := make([]float64, sampleCount)
	model.Render(samples)

	return samples, nil
}
