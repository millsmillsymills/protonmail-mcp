package testvcr_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func TestModeDefaultsToReplay(t *testing.T) {
	t.Setenv("VCR_MODE", "")
	if got := testvcr.Mode(); got != testvcr.ModeReplay {
		t.Fatalf("default mode = %v, want replay", got)
	}
}

func TestModeRecord(t *testing.T) {
	t.Setenv("VCR_MODE", "record")
	if got := testvcr.Mode(); got != testvcr.ModeRecord {
		t.Fatalf("mode = %v, want record", got)
	}
}

func TestCassettePathResolvesUnderCallerTestdata(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VCR_TESTDATA_OVERRIDE", dir)
	t.Setenv("VCR_MODE", "replay")
	yaml := "version: 2\ninteractions: []\n"
	if err := os.WriteFile(filepath.Join(dir, "smoke.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := testvcr.New(t, "smoke")
	if rt == nil {
		t.Fatal("expected non-nil transport")
	}
}
