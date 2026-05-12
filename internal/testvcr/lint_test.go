package testvcr_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func TestLintFlagsBearerToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "leaky.yaml")
	if err := os.WriteFile(path, []byte("Authorization: Bearer eyJraWQiOi.ABCDEFGHIJ.signature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := testvcr.Scan(dir)
	if len(got) == 0 {
		t.Fatal("expected at least one finding")
	}
	if got[0].Rule != "bearer-token" {
		t.Fatalf("rule = %q, want bearer-token", got[0].Rule)
	}
}

func TestLintAllowsPublicPGP(t *testing.T) {
	dir := t.TempDir()
	body := "-----BEGIN PGP PUBLIC KEY BLOCK-----\nstuff\n-----END PGP PUBLIC KEY BLOCK-----\n"
	if err := os.WriteFile(filepath.Join(dir, "public.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := testvcr.Scan(dir)
	for _, f := range got {
		if f.Rule == "pgp" {
			t.Fatalf("public key flagged: %+v", f)
		}
	}
}

func TestLintFlagsPrivatePGP(t *testing.T) {
	dir := t.TempDir()
	body := "-----BEGIN PGP PRIVATE KEY BLOCK-----\n"
	if err := os.WriteFile(filepath.Join(dir, "private.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := testvcr.Scan(dir)
	if len(got) == 0 || got[0].Rule != "pgp-private" {
		t.Fatalf("expected pgp-private finding; got %+v", got)
	}
}

func TestLintFlagsProtonEmail(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "leaky.yaml"), []byte("alice@protonmail.com"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := testvcr.Scan(dir)
	if len(got) == 0 || got[0].Rule != "proton-email" {
		t.Fatalf("expected proton-email finding; got %+v", got)
	}
}

func TestLintAllowsScrubbedAccessToken(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.yaml"), []byte(`"AccessToken": "REDACTED_ACCESSTOKEN_1"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := testvcr.Scan(dir); len(got) != 0 {
		t.Fatalf("expected zero findings on scrubbed cassette, got %+v", got)
	}
}

func TestLintReportsReadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 000 unreliable on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "locked.yaml")
	if err := os.WriteFile(path, []byte("anything\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	got := testvcr.Scan(dir)
	found := false
	for _, f := range got {
		if f.Rule == "read-error" && f.Path == path {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected read-error finding for %s; got %+v", path, got)
	}
}
