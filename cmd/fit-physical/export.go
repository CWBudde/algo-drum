package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/algo-drum/internal/wavio"
)

// maxExportDuration bounds -wav-duration. The fit itself renders 1.2 s, which is
// enough to measure and too short to judge a tail by ear, so the export is
// deliberately allowed to be much longer than the thing it exports.
const maxExportDuration = 30 * time.Second

var errInvalidExport = errors.New("invalid WAV export option")

// exportCandidate renders one fitted candidate and writes it as a mono WAV.
//
// It renders from the candidate's own recorded Config rather than by replaying
// the search position, so what lands on disk is exactly the drum the report
// describes — including when the report was resumed from a checkpoint, where
// the position was measured by this build but the bank came from another run.
//
// velocity01 is passed rather than read off the candidate because a joint fit
// has one per take and none of them is the drum. The caller picks which take to
// listen to; see -wav-take.
func exportCandidate(
	path string,
	candidate Candidate,
	velocity01 float64,
	duration time.Duration,
) (float64, error) {
	if path == "" {
		return 0, fmt.Errorf("%w: empty path", errInvalidExport)
	}

	if duration <= 0 || duration > maxExportDuration {
		return 0, fmt.Errorf("%w: duration %s outside (0,%s]",
			errInvalidExport, duration, maxExportDuration)
	}

	samples, err := renderConfig(candidate.Config, velocity01, duration)
	if err != nil {
		return 0, err
	}

	if err := ensureDir(path); err != nil {
		return 0, fmt.Errorf("create %s: %w", path, err)
	}

	output, err := os.Create(path)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", path, err)
	}

	peak, encodeErr := wavio.WriteMonoPCM16(output, samples, int(candidate.Config.SampleRateHz))

	closeErr := output.Close()

	if encodeErr != nil {
		return 0, fmt.Errorf("encode %s: %w", path, encodeErr)
	}

	if closeErr != nil {
		return 0, fmt.Errorf("close %s: %w", path, closeErr)
	}

	return peak, nil
}

func renderConfig(
	config physical.PhysicalDrum,
	velocity01 float64,
	duration time.Duration,
) ([]float64, error) {
	model, err := physical.NewDoubleHead(config)
	if err != nil {
		return nil, err
	}

	if err := model.Trigger(velocity01); err != nil {
		return nil, err
	}

	samples := make([]float64, int(math.Round(duration.Seconds()*config.SampleRateHz)))
	model.Render(samples)

	return samples, nil
}
