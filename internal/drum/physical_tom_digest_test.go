package drum

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"
)

// physicalTomRenderDigest is two seconds of the shipped physical Tom at its
// default bank, 48 kHz, velocity 1.
//
// It exists to survive refactors of the *plumbing* around the voice. The knob
// spec table and the normalized→SI mapping are the calibration itself, and the
// mapping is deliberately shared with the offline fitter rather than copied, so
// a change to where it lives is a change nothing else in this package would
// notice: PhysicalTomConfig's own tests compare configurations, and the level
// test tolerates a 0.25 window. A digest is the only assertion that hears the
// difference.
//
// Like every digest in this repository it is an amd64 fact — see AGENTS.md, "The
// portable path is what ships". A mismatch on js/wasm is math.Exp, not this
// package. A mismatch here, on the host, is the calibration having moved.
const physicalTomRenderDigest = "b5001868c36c86bc46e146368f0e62bf5d5f419da56e9bbc78f0b8ab6c224e1c"

func TestPhysicalTomRenderIsBitExact(t *testing.T) {
	t.Parallel()

	voice, err := newPhysicalTom(48_000)
	if err != nil {
		t.Fatal(err)
	}

	voice.Trigger(1)

	digest := sha256.New()

	var word [8]byte

	for range 96_000 {
		binary.LittleEndian.PutUint64(word[:], math.Float64bits(voice.Tick()))
		digest.Write(word[:])
	}

	if got := hex.EncodeToString(digest.Sum(nil)); got != physicalTomRenderDigest {
		t.Fatalf(
			"physical tom render digest %s, want %s",
			got,
			physicalTomRenderDigest,
		)
	}
}
