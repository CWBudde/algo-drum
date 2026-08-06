package drum

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// workerPath is the frontend half of the protocol contract, relative to this
// package.
const workerPath = "../../web/src/engine/audioWorker.ts"

var workerProtocolVersion = regexp.MustCompile(`PROTOCOL_VERSION\s*=\s*(\d+)`)

// TestProtocolVersionAgreesWithWorker keeps the two halves of the version gate
// from drifting. The worker refuses to run when the engine reports a different
// number, so a bump applied to only one side would make every build fail to
// load — this test turns that into a failed `go test` instead.
func TestProtocolVersionAgreesWithWorker(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatalf("read %s: %v", workerPath, err)
	}

	match := workerProtocolVersion.FindSubmatch(source)
	if match == nil {
		t.Fatalf(
			"no PROTOCOL_VERSION found in %s: the worker's half of the version "+
				"gate was renamed or removed, which silently disables this check",
			workerPath,
		)
	}

	want, err := strconv.Atoi(string(match[1]))
	if err != nil {
		t.Fatalf("parse PROTOCOL_VERSION %q: %v", match[1], err)
	}

	if want != ProtocolVersion {
		t.Fatalf(
			"ProtocolVersion = %d, but %s pins PROTOCOL_VERSION = %d: bump both "+
				"or neither, or no build of the app will load",
			ProtocolVersion, workerPath, want,
		)
	}
}
