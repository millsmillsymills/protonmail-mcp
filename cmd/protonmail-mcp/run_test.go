package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/zalando/go-keyring"
)

func TestStatusLoggedOut(t *testing.T) {
	keyring.MockInit()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(),
		[]string{"status"},
		[]string{"PROTONMAIL_MCP_API_URL=https://mail.proton.me/api"},
		strings.NewReader(""),
		stdout,
		stderr,
		nil,
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "not logged in") {
		t.Fatalf("stdout = %q, want 'not logged in'", stdout.String())
	}
}

func TestLogoutClearsKeychain(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	seed := keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"}
	if err := kc.SaveSession(seed); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(context.Background(),
		[]string{"logout"},
		nil,
		strings.NewReader(""),
		stdout,
		stderr,
		nil,
	)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if _, err := kc.LoadSession(); err == nil {
		t.Fatal("session still present after logout")
	}
}
