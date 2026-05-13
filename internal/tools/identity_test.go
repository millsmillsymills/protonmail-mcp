package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zalando/go-keyring"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
	"github.com/millsmillsymills/protonmail-mcp/internal/tools"
)

func TestWhoamiRoundTrip(t *testing.T) {
	h := testharness.BootDevServer(t, "user@example.test", "hunter2")
	defer h.Close()

	out, err := h.Call(context.Background(), "proton_whoami", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if out["email"] != "user@example.test" {
		t.Fatalf("unexpected email: %#v", out)
	}
}

func TestWhoamiAuthRequiredEmptyKeychain(t *testing.T) {
	keyring.MockInit()

	kc := keychain.New()
	sess := session.New("https://mail.proton.me/api", kc)

	mcpSrv := mcp.NewServer(&mcp.Implementation{Name: "protonmail-mcp", Version: "test"}, nil)
	tools.Register(mcpSrv, tools.Deps{Session: sess})

	clientT, serverT := mcp.NewInMemoryTransports()
	srvSession, err := mcpSrv.Connect(context.Background(), serverT, nil)
	if err != nil {
		t.Fatalf("mcp server connect: %v", err)
	}
	t.Cleanup(func() { _ = srvSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	csess, err := client.Connect(context.Background(), clientT, nil)
	if err != nil {
		t.Fatalf("mcp client connect: %v", err)
	}
	t.Cleanup(func() { _ = csess.Close() })

	res, err := csess.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "proton_whoami",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("want IsError=true, got IsError=false; content=%+v", res.Content)
	}
	var text string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}
	if !strings.Contains(text, "proton/auth_required") {
		t.Fatalf("want proton/auth_required in error text, got %q", text)
	}
}
