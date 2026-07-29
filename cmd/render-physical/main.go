// Command render-physical renders the experimental single-head physical drum
// to a mono 16-bit PCM WAV file for offline auditioning.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/wav"
	"github.com/go-audio/audio"
)

const (
	maxRenderDuration = 30 * time.Second
	normalizedPeak    = 0.9
)

var errInvalidRenderOption = errors.New("invalid physical render option")

func main() {
	outputPath := flag.String("o", "physical-drum.wav", "output mono PCM WAV file")
	duration := flag.Duration("duration", 3*time.Second, "render duration")
	velocity := flag.Float64("velocity", 0.8, "normalized strike velocity [0,1]")
	strikeRadius := flag.Float64("strike-radius", 0.45, "normalized strike radius [0,1]")
	hardness := flag.Float64("hardness", 0.7, "normalized mallet hardness [0,1]")

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

	peak, encodeErr := writePCM16WAV(output, samples, int(config.SampleRateHz))
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

	model, err := physical.NewSingleHead(config)
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

func writePCM16WAV(
	writer io.WriteSeeker,
	samples []float64,
	sampleRate int,
) (float64, error) {
	if writer == nil {
		return 0, fmt.Errorf("%w: nil WAV writer", errInvalidRenderOption)
	}

	if sampleRate <= 0 {
		return 0, fmt.Errorf("%w: sample rate %d", errInvalidRenderOption, sampleRate)
	}

	peak := 0.0

	for index, sample := range samples {
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			return 0, fmt.Errorf("%w: non-finite sample %d", errInvalidRenderOption, index)
		}

		peak = math.Max(peak, math.Abs(sample))
	}

	scale := 1.0
	if peak > 0 {
		scale = normalizedPeak / peak
	}

	data := make([]float32, len(samples))

	for index, sample := range samples {
		data[index] = float32(math.Max(-1, math.Min(1, sample*scale)))
	}

	buffer := &audio.Float32Buffer{
		Format: &audio.Format{
			NumChannels: 1,
			SampleRate:  sampleRate,
		},
		Data: data,
	}
	encoder := wav.NewEncoder(writer, sampleRate, 16, 1, 1)

	if err := encoder.Write(buffer); err != nil {
		return 0, fmt.Errorf("write WAV samples: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return 0, fmt.Errorf("finalize WAV: %w", err)
	}

	return peak, nil
}
