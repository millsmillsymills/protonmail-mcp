package log_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	mcplog "protonmail-mcp/internal/log"
)

func TestRedactsSensitiveFields(t *testing.T) {
	var buf bytes.Buffer
	logger := mcplog.New(slog.LevelDebug, &buf)
	logger.Info("auth attempt",
		"username", "andy@example.com",
		"password", "hunter2",
		"refresh_token", "abc.def",
		"totp", "123456",
		"PassPhrase", "shh",
		"safe_field", "ok",
	)

	var rec map[string]any
	if err := json.Unmarshal(stripPrefix(buf.Bytes()), &rec); err != nil {
		t.Fatalf("not valid JSON: %v\noutput: %s", err, buf.String())
	}

	cases := []struct {
		key      string
		redacted bool
	}{
		{"username", false},
		{"password", true},
		{"refresh_token", true},
		{"totp", true},
		{"PassPhrase", true},
		{"safe_field", false},
	}
	for _, c := range cases {
		got, _ := rec[c.key].(string)
		if c.redacted && got != "<redacted>" {
			t.Errorf("%s: want <redacted>, got %q", c.key, got)
		}
		if !c.redacted && got == "<redacted>" {
			t.Errorf("%s: was unexpectedly redacted", c.key)
		}
	}
}

// strips the slog JSON record prefix if present (no-op for pure-JSON handler)
func stripPrefix(b []byte) []byte {
	if i := bytes.IndexByte(b, '{'); i > 0 {
		return b[i:]
	}
	return bytes.TrimSpace(b)
}

func TestRedactsNestedGroups(t *testing.T) {
	var buf bytes.Buffer
	logger := mcplog.New(slog.LevelDebug, &buf)
	logger.Info("nested",
		slog.Group("auth", "access_token", "leak", "user", "andy"),
	)
	out := buf.String()
	if strings.Contains(out, "leak") {
		t.Errorf("nested token not redacted: %s", out)
	}
	if !strings.Contains(out, "andy") {
		t.Errorf("nested non-sensitive field was lost: %s", out)
	}
}
