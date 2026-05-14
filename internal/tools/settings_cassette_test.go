package tools_test

import (
	"context"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

func TestGetMailSettingsHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "mail_settings_happy")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_get_mail_settings", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["settings"]; !ok {
		t.Fatalf("envelope missing %q", "settings")
	}
}

func TestGetCoreSettingsHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "core_settings_happy")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_get_core_settings", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["settings"]; !ok {
		t.Fatalf("envelope missing %q", "settings")
	}
}
