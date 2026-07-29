// Command render-physical renders the experimental single-head physical drum
// to a mono 16-bit PCM WAV file for offline auditioning.
package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/cwbudde/algo-drum/internal/physical"
)

const (
	maxRenderDuration = 30 * time.Second
	wavHeaderBytes    = 44
	pcm16Bytes        = 2
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

	wav, peak, err := encodePCM16WAV(samples, int(config.SampleRateHz))
	if err != nil {
		fmt.Fprintf(os.Stderr, "render-physical: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*outputPath, wav, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "render-physical: write %s: %v\n", *outputPath, err)
		os.Exit(1)
	}

	fmt.Printf(
		"wrote %s: %.2fs, %d Hz, %d modes, source peak %.6g\n",
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

func encodePCM16WAV(samples []float64, sampleRate int) ([]byte, float64, error) {
	if sampleRate <= 0 {
		return nil, 0, fmt.Errorf("%w: sample rate %d", errInvalidRenderOption, sampleRate)
	}

	peak := 0.0

	for index, sample := range samples {
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			return nil, 0, fmt.Errorf("%w: non-finite sample %d", errInvalidRenderOption, index)
		}

		peak = math.Max(peak, math.Abs(sample))
	}

	scale := 1.0
	if peak > 0 {
		scale = normalizedPeak / peak
	}

	dataBytes := len(samples) * pcm16Bytes
	wav := make([]byte, wavHeaderBytes+dataBytes)
	copy(wav[0:4], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(len(wav)-8))
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], 1)
	binary.LittleEndian.PutUint32(wav[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(wav[28:32], uint32(sampleRate*pcm16Bytes))
	binary.LittleEndian.PutUint16(wav[32:34], pcm16Bytes)
	binary.LittleEndian.PutUint16(wav[34:36], 16)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], uint32(dataBytes))

	for index, sample := range samples {
		normalized := math.Max(-1, math.Min(1, sample*scale))
		pcm := int16(math.Round(normalized * math.MaxInt16))
		binary.LittleEndian.PutUint16(
			wav[wavHeaderBytes+index*pcm16Bytes:],
			uint16(pcm),
		)
	}

	return wav, peak, nil
}
