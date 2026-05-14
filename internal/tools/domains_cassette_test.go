package tools_test

import (
	"context"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

func TestListCustomDomainsHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "list_custom_domains_happy")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_list_custom_domains", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["domains"]; !ok {
		t.Fatalf("envelope missing %q", "domains")
	}
}

func TestGetCustomDomainHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "get_custom_domain_happy")
	defer h.Close()
	ctx := context.Background()

	listOut, err := h.Call(ctx, "proton_list_custom_domains", map[string]any{})
	if err != nil {
		t.Fatalf("list_custom_domains: %v", err)
	}
	domains, ok := listOut["domains"].([]any)
	if !ok {
		t.Fatalf("domains not a list: %#v", listOut["domains"])
	}
	if len(domains) == 0 {
		t.Skip("cassette has no custom domains; re-record after adding a domain to the test account")
	}
	first, ok := domains[0].(map[string]any)
	if !ok {
		t.Fatalf("domain[0] not an object: %#v", domains[0])
	}
	id, ok := first["id"].(string)
	if !ok || id == "" {
		t.Fatalf("domain[0].id missing or empty: %#v", first)
	}

	out, err := h.Call(ctx, "proton_get_custom_domain", map[string]any{"id": id})
	if err != nil {
		t.Fatalf("get_custom_domain: %v", err)
	}
	if _, ok := out["domain"]; !ok {
		t.Fatalf("envelope missing %q", "domain")
	}
}
