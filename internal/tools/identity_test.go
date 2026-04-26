package tools_test

import (
	"context"
	"testing"

	"github.com/zalando/go-keyring"
	"protonmail-mcp/internal/tools/internal/testharness"
)

func TestWhoamiRoundTrip(t *testing.T) {
	keyring.MockInit()
	h := testharness.Boot(t, "user@example.test", "hunter2")
	defer h.Close()

	out, err := h.Call(context.Background(), "proton_whoami", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if out["email"] != "user@example.test" {
		t.Fatalf("unexpected email: %#v", out)
	}
}
