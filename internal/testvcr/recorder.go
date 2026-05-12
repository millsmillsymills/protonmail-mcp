// Package testvcr provides a thin wrapper around gopkg.in/dnaeon/go-vcr.v4 for
// recording and replaying HTTP exchanges in tests against a real Proton API.
package testvcr

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

// RecorderMode reports whether tests should replay committed cassettes or
// record fresh interactions against the live API.
type RecorderMode int

const (
	ModeReplay RecorderMode = iota
	ModeRecord
)

// Mode reads VCR_MODE from the environment. Defaults to ModeReplay.
func Mode() RecorderMode {
	switch os.Getenv("VCR_MODE") {
	case "record":
		return ModeRecord
	default:
		return ModeReplay
	}
}

// New constructs a RoundTripper backed by a cassette. The cassette path is
// derived from the caller's package directory + name: testdata/cassettes/<name>.yaml.
// VCR_TESTDATA_OVERRIDE, when set, replaces the testdata/cassettes prefix.
func New(t *testing.T, name string) http.RoundTripper {
	t.Helper()
	if err := guardRecordInCI(); err != nil {
		t.Fatal(err)
	}
	path := resolvePath(t, name)
	mode := recorder.ModeReplayOnly
	if Mode() == ModeRecord {
		mode = recorder.ModeRecordOnly
	}
	r, err := recorder.New(path,
		recorder.WithMode(mode),
		recorder.WithMatcher(BodyAwareMatcher),
		recorder.WithHook(saveHook, recorder.BeforeSaveHook),
	)
	if err != nil {
		t.Fatalf("testvcr.New(%q): %v", name, err)
	}
	t.Cleanup(func() {
		if err := r.Stop(); err != nil {
			t.Errorf("testvcr.Stop: %v", err)
		}
	})
	return r.GetDefaultClient().Transport
}

func resolvePath(t *testing.T, name string) string {
	t.Helper()
	if override := os.Getenv("VCR_TESTDATA_OVERRIDE"); override != "" {
		return filepath.Join(override, name)
	}
	// Walk the stack and pick the first frame outside testvcr/testharness
	// source files. _test.go files in those packages stay eligible so their
	// own cassette tests resolve under their package's testdata/.
	for i := 1; i < 16; i++ {
		_, file, _, ok := runtime.Caller(i)
		if !ok {
			break
		}
		if !strings.HasSuffix(file, "_test.go") &&
			(strings.Contains(file, "/internal/testvcr/") ||
				strings.Contains(file, "/internal/testharness/")) {
			continue
		}
		return filepath.Join(filepath.Dir(file), "testdata", "cassettes", name)
	}
	t.Fatal("testvcr: no caller frame outside testvcr/testharness")
	return ""
}

func guardRecordInCI() error {
	if Mode() != ModeRecord {
		return nil
	}
	for _, k := range []string{"CI", "GITHUB_ACTIONS", "BUILDKITE", "CIRCLECI"} {
		if v := os.Getenv(k); v != "" && v != "false" && v != "0" {
			return &CIRecordError{Env: k}
		}
	}
	return nil
}

// CIRecordError is returned when VCR_MODE=record is set in a CI environment.
type CIRecordError struct{ Env string }

func (e *CIRecordError) Error() string {
	return "testvcr: refusing to record while " + e.Env + " is set (CI guard)"
}
