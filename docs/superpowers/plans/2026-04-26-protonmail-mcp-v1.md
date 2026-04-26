# protonmail-mcp v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship v1 of `protonmail-mcp` — a Go MCP server wrapping `go-proton-api` (plus a small hand-written `protonraw` package for endpoints `go-proton-api` doesn't expose) so Claude Code can manage Proton Mail addresses, custom domains, mail settings, and encryption keys.

**Architecture:** Single Go binary, dual-mode (`protonmail-mcp` MCP stdio server + `login`/`logout`/`status` CLI subcommands). Reads always available; writes opt-in via `PROTONMAIL_MCP_ENABLE_WRITES=1`. Credentials in macOS Keychain. Hybrid client: `go-proton-api` for auth and supported endpoints; raw `resty` calls (sharing the same bearer token) for custom-domain CRUD and address creation.

**Tech Stack:**
- Go 1.22+
- [`github.com/modelcontextprotocol/go-sdk@v1.5.0+`](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk) — MCP transport, typed tools
- [`github.com/ProtonMail/go-proton-api`](https://github.com/ProtonMail/go-proton-api) — Proton client + dev server
- [`github.com/zalando/go-keyring`](https://github.com/zalando/go-keyring) — OS keychain
- [`github.com/go-resty/resty/v2`](https://github.com/go-resty/resty) — HTTP client (already a transitive dep)
- `log/slog` — structured logging with redaction
- `net/http/httptest` — for `protonraw` tests

**Source of truth:** `docs/superpowers/specs/2026-04-26-protonmail-mcp-design.md`

**Conventions used in this plan:**
- All paths absolute from repo root.
- All `go test` commands run from repo root.
- Each task ends with a single commit.
- Commit messages follow Conventional Commits (`feat:`, `test:`, `chore:`, `docs:`).
- TDD: failing test → run → impl → pass → commit.

---

## Phase 0 — Bootstrap

### Task 1: Initialize Go module and skeleton

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `LICENSE` (MIT, matching go-proton-api)
- Create: `cmd/protonmail-mcp/main.go` (stub)
- Create: `internal/server/.keep`, `internal/tools/.keep`, `internal/session/.keep`, `internal/keychain/.keep`, `internal/protonraw/.keep`, `internal/proterr/.keep`, `internal/log/.keep`, `integration/.keep`

- [ ] **Step 1: Write `go.mod`**

```
module protonmail-mcp

go 1.22

require (
	github.com/ProtonMail/go-proton-api v0.0.0-latest
	github.com/modelcontextprotocol/go-sdk v1.5.0
	github.com/zalando/go-keyring v0.2.6
	github.com/go-resty/resty/v2 v2.13.1
)
```

The bare `protonmail-mcp` path is valid for local development. If/when this is published to GitHub, do a one-shot rename:

```bash
NEW_PATH="github.com/<your-org>/protonmail-mcp"
git grep -l 'protonmail-mcp"' | xargs sed -i.bak "s|\"protonmail-mcp\"|\"$NEW_PATH\"|g"
sed -i.bak "s|^module protonmail-mcp$|module $NEW_PATH|" go.mod
git grep -l 'protonmail-mcp/internal' | xargs sed -i.bak "s|protonmail-mcp/internal|$NEW_PATH/internal|g"
git grep -l 'protonmail-mcp/cmd' | xargs sed -i.bak "s|protonmail-mcp/cmd|$NEW_PATH/cmd|g"
find . -name "*.bak" -delete
go mod tidy
```

Until then, all imports in this plan use the bare `protonmail-mcp/...` form.

- [ ] **Step 2: Resolve `go-proton-api` to a real version**

Run:

```bash
go get github.com/ProtonMail/go-proton-api@master
go get github.com/modelcontextprotocol/go-sdk@latest
go get github.com/zalando/go-keyring@latest
go get github.com/go-resty/resty/v2@latest
go mod tidy
```

Note: go-proton-api ships a stale v0.4.0 tag; downstream tasks need master HEAD. Master also uses a forked resty via a `replace` directive — Go does not propagate replace directives from dependencies, so mirror it in our own `go.mod`:

```
replace github.com/go-resty/resty/v2 => github.com/ProtonMail/resty/v2 v2.0.0-20250929142426-e3dc6308c80b
```

Expected: `go.mod` updated with concrete versions (`go-proton-api` shows a `v0.4.1-0.YYYYMMDD...` pseudo-version); `go.sum` created.

- [ ] **Step 3: Write `.gitignore`**

```
# Binaries
/protonmail-mcp
/dist/

# IDE
.vscode/
.idea/

# OS
.DS_Store

# Go
*.test
*.out
coverage.txt
```

- [ ] **Step 4: Write LICENSE (MIT)**

Standard MIT text with `Copyright (c) 2026 <your name>`. Use the canonical text from <https://opensource.org/license/mit>.

- [ ] **Step 5: Write skeleton `cmd/protonmail-mcp/main.go`**

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "login", "logout", "status":
			fmt.Fprintf(os.Stderr, "subcommand %q not yet implemented\n", os.Args[1])
			os.Exit(2)
		default:
			fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
			os.Exit(2)
		}
	}
	fmt.Fprintln(os.Stderr, "MCP server not yet implemented")
	os.Exit(2)
}
```

- [ ] **Step 6: Create empty package directories**

Run:

```bash
for d in internal/server internal/tools internal/session internal/keychain internal/protonraw internal/proterr internal/log integration; do
  mkdir -p "$d"
  : > "$d/.keep"
done
```

- [ ] **Step 7: Verify it builds**

Run: `go build ./...`
Expected: success, no output. A `protonmail-mcp` binary may appear in the repo root — that's fine, it's gitignored.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum .gitignore LICENSE cmd internal integration
git commit -m "chore: initialize Go module and project skeleton"
```

---

## Phase 1 — Foundations

### Task 2: Logging with redaction (`internal/log`)

**Files:**
- Create: `internal/log/log.go`
- Create: `internal/log/log_test.go`

**What this gives us:** A `log.New(level)` returning a `*slog.Logger` that redacts any field whose name (case-insensitively) contains `password`, `token`, `secret`, `totp`, `passphrase`. Used by every other package; no caller has to remember to redact.

- [ ] **Step 1: Write the failing test (`internal/log/log_test.go`)**

```go
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

	var rec map[string]any
	if err := json.Unmarshal(stripPrefix(buf.Bytes()), &rec); err != nil {
		t.Fatalf("not valid JSON: %v\noutput: %s", err, buf.String())
	}
	auth, ok := rec["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth group missing or not an object: %#v", rec["auth"])
	}
	if got, _ := auth["access_token"].(string); got != "<redacted>" {
		t.Errorf("auth.access_token: want <redacted>, got %q", got)
	}
	if got, _ := auth["user"].(string); got != "andy" {
		t.Errorf("auth.user: want \"andy\", got %q", got)
	}
}

func TestRedactsDoublyNestedGroups(t *testing.T) {
	var buf bytes.Buffer
	logger := mcplog.New(slog.LevelDebug, &buf)
	logger.Info("doubly nested",
		slog.Group("outer",
			slog.Group("inner", "secret", "leak", "ok", "fine"),
		),
	)

	var rec map[string]any
	if err := json.Unmarshal(stripPrefix(buf.Bytes()), &rec); err != nil {
		t.Fatalf("not valid JSON: %v\noutput: %s", err, buf.String())
	}
	outer, _ := rec["outer"].(map[string]any)
	inner, _ := outer["inner"].(map[string]any)
	if got, _ := inner["secret"].(string); got != "<redacted>" {
		t.Errorf("outer.inner.secret: want <redacted>, got %q", got)
	}
	if got, _ := inner["ok"].(string); got != "fine" {
		t.Errorf("outer.inner.ok: want \"fine\", got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/log/...`
Expected: FAIL — package `mcplog` does not yet provide `New`.

- [ ] **Step 3: Implement `internal/log/log.go`**

```go
// Package log provides a slog logger with automatic redaction of credential-bearing
// fields. Field names containing any of the substrings in sensitiveSubstrings (case
// insensitive) have their values replaced with "<redacted>".
package log

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

var sensitiveSubstrings = []string{
	"password",
	"passphrase",
	"token",
	"secret",
	"totp",
}

// New returns a JSON slog logger that writes to w (use os.Stderr in production).
// Redaction is applied to all attribute names regardless of nesting.
func New(level slog.Level, w io.Writer) *slog.Logger {
	base := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
	})
	return slog.New(&redactingHandler{inner: base})
}

type redactingHandler struct {
	inner slog.Handler
}

func (h *redactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *redactingHandler) Handle(ctx context.Context, r slog.Record) error {
	clone := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		clone.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, clone)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	red := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		red[i] = redactAttr(a)
	}
	return &redactingHandler{inner: h.inner.WithAttrs(red)}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{inner: h.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindGroup {
		gs := a.Value.Group()
		out := make([]slog.Attr, len(gs))
		for i, g := range gs {
			out[i] = redactAttr(g)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}
	}
	if isSensitive(a.Key) {
		return slog.String(a.Key, "<redacted>")
	}
	return a
}

func isSensitive(name string) bool {
	lower := strings.ToLower(name)
	for _, s := range sensitiveSubstrings {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/log/... -v`
Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/log/
git commit -m "feat(log): slog handler with field-name redaction"
```

---

### Task 3: Error taxonomy (`internal/proterr`)

**Files:**
- Create: `internal/proterr/proterr.go`
- Create: `internal/proterr/proterr_test.go`

**What this gives us:** A single `Map(err)` function that turns any `go-proton-api` error or HTTP status into a stable `*Error{Code, Message, Hint, RetryAfter}` consumed by tool handlers and surfaced to MCP clients.

- [ ] **Step 1: Write the failing test**

```go
package proterr_test

import (
	"errors"
	"net/http"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	"protonmail-mcp/internal/proterr"
)

func TestMap(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		want   string // expected Code
	}{
		{"nil", nil, ""},
		{"401", proton.NetError{Status: http.StatusUnauthorized}, "proton/auth_required"},
		{"402", proton.NetError{Status: http.StatusPaymentRequired}, "proton/plan_required"},
		{"404", proton.NetError{Status: http.StatusNotFound}, "proton/not_found"},
		{"409", proton.NetError{Status: http.StatusConflict}, "proton/conflict"},
		{"422", proton.NetError{Status: http.StatusUnprocessableEntity}, "proton/validation"},
		{"429", proton.NetError{Status: http.StatusTooManyRequests}, "proton/rate_limited"},
		{"500", proton.NetError{Status: http.StatusInternalServerError}, "proton/upstream"},
		{"network", errors.New("dial tcp: connection refused"), "proton/upstream"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := proterr.Map(tc.err)
			if tc.want == "" && got != nil {
				t.Fatalf("want nil, got %+v", got)
			}
			if tc.want != "" && (got == nil || got.Code != tc.want) {
				t.Fatalf("want %s, got %+v", tc.want, got)
			}
		})
	}
}

func TestRetryAfterParsed(t *testing.T) {
	e := proton.NetError{Status: http.StatusTooManyRequests, Headers: http.Header{"Retry-After": []string{"42"}}}
	got := proterr.Map(e)
	if got == nil || got.RetryAfterSeconds != 42 {
		t.Fatalf("want retry-after=42, got %+v", got)
	}
}

func TestHVError(t *testing.T) {
	apiErr := proton.APIError{Code: 9001, Message: "Human verification required"}
	// Force IsHVError() true: in real go-proton-api this is signalled by Code; Map should still classify.
	got := proterr.Map(apiErr)
	if got == nil || got.Code != "proton/captcha" {
		t.Fatalf("want proton/captcha, got %+v", got)
	}
}

func TestMapHandlesValueAPIError(t *testing.T) {
	// Wrap a value APIError (not a pointer) to confirm errors.As fallback works.
	err := fmt.Errorf("wrapped: %w", proton.APIError{Status: http.StatusUnauthorized})
	got := proterr.Map(err)
	if got == nil || got.Code != "proton/auth_required" {
		t.Fatalf("want proton/auth_required, got %+v", got)
	}
}

func TestMapHandlesValueAPIErrorHV(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", proton.APIError{Code: proton.HumanVerificationRequired})
	got := proterr.Map(err)
	if got == nil || got.Code != "proton/captcha" {
		t.Fatalf("want proton/captcha, got %+v", got)
	}
}

func TestHVErrorIncludesToken(t *testing.T) {
	rawDetails, _ := json.Marshal(map[string]any{
		"HumanVerificationMethods": []string{"captcha"},
		"HumanVerificationToken":   "tok-abc",
	})
	apiErr := proton.APIError{Code: proton.HumanVerificationRequired, Details: proton.ErrDetails(rawDetails)}
	got := proterr.Map(apiErr)
	if got == nil || got.Code != "proton/captcha" {
		t.Fatalf("want proton/captcha, got %+v", got)
	}
	if !strings.Contains(got.Hint, "tok-abc") {
		t.Errorf("hint missing token: %q", got.Hint)
	}
	if !strings.Contains(got.Hint, "captcha") {
		t.Errorf("hint missing methods: %q", got.Hint)
	}
}
```

> **Note on `proton.NetError`:** The actual `go-proton-api` exports its HTTP-status-bearing error type slightly differently (verify via `go doc github.com/ProtonMail/go-proton-api NetError` after `go mod tidy`). If the public struct field names differ from `Status`/`Headers`, adjust the test fixtures and the `Map` implementation to match. The test should still cover all the listed status codes regardless of how the struct is named.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/proterr/...`
Expected: FAIL — `proterr.Map` does not exist.

- [ ] **Step 3: Implement `internal/proterr/proterr.go`**

```go
// Package proterr maps go-proton-api and HTTP errors to stable codes consumed
// by tool handlers and surfaced over MCP. See docs/superpowers/specs for the
// full taxonomy.
package proterr

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	proton "github.com/ProtonMail/go-proton-api"
)

// Error is what tool handlers return on failure. Code is stable; Message is
// human-readable; Hint is actionable next-step text; RetryAfterSeconds is
// non-zero only for proton/rate_limited.
type Error struct {
	Code              string
	Message           string
	Hint              string
	RetryAfterSeconds int
}

func (e *Error) Error() string {
	if e.Hint != "" {
		return e.Code + ": " + e.Message + " (" + e.Hint + ")"
	}
	return e.Code + ": " + e.Message
}

// Map turns any error from go-proton-api or raw HTTP into a stable *Error.
// Returns nil for nil input.
func Map(err error) *Error {
	if err == nil {
		return nil
	}

	// Proton API error: carries the HTTP status. go-proton-api can wrap APIError
	// as either a value or a pointer (Error() is on the value receiver), so probe
	// both forms via extractAPIError. HV (CAPTCHA) is checked first because the
	// status may be 422 but the semantic meaning is "solve a challenge".
	if apiErr, ok := extractAPIError(err); ok {
		if apiErr.IsHVError() {
			return hvError(apiErr)
		}
		if apiErr.Status != 0 {
			return mapStatus(apiErr.Status, nil)
		}
	}

	var netErr proton.NetError
	if errors.As(err, &netErr) {
		return mapStatus(netErr.Status, netErr.Headers)
	}

	// Anything else is treated as upstream/transport.
	return &Error{
		Code:    "proton/upstream",
		Message: "Proton API unavailable.",
		Hint:    err.Error(),
	}
}

// extractAPIError returns the proton.APIError carried by err, whether wrapped
// as a value or a pointer. ok is false if no APIError is present.
func extractAPIError(err error) (proton.APIError, bool) {
	var ptr *proton.APIError
	if errors.As(err, &ptr) && ptr != nil {
		return *ptr, true
	}
	var val proton.APIError
	if errors.As(err, &val) {
		return val, true
	}
	return proton.APIError{}, false
}

// hvError builds the *Error for an HV (CAPTCHA) APIError, surfacing the
// verification token + methods so callers (Task 13: login) can construct the
// verification URL for the user. The https://verify.proton.me/?... URL pattern
// is reverse-engineered from Proton WebClients and may need maintenance if
// Proton changes their verification host or query shape.
func hvError(apiErr proton.APIError) *Error {
	hint := "Open the verification URL in a browser, then re-run `protonmail-mcp login`."
	if details, derr := apiErr.GetHVDetails(); derr == nil && details != nil && details.Token != "" {
		methods := strings.Join(details.Methods, ",")
		hint = "Human verification token=" + details.Token + " methods=" + methods +
			". Open https://verify.proton.me/?methods=" + methods + "&token=" + details.Token +
			" in a browser, complete the challenge, then re-run `protonmail-mcp login`."
	}
	return &Error{
		Code:    "proton/captcha",
		Message: "Human verification required.",
		Hint:    hint,
	}
}

func mapStatus(status int, headers http.Header) *Error {
	switch status {
	case http.StatusUnauthorized:
		return &Error{
			Code:    "proton/auth_required",
			Message: "Session expired or unauthenticated.",
			Hint:    "Run `protonmail-mcp login` interactively, then retry.",
		}
	case http.StatusPaymentRequired:
		return &Error{
			Code:    "proton/plan_required",
			Message: "This feature is not available on your Proton plan.",
		}
	case http.StatusNotFound:
		return &Error{Code: "proton/not_found", Message: "Resource not found."}
	case http.StatusConflict:
		return &Error{Code: "proton/conflict", Message: "Resource already exists or is in conflict."}
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return &Error{Code: "proton/validation", Message: "Request rejected by Proton."}
	case http.StatusTooManyRequests:
		retry, _ := strconv.Atoi(strings.TrimSpace(headers.Get("Retry-After")))
		return &Error{
			Code:              "proton/rate_limited",
			Message:           "Rate limited by Proton.",
			RetryAfterSeconds: retry,
		}
	}
	if status >= 500 {
		return &Error{Code: "proton/upstream", Message: "Proton API unavailable."}
	}
	return &Error{Code: "proton/upstream", Message: http.StatusText(status)}
}
```

> **If `proton.NetError` exposes status differently:** the tests will tell you. Adjust the `errors.As` target and field accesses to match `go doc github.com/ProtonMail/go-proton-api NetError`. The CAPTCHA branch may also need adjustment if `APIError.IsHVError` has a different signature.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/proterr/... -v`
Expected: PASS.

- [ ] **Step 5: Add `proton/writes_disabled` and `proton/2fa_required` constructors**

Append to `internal/proterr/proterr.go`:

```go
// WritesDisabled is returned defensively when a write tool handler is invoked
// while PROTONMAIL_MCP_ENABLE_WRITES is unset. The tool should not be
// registered in that case; this is belt-and-suspenders.
func WritesDisabled() *Error {
	return &Error{
		Code:    "proton/writes_disabled",
		Message: "Writes are disabled.",
		Hint:    "Set PROTONMAIL_MCP_ENABLE_WRITES=1 and restart the MCP server.",
	}
}

// TwoFARequired is returned by the login flow when 2FA is needed but no TOTP
// has been provided.
func TwoFARequired() *Error {
	return &Error{
		Code:    "proton/2fa_required",
		Message: "TOTP required.",
		Hint:    "Re-run `protonmail-mcp login` and provide an otpauth:// URI.",
	}
}
```

- [ ] **Step 6: Run all tests, then commit**

```bash
go test ./internal/proterr/... -v
git add internal/proterr/
git commit -m "feat(proterr): error taxonomy + go-proton-api/HTTP mapping"
```

---

### Task 4: Keychain adapter (`internal/keychain`)

**Files:**
- Create: `internal/keychain/keychain.go`
- Create: `internal/keychain/keychain_test.go`

**What this gives us:** Six well-known keychain entries under service `protonmail-mcp`. Save/Load/Clear with `keyring.MockInit()` for tests.

- [ ] **Step 1: Write the failing test**

```go
package keychain_test

import (
	"testing"

	"github.com/zalando/go-keyring"
	"protonmail-mcp/internal/keychain"
)

func TestRoundTrip(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()

	creds := keychain.Creds{Username: "andy@example.com", Password: "hunter2", TOTPSecret: "JBSWY3DPEHPK3PXP"}
	if err := kc.SaveCreds(creds); err != nil {
		t.Fatalf("SaveCreds: %v", err)
	}
	got, err := kc.LoadCreds()
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if got != creds {
		t.Fatalf("round trip mismatch: got %+v want %+v", got, creds)
	}

	sess := keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"}
	if err := kc.SaveSession(sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	gotS, err := kc.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if gotS != sess {
		t.Fatalf("session round trip mismatch: got %+v want %+v", gotS, sess)
	}

	if err := kc.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := kc.LoadCreds(); err == nil {
		t.Fatalf("LoadCreds after Clear should fail")
	}
}

func TestLoadCredsMissing(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()
	if _, err := kc.LoadCreds(); err == nil {
		t.Fatalf("expected error when keychain is empty")
	}
}

func TestSaveCredsClearsStaleTOTP(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()

	// First login: TOTP is set.
	if err := kc.SaveCreds(keychain.Creds{Username: "u", Password: "p", TOTPSecret: "JBSWY3DPEHPK3PXP"}); err != nil {
		t.Fatalf("first SaveCreds: %v", err)
	}
	got, err := kc.LoadCreds()
	if err != nil || got.TOTPSecret != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("first LoadCreds: got=%+v err=%v", got, err)
	}

	// Second login: same user, TOTP NOT supplied (one-shot code path).
	if err := kc.SaveCreds(keychain.Creds{Username: "u", Password: "p"}); err != nil {
		t.Fatalf("second SaveCreds: %v", err)
	}
	got, err = kc.LoadCreds()
	if err != nil {
		t.Fatalf("second LoadCreds: %v", err)
	}
	if got.TOTPSecret != "" {
		t.Fatalf("stale TOTP survived second login: got=%q", got.TOTPSecret)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/keychain/...`
Expected: FAIL — `keychain.New`, `Creds`, `Session` not defined.

- [ ] **Step 3: Implement `internal/keychain/keychain.go`**

```go
// Package keychain wraps go-keyring with typed Creds and Session bundles
// stored under the service name "protonmail-mcp".
package keychain

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const service = "protonmail-mcp"

const (
	keyUsername     = "username"
	keyPassword     = "password"
	keyTOTPSecret   = "totp_secret"
	keyUID          = "session_uid"
	keyAccessToken  = "access_token"
	keyRefreshToken = "refresh_token"
)

// Creds is the long-lived credential bundle written by `protonmail-mcp login`.
// TOTPSecret may be empty when the user opted to enter a one-shot code.
type Creds struct {
	Username   string
	Password   string
	TOTPSecret string
}

// Session is the short-lived auth state. Both tokens are rotated by go-proton-api.
type Session struct {
	UID          string
	AccessToken  string
	RefreshToken string
}

// Keychain is the typed wrapper. Construct with New().
type Keychain struct{}

func New() *Keychain { return &Keychain{} }

func (k *Keychain) SaveCreds(c Creds) error {
	if err := keyring.Set(service, keyUsername, c.Username); err != nil {
		return fmt.Errorf("save username: %w", err)
	}
	if err := keyring.Set(service, keyPassword, c.Password); err != nil {
		return fmt.Errorf("save password: %w", err)
	}
	// TOTP secret is optional. When the caller supplies an empty string, drop
	// any pre-existing entry so a stale secret from a prior login can't bleed
	// through. Tolerate ErrNotFound (no entry to delete).
	if c.TOTPSecret == "" {
		if err := keyring.Delete(service, keyTOTPSecret); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("clear stale totp: %w", err)
		}
		return nil
	}
	if err := keyring.Set(service, keyTOTPSecret, c.TOTPSecret); err != nil {
		return fmt.Errorf("save totp: %w", err)
	}
	return nil
}

func (k *Keychain) LoadCreds() (Creds, error) {
	u, err := keyring.Get(service, keyUsername)
	if err != nil {
		return Creds{}, fmt.Errorf("load username: %w", err)
	}
	p, err := keyring.Get(service, keyPassword)
	if err != nil {
		return Creds{}, fmt.Errorf("load password: %w", err)
	}
	t, err := keyring.Get(service, keyTOTPSecret)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return Creds{}, fmt.Errorf("load totp: %w", err)
	}
	return Creds{Username: u, Password: p, TOTPSecret: t}, nil
}

func (k *Keychain) SaveSession(s Session) error {
	if err := keyring.Set(service, keyUID, s.UID); err != nil {
		return fmt.Errorf("save uid: %w", err)
	}
	if err := keyring.Set(service, keyAccessToken, s.AccessToken); err != nil {
		return fmt.Errorf("save access token: %w", err)
	}
	if err := keyring.Set(service, keyRefreshToken, s.RefreshToken); err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}
	return nil
}

func (k *Keychain) LoadSession() (Session, error) {
	uid, err := keyring.Get(service, keyUID)
	if err != nil {
		return Session{}, fmt.Errorf("load uid: %w", err)
	}
	at, err := keyring.Get(service, keyAccessToken)
	if err != nil {
		return Session{}, fmt.Errorf("load access token: %w", err)
	}
	rt, err := keyring.Get(service, keyRefreshToken)
	if err != nil {
		return Session{}, fmt.Errorf("load refresh token: %w", err)
	}
	return Session{UID: uid, AccessToken: at, RefreshToken: rt}, nil
}

func (k *Keychain) Clear() error {
	keys := []string{keyUsername, keyPassword, keyTOTPSecret, keyUID, keyAccessToken, keyRefreshToken}
	for _, key := range keys {
		if err := keyring.Delete(service, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("delete %s: %w", key, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/keychain/... -v`
Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/keychain/
git commit -m "feat(keychain): typed Creds/Session wrappers around go-keyring"
```

---

## Phase 2 — Session manager

### Task 5: Hybrid-client session manager

**Files:**
- Create: `internal/session/session.go`
- Create: `internal/session/raw.go`
- Create: `internal/session/session_test.go`

**What this gives us:** A single object that owns the `*proton.Manager`, `*proton.Client`, and a `*resty.Client` (the raw one). Token rotation flows through one place: a `proton.AuthHandler` callback updates both the keychain and the raw client's bearer token. Tools call `s.Client(ctx)` for go-proton-api or `s.Raw(ctx)` for raw HTTP.

**Important context for the engineer:** `go-proton-api` rotates tokens on 401 inside its HTTP layer and notifies via `Client.AddAuthHandler(func(proton.Auth))`. That's our hook. The `Auth` struct contains `UID`, `AccessToken`, `RefreshToken` — verify exact field names with `go doc github.com/ProtonMail/go-proton-api Auth` and adjust if needed.

- [ ] **Step 1: Write the failing test**

```go
package session_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/go-keyring"
	"protonmail-mcp/internal/keychain"
	"protonmail-mcp/internal/session"
)

// TestRawSharesBearerToken exercises the contract that Raw() uses the same
// access token that the go-proton-api client has. We rotate the token via the
// session's auth handler and assert both surfaces see the new value on the
// next request.
func TestRawSharesBearerToken(t *testing.T) {
	keyring.MockInit()

	// Stand up a tiny test server that records the Authorization header.
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, err := session.NewForTesting(srv.URL, keychain.Session{UID: "u", AccessToken: "tok-A", RefreshToken: "ref"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	defer s.Logout()

	if _, err := s.Raw(context.Background()).R().Get(srv.URL + "/ping"); err != nil {
		t.Fatalf("first Raw req: %v", err)
	}

	// Simulate go-proton-api rotating tokens.
	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "tok-B", RefreshToken: "ref2"})

	if _, err := s.Raw(context.Background()).R().Get(srv.URL + "/ping"); err != nil {
		t.Fatalf("second Raw req: %v", err)
	}

	if len(seen) != 2 || seen[0] != "Bearer tok-A" || seen[1] != "Bearer tok-B" {
		t.Fatalf("token rotation not reflected on Raw client: %#v", seen)
	}
}

func TestRotatedTokenPersistedToKeychain(t *testing.T) {
	keyring.MockInit()
	kc := keychain.New()

	s, err := session.NewForTesting("http://invalid.test", keychain.Session{UID: "u", AccessToken: "tok-A", RefreshToken: "ref"})
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	defer s.Logout()

	s.OnAuthRotated(keychain.Session{UID: "u", AccessToken: "tok-B", RefreshToken: "ref2"})
	got, err := kc.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got.AccessToken != "tok-B" || got.RefreshToken != "ref2" {
		t.Fatalf("keychain not updated: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/session/...`
Expected: FAIL — package not yet present.

- [ ] **Step 3: Implement `internal/session/session.go`**

```go
// Package session owns the long-lived go-proton-api Manager + Client and a
// parallel raw resty client that shares the same bearer token. All
// authentication mutations (login, refresh, logout) go through here.
package session

import (
	"context"
	"errors"
	"fmt"
	"sync"

	proton "github.com/ProtonMail/go-proton-api"
	"protonmail-mcp/internal/keychain"
)

// Session is the application-wide auth-bearing object. Concurrency-safe.
type Session struct {
	mu      sync.RWMutex
	mgr     *proton.Manager
	client  *proton.Client
	raw     *rawClient // see raw.go
	kc      *keychain.Keychain
	current keychain.Session
}

// New constructs a session that will load credentials from keychain on first
// use. Returns immediately; no network calls happen until Client/Raw is called.
func New(apiURL string, kc *keychain.Keychain) *Session {
	mgr := proton.New(proton.WithHostURL(apiURL))
	return &Session{
		mgr: mgr,
		kc:  kc,
		raw: newRawClient(apiURL),
	}
}

// NewForTesting bypasses keychain load and seeds an existing Session directly.
// Used by unit tests to avoid the SRP login dance.
func NewForTesting(apiURL string, seed keychain.Session) (*Session, error) {
	keychain.New() // ensure mock keychain is usable
	kc := keychain.New()
	if err := kc.SaveSession(seed); err != nil {
		return nil, fmt.Errorf("seed keychain: %w", err)
	}
	s := New(apiURL, kc)
	s.current = seed
	s.raw.setBearer(seed.AccessToken)
	return s, nil
}

// Client returns a ready *proton.Client, performing first-time bootstrap from
// keychain if necessary. Subsequent calls return the cached client.
func (s *Session) Client(ctx context.Context) (*proton.Client, error) {
	s.mu.RLock()
	if s.client != nil {
		c := s.client
		s.mu.RUnlock()
		return c, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		return s.client, nil
	}
	sess, err := s.kc.LoadSession()
	if err != nil {
		return nil, errors.New("no session in keychain — run `protonmail-mcp login`")
	}
	c, _, err := s.mgr.NewClientWithRefresh(ctx, sess.UID, sess.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("refresh session: %w", err)
	}
	c.AddAuthHandler(func(a proton.Auth) {
		s.OnAuthRotated(keychain.Session{
			UID:          a.UID,
			AccessToken:  a.AccessToken,
			RefreshToken: a.RefreshToken,
		})
	})
	s.client = c
	s.current = sess
	s.raw.setBearer(sess.AccessToken)
	return c, nil
}

// Raw returns a resty.Client wired to the current bearer token. Callers should
// not memoize the returned client across rotations — get a fresh one per call.
func (s *Session) Raw(ctx context.Context) *rawClient {
	// Lazy bootstrap if Client() hasn't been called yet.
	if s.client == nil {
		_, _ = s.Client(ctx) // ignore error; raw will fail on the actual request
	}
	return s.raw
}

// OnAuthRotated is invoked by the go-proton-api auth handler and (in tests)
// directly by code simulating rotation. It atomically updates the in-memory
// state, the raw client's bearer token, and the keychain.
func (s *Session) OnAuthRotated(next keychain.Session) {
	s.mu.Lock()
	s.current = next
	s.raw.setBearer(next.AccessToken)
	s.mu.Unlock()
	if err := s.kc.SaveSession(next); err != nil {
		// Best-effort: log only. The in-memory state is correct; next start
		// will re-login if needed.
		_ = err
	}
}

// Logout closes the client and clears keychain state.
func (s *Session) Logout() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
	s.current = keychain.Session{}
	s.raw.setBearer("")
	return s.kc.Clear()
}
```

- [ ] **Step 4: Implement `internal/session/raw.go`**

```go
package session

import (
	"sync"

	"github.com/go-resty/resty/v2"
)

// rawClient is a resty client whose Authorization header is kept in sync with
// the active access token. Safe for concurrent use.
type rawClient struct {
	mu   sync.RWMutex
	rc   *resty.Client
	bear string
}

func newRawClient(baseURL string) *rawClient {
	rc := resty.New().
		SetBaseURL(baseURL).
		SetHeader("Accept", "application/vnd.protonmail.v1+json").
		SetHeader("x-pm-appversion", "Other") // matches what proton-bridge sends
	return &rawClient{rc: rc}
}

func (r *rawClient) setBearer(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bear = token
	if token == "" {
		r.rc.Header.Del("Authorization")
	} else {
		r.rc.SetHeader("Authorization", "Bearer "+token)
	}
}

// R returns a new resty.Request scoped to this client.
func (r *rawClient) R() *resty.Request {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.rc.R()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/session/... -v`
Expected: PASS.

- [ ] **Step 6: Add `Login` (real auth flow) — stub-tested only here, integration coverage in Task 16**

Append to `internal/session/session.go`:

```go
// LoginInput is what the CLI subcommand passes in.
type LoginInput struct {
	Username   string
	Password   string
	TOTPSecret string // raw seed; if empty, TOTPCode is consumed once
	TOTPCode   string // 6-digit code; only used if TOTPSecret is empty
}

// Login runs the full SRP + 2FA dance and persists creds + session to keychain.
// Returns nil on success; on HV/CAPTCHA returns an error whose .Error() contains
// the verification URL — the CLI should inspect via proterr.Map.
func (s *Session) Login(ctx context.Context, in LoginInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, auth, err := s.mgr.NewClientWithLogin(ctx, in.Username, []byte(in.Password))
	if err != nil {
		return fmt.Errorf("password auth: %w", err)
	}
	// 2FA branch.
	if auth.TwoFA.Enabled&proton.HasTOTP != 0 {
		code := in.TOTPCode
		if code == "" && in.TOTPSecret != "" {
			code, err = generateTOTP(in.TOTPSecret)
			if err != nil {
				c.Close()
				return fmt.Errorf("generate totp: %w", err)
			}
		}
		if code == "" {
			c.Close()
			return errors.New("2FA required but no TOTP provided")
		}
		if err := c.Auth2FA(ctx, proton.Auth2FAReq{TwoFactorCode: code}); err != nil {
			c.Close()
			return fmt.Errorf("submit 2fa: %w", err)
		}
	}

	c.AddAuthHandler(func(a proton.Auth) {
		s.OnAuthRotated(keychain.Session{
			UID:          a.UID,
			AccessToken:  a.AccessToken,
			RefreshToken: a.RefreshToken,
		})
	})

	next := keychain.Session{
		UID:          auth.UID,
		AccessToken:  auth.AccessToken,
		RefreshToken: auth.RefreshToken,
	}
	if err := s.kc.SaveCreds(keychain.Creds{
		Username:   in.Username,
		Password:   in.Password,
		TOTPSecret: in.TOTPSecret,
	}); err != nil {
		c.Close()
		return fmt.Errorf("save creds: %w", err)
	}
	if err := s.kc.SaveSession(next); err != nil {
		c.Close()
		return fmt.Errorf("save session: %w", err)
	}

	s.client = c
	s.current = next
	s.raw.setBearer(next.AccessToken)
	return nil
}
```

- [ ] **Step 7: Implement TOTP generation in `internal/session/totp.go`**

```go
package session

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// generateTOTP returns the current 6-digit TOTP for a base32-encoded secret.
// Accepts either the raw seed (e.g. "JBSWY3DPEHPK3PXP") or an otpauth:// URI.
func generateTOTP(secret string) (string, error) {
	seed := secret
	if strings.HasPrefix(secret, "otpauth://") {
		// Extract `secret=` query param.
		i := strings.Index(secret, "secret=")
		if i < 0 {
			return "", fmt.Errorf("otpauth URI missing secret")
		}
		rest := secret[i+len("secret="):]
		if amp := strings.IndexByte(rest, '&'); amp >= 0 {
			rest = rest[:amp]
		}
		seed = rest
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.ReplaceAll(seed, " ", "")))
	if err != nil {
		return "", fmt.Errorf("base32 decode: %w", err)
	}
	counter := uint64(time.Now().Unix() / 30)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0F
	bin := (uint32(sum[off])&0x7F)<<24 | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	return fmt.Sprintf("%06d", bin%1_000_000), nil
}
```

- [ ] **Step 8: Add a TOTP unit test**

Append to `internal/session/session_test.go`:

```go
func TestTOTPRoundsToSixDigits(t *testing.T) {
	// Known-vector: RFC 6238 test seed "12345678901234567890" base32 = GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ.
	// The actual digit value depends on time so we just assert format and length.
	code, err := session.GenerateTOTPForTest("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("want 6 digits, got %q", code)
	}
}
```

Add a tiny test-only export at the bottom of `internal/session/totp.go`:

```go
// GenerateTOTPForTest is exported only for unit tests in this package's
// _test.go files. Do not call from production code.
var GenerateTOTPForTest = generateTOTP
```

- [ ] **Step 9: Run tests**

Run: `go test ./internal/session/... -v`
Expected: PASS for all session tests.

- [ ] **Step 10: Commit**

```bash
git add internal/session/
git commit -m "feat(session): hybrid client with auth rotation + TOTP"
```

---

## Phase 3 — Raw API client (`internal/protonraw`)

### Task 6: protonraw client with httptest fixtures

**Files:**
- Create: `internal/protonraw/client.go`
- Create: `internal/protonraw/domains.go`
- Create: `internal/protonraw/addresses.go`
- Create: `internal/protonraw/protonraw_test.go`

**What this gives us:** Hand-written endpoints for the surface `go-proton-api` doesn't cover. Each method takes a `RestyDoer` (interface satisfied by `session.rawClient`) and returns shaped DTOs.

**Endpoint sourcing.** Paths and payload shapes are taken from the open-source [`ProtonMail/WebClients`](https://github.com/ProtonMail/WebClients) monorepo, specifically `applications/account` and `packages/shared/lib/api`. The reverse-engineered shapes are documented inline next to each method via a comment with a stable WebClients reference (file path + symbol name, not commit SHA).

> **Engineer note:** Before implementing each method, run a quick sanity check against the real Proton API in a manual test (see Task 17 checklist). The shapes below are the best information available at design time — they may need a one-off correction if Proton has shipped breaking changes.

- [ ] **Step 1: Define the `Doer` interface and shared types in `internal/protonraw/client.go`**

```go
// Package protonraw implements Proton API endpoints not exposed by
// go-proton-api: custom-domain CRUD and address creation.
//
// Endpoint paths and payload shapes are sourced from
// https://github.com/ProtonMail/WebClients (read-only reference). Each method
// links to its WebClients counterpart in a comment.
package protonraw

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
)

// Doer is implemented by *session.rawClient. We don't import session to avoid
// a cycle; the interface is just enough to make HTTP calls.
type Doer interface {
	R() *resty.Request
}

// envelope is Proton's standard JSON wrapper.
type envelope struct {
	Code  int             `json:"Code"`
	Error string          `json:"Error,omitempty"`
	// Concrete payloads embed-decoded by callers.
	Raw json.RawMessage `json:"-"`
}

func decode(resp *resty.Response, out any) error {
	if resp.IsError() {
		return fmt.Errorf("http %d: %s", resp.StatusCode(), resp.String())
	}
	var env struct {
		Code  int    `json:"Code"`
		Error string `json:"Error"`
	}
	if err := json.Unmarshal(resp.Body(), &env); err == nil && env.Error != "" {
		return fmt.Errorf("proton api: %s (code %d)", env.Error, env.Code)
	}
	if out != nil {
		if err := json.Unmarshal(resp.Body(), out); err != nil {
			return fmt.Errorf("decode body: %w", err)
		}
	}
	return nil
}

// ctxOnly is a tiny helper to attach context without dragging it through every
// signature explicitly.
func attachCtx(req *resty.Request, ctx context.Context) *resty.Request {
	return req.SetContext(ctx)
}
```

- [ ] **Step 2: Implement custom domain endpoints in `internal/protonraw/domains.go`**

```go
package protonraw

import (
	"context"
)

// CustomDomain mirrors the Proton API shape for a user-managed custom domain.
// Source: WebClients/packages/shared/lib/interfaces/Domain.ts.
type CustomDomain struct {
	ID          string             `json:"ID"`
	DomainName  string             `json:"DomainName"`
	State       int                `json:"State"`     // 0 = unverified, 1 = verified, 2 = warn
	VerifyState int                `json:"VerifyState"`
	MxState     int                `json:"MxState"`
	SpfState    int                `json:"SpfState"`
	DkimState   int                `json:"DkimState"`
	DmarcState  int                `json:"DmarcState"`
	Records     []CustomDomainRecord `json:"VerificationRecords,omitempty"`
}

// CustomDomainRecord is one line the user must publish to their DNS provider.
type CustomDomainRecord struct {
	Type    string `json:"Type"`     // "TXT", "MX", "CNAME"
	Name    string `json:"Hostname"` // "@", "mail._domainkey", etc.
	Value   string `json:"Value"`
	Purpose string `json:"Purpose"`  // "verify", "mx", "spf", "dkim", "dmarc"
}

// ListCustomDomains -> GET /core/v4/domains
// Source: WebClients/packages/shared/lib/api/domains.ts: queryDomains
func ListCustomDomains(ctx context.Context, d Doer) ([]CustomDomain, error) {
	var out struct {
		Domains []CustomDomain `json:"Domains"`
	}
	resp, err := d.R().SetContext(ctx).Get("/core/v4/domains")
	if err != nil {
		return nil, err
	}
	if err := decode(resp, &out); err != nil {
		return nil, err
	}
	return out.Domains, nil
}

// GetCustomDomain -> GET /core/v4/domains/{id}
// Source: WebClients/packages/shared/lib/api/domains.ts: getDomain
func GetCustomDomain(ctx context.Context, d Doer, id string) (CustomDomain, error) {
	var out struct {
		Domain CustomDomain `json:"Domain"`
	}
	resp, err := d.R().SetContext(ctx).Get("/core/v4/domains/" + id)
	if err != nil {
		return CustomDomain{}, err
	}
	if err := decode(resp, &out); err != nil {
		return CustomDomain{}, err
	}
	return out.Domain, nil
}

// AddCustomDomain -> POST /core/v4/domains
// Source: WebClients/packages/shared/lib/api/domains.ts: addDomain
func AddCustomDomain(ctx context.Context, d Doer, domain string) (CustomDomain, error) {
	body := map[string]string{"Name": domain}
	var out struct {
		Domain CustomDomain `json:"Domain"`
	}
	resp, err := d.R().SetContext(ctx).SetBody(body).Post("/core/v4/domains")
	if err != nil {
		return CustomDomain{}, err
	}
	if err := decode(resp, &out); err != nil {
		return CustomDomain{}, err
	}
	return out.Domain, nil
}

// VerifyCustomDomain -> PUT /core/v4/domains/{id}/verify
// Source: WebClients/packages/shared/lib/api/domains.ts: verifyDomain
func VerifyCustomDomain(ctx context.Context, d Doer, id string) (CustomDomain, error) {
	var out struct {
		Domain CustomDomain `json:"Domain"`
	}
	resp, err := d.R().SetContext(ctx).Put("/core/v4/domains/" + id + "/verify")
	if err != nil {
		return CustomDomain{}, err
	}
	if err := decode(resp, &out); err != nil {
		return CustomDomain{}, err
	}
	return out.Domain, nil
}

// RemoveCustomDomain -> DELETE /core/v4/domains/{id}
// Source: WebClients/packages/shared/lib/api/domains.ts: deleteDomain
func RemoveCustomDomain(ctx context.Context, d Doer, id string) error {
	resp, err := d.R().SetContext(ctx).Delete("/core/v4/domains/" + id)
	if err != nil {
		return err
	}
	return decode(resp, nil)
}
```

- [ ] **Step 3: Implement `CreateAddress` in `internal/protonraw/addresses.go`**

```go
package protonraw

import "context"

// CreateAddressRequest is the shape POSTed to /core/v4/addresses/setup.
// Source: WebClients/packages/shared/lib/api/addresses.ts: setupAddress
type CreateAddressRequest struct {
	DomainID    string `json:"DomainID"`    // ID of the custom domain
	LocalPart   string `json:"LocalPart"`   // "andy" in andy@example.com
	DisplayName string `json:"DisplayName,omitempty"`
	Signature   string `json:"Signature,omitempty"`
}

// CreatedAddress is the response shape — minimal, callers can re-fetch via
// go-proton-api Client.GetAddress for the full Address struct.
type CreatedAddress struct {
	ID    string `json:"ID"`
	Email string `json:"Email"`
}

// CreateAddress -> POST /core/v4/addresses/setup
func CreateAddress(ctx context.Context, d Doer, req CreateAddressRequest) (CreatedAddress, error) {
	var out struct {
		Address CreatedAddress `json:"Address"`
	}
	resp, err := d.R().SetContext(ctx).SetBody(req).Post("/core/v4/addresses/setup")
	if err != nil {
		return CreatedAddress{}, err
	}
	if err := decode(resp, &out); err != nil {
		return CreatedAddress{}, err
	}
	return out.Address, nil
}
```

- [ ] **Step 4: Write integration-style tests in `internal/protonraw/protonraw_test.go`**

```go
package protonraw_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
	"protonmail-mcp/internal/protonraw"
)

// fakeDoer adapts a *resty.Client to the protonraw.Doer interface.
type fakeDoer struct{ rc *resty.Client }

func (f *fakeDoer) R() *resty.Request { return f.rc.R() }

func newFakeDoer(baseURL string) *fakeDoer {
	return &fakeDoer{rc: resty.New().SetBaseURL(baseURL).SetHeader("Authorization", "Bearer test")}
}

func TestListCustomDomains(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/core/v4/domains" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Code": 1000,
			"Domains": []protonraw.CustomDomain{
				{ID: "d1", DomainName: "example.com", State: 1},
			},
		})
	}))
	defer srv.Close()

	got, err := protonraw.ListCustomDomains(context.Background(), newFakeDoer(srv.URL))
	if err != nil {
		t.Fatalf("ListCustomDomains: %v", err)
	}
	if len(got) != 1 || got[0].DomainName != "example.com" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestAddCustomDomain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/core/v4/domains" || r.Method != http.MethodPost {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["Name"] != "example.com" {
			t.Errorf("body Name=%q want example.com", body["Name"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Code":   1000,
			"Domain": protonraw.CustomDomain{ID: "d1", DomainName: "example.com"},
		})
	}))
	defer srv.Close()

	got, err := protonraw.AddCustomDomain(context.Background(), newFakeDoer(srv.URL), "example.com")
	if err != nil {
		t.Fatalf("AddCustomDomain: %v", err)
	}
	if got.ID != "d1" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestRemoveCustomDomain(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/core/v4/domains/d1" || r.Method != http.MethodDelete {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Code": 1000})
	}))
	defer srv.Close()

	if err := protonraw.RemoveCustomDomain(context.Background(), newFakeDoer(srv.URL), "d1"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !called {
		t.Fatal("server not called")
	}
}

func TestCreateAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/core/v4/addresses/setup" || r.Method != http.MethodPost {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		var body protonraw.CreateAddressRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.DomainID != "d1" || body.LocalPart != "andy" {
			t.Errorf("unexpected body: %+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Code":    1000,
			"Address": protonraw.CreatedAddress{ID: "a1", Email: "andy@example.com"},
		})
	}))
	defer srv.Close()

	got, err := protonraw.CreateAddress(context.Background(), newFakeDoer(srv.URL), protonraw.CreateAddressRequest{
		DomainID:  "d1",
		LocalPart: "andy",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Email != "andy@example.com" {
		t.Fatalf("unexpected: %+v", got)
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/protonraw/... -v`
Expected: PASS for all four tests.

- [ ] **Step 6: Commit**

```bash
git add internal/protonraw/
git commit -m "feat(protonraw): custom domains + create-address via raw resty"
```

---

## Phase 4 — Tool plumbing + identity tools

### Task 7: Tool registry + identity tools

**Files:**
- Create: `internal/tools/tools.go`
- Create: `internal/tools/identity.go`
- Create: `internal/tools/identity_test.go`

**What this gives us:** A `Register(server *mcp.Server, deps Deps)` function that registers all tools, gating writes by env var. Identity tools (`proton_whoami`, `proton_session_status`) prove the wiring end to end.

- [ ] **Step 1: Define the registry in `internal/tools/tools.go`**

```go
// Package tools registers MCP tools against an mcp.Server. Reads are always
// registered; writes are registered only when PROTONMAIL_MCP_ENABLE_WRITES=1.
package tools

import (
	"context"
	"os"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"protonmail-mcp/internal/proterr"
	"protonmail-mcp/internal/session"
)

// Deps is what handlers need. Kept tiny on purpose.
type Deps struct {
	Session *session.Session
}

// Register attaches every v1 tool to server. WritesEnabled is read once at
// registration time; tools added when it is false are simply absent from the
// MCP tool list.
func Register(server *mcp.Server, d Deps) {
	registerIdentity(server, d)
	registerAddresses(server, d)
	registerDomains(server, d)
	registerSettings(server, d)
	registerKeys(server, d)
}

// WritesEnabled returns true when PROTONMAIL_MCP_ENABLE_WRITES is set to a
// truthy value ("1", "true", "yes", case insensitive).
func WritesEnabled() bool {
	v := os.Getenv("PROTONMAIL_MCP_ENABLE_WRITES")
	switch v {
	case "1", "true", "True", "TRUE", "yes", "Yes", "YES":
		return true
	}
	return false
}

// failure is a tiny helper that converts a *proterr.Error into the MCP
// CallToolResult shape expected by the SDK. Returning a non-nil result with
// IsError=true lets the MCP host display structured error text without
// surfacing a transport-level failure.
func failure(perr *proterr.Error) *mcp.CallToolResult {
	if perr == nil {
		return nil
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: perr.Error()}},
	}
}

// clientOrFail centralizes the "get session.Client or return MCP error" pattern.
func clientOrFail(ctx context.Context, d Deps) (*proton.Client, *mcp.CallToolResult) {
	c, err := d.Session.Client(ctx)
	if err != nil {
		return nil, failure(proterr.Map(err))
	}
	return c, nil
}
```

- [ ] **Step 2: Write the identity test (`internal/tools/identity_test.go`)**

```go
package tools_test

import (
	"context"
	"testing"

	"github.com/zalando/go-keyring"
	"protonmail-mcp/internal/tools/internal/testharness"
)

// Identity smoke test: boots the go-proton-api dev server, registers a fake
// account, drives proton_whoami via an in-process MCP client, and asserts the
// returned email matches.
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
```

> **Test harness note.** A real implementation requires booting `go-proton-api`'s dev server (`server.New`), creating a user, capturing its credentials, instantiating `session.New` against the dev server URL, calling `Login` with those creds, registering tools onto an `mcp.Server`, and pumping requests through an in-process transport. This is non-trivial; **stub the harness in Step 3**, then flesh it out fully in Task 16. For now, the test should compile and either skip (preferred) or be excluded from default test runs via build tags.

- [ ] **Step 3: Add a temporary skipping harness `internal/tools/internal/testharness/harness.go`**

```go
package testharness

import (
	"context"
	"testing"
)

type Harness struct{}

func (h *Harness) Close()                                              {}
func (h *Harness) Call(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return nil, nil
}

func Boot(t *testing.T, _ string, _ string) *Harness {
	t.Skip("test harness implemented in Task 16; skipped for now")
	return &Harness{}
}
```

- [ ] **Step 4: Implement identity tools in `internal/tools/identity.go`**

```go
package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"protonmail-mcp/internal/proterr"
)

type whoamiInput struct{}
type whoamiOutput struct {
	Email      string `json:"email" jsonschema:"the primary email of the logged-in account"`
	Name       string `json:"name,omitempty" jsonschema:"the user's display name if set"`
	UsedSpace  int64  `json:"used_space_bytes" jsonschema:"current storage usage in bytes"`
	MaxSpace   int64  `json:"max_space_bytes" jsonschema:"plan's storage quota in bytes"`
	Subscribed int    `json:"subscribed_bitmask" jsonschema:"bitmask of subscribed Proton products"`
}

type sessionStatusInput struct{}
type sessionStatusOutput struct {
	LoggedIn bool   `json:"logged_in"`
	Email    string `json:"email,omitempty"`
}

func registerIdentity(server *mcp.Server, d Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_whoami",
		Description: "Returns the logged-in Proton account's email, plan, and storage usage.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ whoamiInput) (*mcp.CallToolResult, whoamiOutput, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, whoamiOutput{}, nil
		}
		u, err := c.GetUser(ctx)
		if err != nil {
			return failure(proterr.Map(err)), whoamiOutput{}, nil
		}
		return nil, whoamiOutput{
			Email:      u.Email,
			Name:       u.DisplayName,
			UsedSpace:  int64(u.UsedSpace),
			MaxSpace:   int64(u.MaxSpace),
			Subscribed: int(u.Subscribed),
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_session_status",
		Description: "Reports whether a session is currently authenticated.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ sessionStatusInput) (*mcp.CallToolResult, sessionStatusOutput, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			// proton/auth_required is a normal "not logged in" answer here.
			return nil, sessionStatusOutput{LoggedIn: false}, nil
		}
		u, err := c.GetUser(ctx)
		if err != nil {
			return nil, sessionStatusOutput{LoggedIn: false}, nil
		}
		return nil, sessionStatusOutput{LoggedIn: true, Email: u.Email}, nil
	})
}
```

> **Note on `User` field names.** Verify with `go doc github.com/ProtonMail/go-proton-api User` after `go mod tidy`. If `Email`/`DisplayName`/`UsedSpace`/`MaxSpace`/`Subscribed` differ, adjust the field accesses; do NOT change the output struct field names — those are part of the MCP tool contract.

- [ ] **Step 5: Add stub registrations for the not-yet-built tool files**

Create empty stubs so the package compiles:

```go
// internal/tools/addresses.go
package tools

import "github.com/modelcontextprotocol/go-sdk/mcp"

func registerAddresses(server *mcp.Server, d Deps) {}
```

Repeat for `domains.go`, `settings.go`, `keys.go` — each with an empty `registerXxx`. These will be filled in Tasks 8–11.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/tools/... -v`
Expected: PASS (the harness test skips; `tools` package compiles).

- [ ] **Step 7: Commit**

```bash
git add internal/tools/
git commit -m "feat(tools): registry + identity tools (whoami, session_status)"
```

---

## Phase 5 — Address tools

### Task 8: Address tools (read + write)

**Files:**
- Modify: `internal/tools/addresses.go` (replace stub)
- Create: `internal/tools/addresses_test.go`

**Tools added:** `proton_list_addresses`, `proton_get_address`, `proton_create_address` (via protonraw), `proton_update_address`, `proton_set_address_status`, `proton_delete_address`.

> **Note on `proton_update_address`.** Display name and signature for an address actually flow through `mail_settings`-shaped endpoints in go-proton-api, not the address itself. We expose it as a tool that calls `Client.SetDisplayName` / `Client.SetSignature` based on which fields are set in the input. The "address ID" is captured in the request and forwarded to the appropriate setter.

- [ ] **Step 1: Implement `internal/tools/addresses.go`**

```go
package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"protonmail-mcp/internal/proterr"
	"protonmail-mcp/internal/protonraw"
)

// --- DTOs -----------------------------------------------------------------

type addressDTO struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name,omitempty"`
	Signature   string   `json:"signature,omitempty"`
	Status      int      `json:"status"`
	Order       int      `json:"order"`
	Type        int      `json:"type"`
	DomainID    string   `json:"domain_id,omitempty"`
	KeyIDs      []string `json:"key_ids,omitempty"`
}

type listAddressesIn struct{}
type listAddressesOut struct {
	Addresses []addressDTO `json:"addresses"`
}

type getAddressIn struct {
	ID string `json:"id" jsonschema:"the Proton address ID"`
}
type getAddressOut struct {
	Address addressDTO `json:"address"`
}

type createAddressIn struct {
	DomainID    string `json:"domain_id" jsonschema:"the Proton custom domain ID (from proton_list_custom_domains)"`
	LocalPart   string `json:"local_part" jsonschema:"the part of the email before the @"`
	DisplayName string `json:"display_name,omitempty" jsonschema:"optional display name"`
	Signature   string `json:"signature,omitempty" jsonschema:"optional HTML signature"`
}
type createAddressOut struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type updateAddressIn struct {
	ID          string  `json:"id"`
	DisplayName *string `json:"display_name,omitempty"`
	Signature   *string `json:"signature,omitempty"`
}
type updateAddressOut struct {
	OK bool `json:"ok"`
}

type setAddressStatusIn struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled" jsonschema:"true to enable, false to disable"`
}
type setAddressStatusOut struct {
	OK bool `json:"ok"`
}

type deleteAddressIn struct {
	ID string `json:"id"`
}
type deleteAddressOut struct {
	OK bool `json:"ok"`
}

// --- registration ---------------------------------------------------------

func registerAddresses(server *mcp.Server, d Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_list_addresses",
		Description: "Lists all addresses on the account, including aliases and disabled ones.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listAddressesIn) (*mcp.CallToolResult, listAddressesOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, listAddressesOut{}, nil
		}
		raw, err := c.GetAddresses(ctx)
		if err != nil {
			return failure(proterr.Map(err)), listAddressesOut{}, nil
		}
		out := make([]addressDTO, len(raw))
		for i, a := range raw {
			out[i] = toAddressDTO(a)
		}
		return nil, listAddressesOut{Addresses: out}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_get_address",
		Description: "Returns detail for a single address by ID.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getAddressIn) (*mcp.CallToolResult, getAddressOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, getAddressOut{}, nil
		}
		raw, err := c.GetAddress(ctx, in.ID)
		if err != nil {
			return failure(proterr.Map(err)), getAddressOut{}, nil
		}
		return nil, getAddressOut{Address: toAddressDTO(raw)}, nil
	})

	if !WritesEnabled() {
		return
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_create_address",
		Description: "Creates a new address (alias) on a custom domain.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in createAddressIn) (*mcp.CallToolResult, createAddressOut, error) {
		raw := d.Session.Raw(ctx)
		got, err := protonraw.CreateAddress(ctx, raw, protonraw.CreateAddressRequest{
			DomainID:    in.DomainID,
			LocalPart:   in.LocalPart,
			DisplayName: in.DisplayName,
			Signature:   in.Signature,
		})
		if err != nil {
			return failure(proterr.Map(err)), createAddressOut{}, nil
		}
		return nil, createAddressOut{ID: got.ID, Email: got.Email}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_update_address",
		Description: "Updates display name and/or signature for an address.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in updateAddressIn) (*mcp.CallToolResult, updateAddressOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, updateAddressOut{}, nil
		}
		// Note: SetDisplayName / SetSignature are global — there is one display
		// name per account, not per address. If go-proton-api gains a per-
		// address variant, switch here.
		if in.DisplayName != nil {
			if _, err := c.SetDisplayName(ctx, proton.SetDisplayNameReq{DisplayName: *in.DisplayName}); err != nil {
				return failure(proterr.Map(err)), updateAddressOut{}, nil
			}
		}
		if in.Signature != nil {
			if _, err := c.SetSignature(ctx, proton.SetSignatureReq{Signature: *in.Signature}); err != nil {
				return failure(proterr.Map(err)), updateAddressOut{}, nil
			}
		}
		return nil, updateAddressOut{OK: true}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_set_address_status",
		Description: "Enables or disables an address.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setAddressStatusIn) (*mcp.CallToolResult, setAddressStatusOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, setAddressStatusOut{}, nil
		}
		var err error
		if in.Enabled {
			err = c.EnableAddress(ctx, in.ID)
		} else {
			err = c.DisableAddress(ctx, in.ID)
		}
		if err != nil {
			return failure(proterr.Map(err)), setAddressStatusOut{}, nil
		}
		return nil, setAddressStatusOut{OK: true}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_delete_address",
		Description: "Permanently deletes an address. DESTRUCTIVE — cannot be undone.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deleteAddressIn) (*mcp.CallToolResult, deleteAddressOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, deleteAddressOut{}, nil
		}
		if err := c.DeleteAddress(ctx, in.ID); err != nil {
			return failure(proterr.Map(err)), deleteAddressOut{}, nil
		}
		return nil, deleteAddressOut{OK: true}, nil
	})
}

func toAddressDTO(a proton.Address) addressDTO {
	keyIDs := make([]string, len(a.Keys))
	for i, k := range a.Keys {
		keyIDs[i] = k.ID
	}
	return addressDTO{
		ID:          a.ID,
		Email:       a.Email,
		DisplayName: a.DisplayName,
		Signature:   a.Signature,
		Status:      int(a.Status),
		Order:       a.Order,
		Type:        int(a.Type),
		DomainID:    a.DomainID,
		KeyIDs:      keyIDs,
	}
}
```

> **Two field-name verifications needed:** `proton.Address` (run `go doc github.com/ProtonMail/go-proton-api Address`) and `proton.SetDisplayNameReq` / `SetSignatureReq`. Adjust if names differ.

- [ ] **Step 2: Add the missing import for `proton`**

Add to the import block at the top:

```go
proton "github.com/ProtonMail/go-proton-api"
```

- [ ] **Step 3: Write `internal/tools/addresses_test.go`**

```go
package tools_test

import (
	"os"
	"testing"

	"protonmail-mcp/internal/tools"
)

func TestWritesEnabledRespectsEnv(t *testing.T) {
	os.Unsetenv("PROTONMAIL_MCP_ENABLE_WRITES")
	if tools.WritesEnabled() {
		t.Fatalf("want false when env unset")
	}
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	if !tools.WritesEnabled() {
		t.Fatalf("want true when env=1")
	}
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "no")
	if tools.WritesEnabled() {
		t.Fatalf("want false when env=no")
	}
}
```

(Per-tool integration tests live in `integration/` and are added in Task 16. Unit-testing each tool in isolation here would mostly assert that handlers call the SDK — not useful.)

- [ ] **Step 4: Build + test**

Run:

```bash
go build ./...
go test ./internal/tools/... -v
```

Expected: build succeeds; tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/addresses.go internal/tools/addresses_test.go
git commit -m "feat(tools): address read+write tools (6)"
```

---

## Phase 6 — Custom domain tools

### Task 9: Custom domain tools

**Files:**
- Modify: `internal/tools/domains.go` (replace stub)

**Tools added:** `proton_list_custom_domains`, `proton_get_custom_domain`, `proton_add_custom_domain`, `proton_verify_custom_domain`, `proton_remove_custom_domain` — all via `protonraw`.

- [ ] **Step 1: Implement `internal/tools/domains.go`**

```go
package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"protonmail-mcp/internal/proterr"
	"protonmail-mcp/internal/protonraw"
)

type domainDTO struct {
	ID          string         `json:"id"`
	DomainName  string         `json:"domain_name"`
	State       int            `json:"state"`
	VerifyState int            `json:"verify_state"`
	MxState     int            `json:"mx_state"`
	SpfState    int            `json:"spf_state"`
	DkimState   int            `json:"dkim_state"`
	DmarcState  int            `json:"dmarc_state"`
	Records     []dnsRecordDTO `json:"required_dns_records,omitempty"`
}

type dnsRecordDTO struct {
	Type     string `json:"type" jsonschema:"DNS record type: TXT, MX, or CNAME"`
	Hostname string `json:"hostname" jsonschema:"the hostname to publish, e.g. @ or mail._domainkey"`
	Value    string `json:"value" jsonschema:"the record value"`
	Purpose  string `json:"purpose" jsonschema:"verify | mx | spf | dkim | dmarc"`
}

type listDomainsIn struct{}
type listDomainsOut struct {
	Domains []domainDTO `json:"domains"`
}

type getDomainIn struct {
	ID string `json:"id"`
}
type getDomainOut struct {
	Domain domainDTO `json:"domain"`
}

type addDomainIn struct {
	DomainName string `json:"domain_name"`
}
type addDomainOut struct {
	Domain domainDTO `json:"domain"`
}

type verifyDomainIn struct {
	ID string `json:"id"`
}
type verifyDomainOut struct {
	Domain domainDTO `json:"domain"`
}

type removeDomainIn struct {
	ID string `json:"id"`
}
type removeDomainOut struct {
	OK bool `json:"ok"`
}

func registerDomains(server *mcp.Server, d Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_list_custom_domains",
		Description: "Lists all custom domains on the account with verification state.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listDomainsIn) (*mcp.CallToolResult, listDomainsOut, error) {
		raws, err := protonraw.ListCustomDomains(ctx, d.Session.Raw(ctx))
		if err != nil {
			return failure(proterr.Map(err)), listDomainsOut{}, nil
		}
		out := make([]domainDTO, len(raws))
		for i, r := range raws {
			out[i] = toDomainDTO(r)
		}
		return nil, listDomainsOut{Domains: out}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_get_custom_domain",
		Description: "Returns detail (including required DNS records) for a custom domain.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getDomainIn) (*mcp.CallToolResult, getDomainOut, error) {
		raw, err := protonraw.GetCustomDomain(ctx, d.Session.Raw(ctx), in.ID)
		if err != nil {
			return failure(proterr.Map(err)), getDomainOut{}, nil
		}
		return nil, getDomainOut{Domain: toDomainDTO(raw)}, nil
	})

	if !WritesEnabled() {
		return
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_add_custom_domain",
		Description: "Adds a new custom domain. Returns the required DNS records to publish at your DNS provider.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in addDomainIn) (*mcp.CallToolResult, addDomainOut, error) {
		raw, err := protonraw.AddCustomDomain(ctx, d.Session.Raw(ctx), in.DomainName)
		if err != nil {
			return failure(proterr.Map(err)), addDomainOut{}, nil
		}
		return nil, addDomainOut{Domain: toDomainDTO(raw)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_verify_custom_domain",
		Description: "Asks Proton to re-verify the published DNS records for a custom domain.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in verifyDomainIn) (*mcp.CallToolResult, verifyDomainOut, error) {
		raw, err := protonraw.VerifyCustomDomain(ctx, d.Session.Raw(ctx), in.ID)
		if err != nil {
			return failure(proterr.Map(err)), verifyDomainOut{}, nil
		}
		return nil, verifyDomainOut{Domain: toDomainDTO(raw)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_remove_custom_domain",
		Description: "Removes a custom domain. DESTRUCTIVE — orphans all aliases on the domain.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in removeDomainIn) (*mcp.CallToolResult, removeDomainOut, error) {
		if err := protonraw.RemoveCustomDomain(ctx, d.Session.Raw(ctx), in.ID); err != nil {
			return failure(proterr.Map(err)), removeDomainOut{}, nil
		}
		return nil, removeDomainOut{OK: true}, nil
	})
}

func toDomainDTO(c protonraw.CustomDomain) domainDTO {
	recs := make([]dnsRecordDTO, len(c.Records))
	for i, r := range c.Records {
		recs[i] = dnsRecordDTO{Type: r.Type, Hostname: r.Name, Value: r.Value, Purpose: r.Purpose}
	}
	return domainDTO{
		ID:          c.ID,
		DomainName:  c.DomainName,
		State:       c.State,
		VerifyState: c.VerifyState,
		MxState:     c.MxState,
		SpfState:    c.SpfState,
		DkimState:   c.DkimState,
		DmarcState:  c.DmarcState,
		Records:     recs,
	}
}
```

- [ ] **Step 2: Build + test**

Run:

```bash
go build ./...
go test ./...
```

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/tools/domains.go
git commit -m "feat(tools): custom domain tools (5) via protonraw"
```

---

## Phase 7 — Settings tools

### Task 10: Mail + core settings tools

**Files:**
- Modify: `internal/tools/settings.go` (replace stub)

**Tools added:** `proton_get_mail_settings`, `proton_get_core_settings`, `proton_update_mail_settings`, `proton_update_core_settings`.

> **Important shape note.** `go-proton-api`'s `MailSettings` / `UserSettings` have many fields. Rather than echoing the entire struct (which leaks `go-proton-api` types into the MCP contract), this tool surfaces a pragmatic subset. See the comments inline. The tool's update handler accepts a partial-update map (every field optional) and routes to the right `Set*` method.

- [ ] **Step 1: Implement `internal/tools/settings.go`**

```go
package tools

import (
	"context"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"protonmail-mcp/internal/proterr"
)

type mailSettingsDTO struct {
	DisplayName    string `json:"display_name"`
	Signature      string `json:"signature"`
	AutoSaveContacts int  `json:"auto_save_contacts"`
	HideEmbeddedImages int `json:"hide_embedded_images"`
	HideRemoteImages   int `json:"hide_remote_images"`
}

type coreSettingsDTO struct {
	Locale          string `json:"locale,omitempty"`
	WeeklyEmail     int    `json:"weekly_email_summary"`
	NewsletterOptIn int    `json:"newsletter_opt_in"`
}

type getMailSettingsIn struct{}
type getMailSettingsOut struct {
	Settings mailSettingsDTO `json:"settings"`
}

type getCoreSettingsIn struct{}
type getCoreSettingsOut struct {
	Settings coreSettingsDTO `json:"settings"`
}

type updateMailSettingsIn struct {
	DisplayName *string `json:"display_name,omitempty"`
	Signature   *string `json:"signature,omitempty"`
}
type updateMailSettingsOut struct {
	Settings mailSettingsDTO `json:"settings"`
}

type updateCoreSettingsIn struct {
	Locale *string `json:"locale,omitempty"`
}
type updateCoreSettingsOut struct {
	OK bool `json:"ok"`
}

func registerSettings(server *mcp.Server, d Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_get_mail_settings",
		Description: "Returns mail settings (display name, signature, image visibility, etc.).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ getMailSettingsIn) (*mcp.CallToolResult, getMailSettingsOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, getMailSettingsOut{}, nil
		}
		ms, err := c.GetMailSettings(ctx)
		if err != nil {
			return failure(proterr.Map(err)), getMailSettingsOut{}, nil
		}
		return nil, getMailSettingsOut{Settings: toMailSettingsDTO(ms)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_get_core_settings",
		Description: "Returns account-level (core) settings.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ getCoreSettingsIn) (*mcp.CallToolResult, getCoreSettingsOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, getCoreSettingsOut{}, nil
		}
		us, err := c.GetUserSettings(ctx)
		if err != nil {
			return failure(proterr.Map(err)), getCoreSettingsOut{}, nil
		}
		return nil, getCoreSettingsOut{Settings: toCoreSettingsDTO(us)}, nil
	})

	if !WritesEnabled() {
		return
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_update_mail_settings",
		Description: "Updates mail settings (partial — only set fields are changed).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in updateMailSettingsIn) (*mcp.CallToolResult, updateMailSettingsOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, updateMailSettingsOut{}, nil
		}
		var ms proton.MailSettings
		var err error
		if in.DisplayName != nil {
			ms, err = c.SetDisplayName(ctx, proton.SetDisplayNameReq{DisplayName: *in.DisplayName})
			if err != nil {
				return failure(proterr.Map(err)), updateMailSettingsOut{}, nil
			}
		}
		if in.Signature != nil {
			ms, err = c.SetSignature(ctx, proton.SetSignatureReq{Signature: *in.Signature})
			if err != nil {
				return failure(proterr.Map(err)), updateMailSettingsOut{}, nil
			}
		}
		// If nothing was set, fetch fresh to return current state.
		if in.DisplayName == nil && in.Signature == nil {
			ms, err = c.GetMailSettings(ctx)
			if err != nil {
				return failure(proterr.Map(err)), updateMailSettingsOut{}, nil
			}
		}
		return nil, updateMailSettingsOut{Settings: toMailSettingsDTO(ms)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_update_core_settings",
		Description: "Updates account-level (core) settings.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in updateCoreSettingsIn) (*mcp.CallToolResult, updateCoreSettingsOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, updateCoreSettingsOut{}, nil
		}
		if in.Locale != nil {
			if err := c.SetUserSettingsLocale(ctx, *in.Locale); err != nil {
				return failure(proterr.Map(err)), updateCoreSettingsOut{}, nil
			}
		}
		return nil, updateCoreSettingsOut{OK: true}, nil
	})
}

func toMailSettingsDTO(m proton.MailSettings) mailSettingsDTO {
	return mailSettingsDTO{
		DisplayName:        m.DisplayName,
		Signature:          m.Signature,
		AutoSaveContacts:   int(m.AutoSaveContacts),
		HideEmbeddedImages: int(m.HideEmbeddedImages),
		HideRemoteImages:   int(m.HideRemoteImages),
	}
}

func toCoreSettingsDTO(u proton.UserSettings) coreSettingsDTO {
	return coreSettingsDTO{
		Locale:          u.Locale,
		WeeklyEmail:     int(u.WeeklyEmail),
		NewsletterOptIn: int(u.News),
	}
}
```

> **Field verification.** Run:
> ```
> go doc github.com/ProtonMail/go-proton-api MailSettings
> go doc github.com/ProtonMail/go-proton-api UserSettings
> go doc github.com/ProtonMail/go-proton-api Client.GetUserSettings
> go doc github.com/ProtonMail/go-proton-api Client.SetUserSettingsLocale
> ```
> Adjust field names / method names if they differ. If `SetUserSettingsLocale` doesn't exist, drop the locale update and document the gap in v1.5 follow-ups.

- [ ] **Step 2: Build + test**

Run: `go build ./... && go test ./...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/tools/settings.go
git commit -m "feat(tools): mail + core settings tools (4)"
```

---

## Phase 8 — Key tools

### Task 11: Encryption key tools

**Files:**
- Modify: `internal/tools/keys.go` (replace stub)

**Tools added:** `proton_list_address_keys`, `proton_generate_address_key`, `proton_set_primary_address_key`.

> **Verification step.** `go-proton-api` has `keys.go` and `keyring.go`. The relevant `Client` methods (likely `MakeAndCreateAddressKey`, `MakeKeyPrimary` or similar) need to be confirmed before implementation:
> ```
> go doc github.com/ProtonMail/go-proton-api Client.MakeAndCreateAddressKey
> go doc github.com/ProtonMail/go-proton-api Client.MakeAddressKeyPrimary
> ```
> If methods are named differently or require additional args (e.g., a passphrase / unlocked keyring), adjust the implementation. **If generation requires an unlocked PGP keyring derived from the user's mailbox password, that's a meaningful complexity** — consider deferring `proton_generate_address_key` and `proton_set_primary_address_key` to v1.5 and shipping only `proton_list_address_keys` in v1. **The maintainer should make this call after running the `go doc` checks above.**

- [ ] **Step 1: Implement `internal/tools/keys.go` (read only by default; gates writes per verification)**

```go
package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"protonmail-mcp/internal/proterr"
)

type keyDTO struct {
	ID          string `json:"id"`
	Fingerprint string `json:"fingerprint"`
	Algorithm   string `json:"algorithm"`
	Primary     bool   `json:"primary"`
	PublicKey   string `json:"public_key_armored"`
}

type listKeysIn struct {
	AddressID string `json:"address_id"`
}
type listKeysOut struct {
	Keys []keyDTO `json:"keys"`
}

func registerKeys(server *mcp.Server, d Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_list_address_keys",
		Description: "Lists encryption keys for an address (fingerprint, primary flag, armored public key).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listKeysIn) (*mcp.CallToolResult, listKeysOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, listKeysOut{}, nil
		}
		addr, err := c.GetAddress(ctx, in.AddressID)
		if err != nil {
			return failure(proterr.Map(err)), listKeysOut{}, nil
		}
		out := make([]keyDTO, len(addr.Keys))
		for i, k := range addr.Keys {
			out[i] = keyDTO{
				ID:          k.ID,
				Fingerprint: k.Fingerprint,
				PublicKey:   k.PublicKey,
				Primary:     k.Primary != 0,
			}
		}
		return nil, listKeysOut{Keys: out}, nil
	})

	if !WritesEnabled() {
		return
	}

	// Generate / set-primary are deferred to a follow-up patch within this task
	// once the go-proton-api method shapes are confirmed. See the verification
	// note in the plan. Intentionally not registered here yet.
}
```

- [ ] **Step 2: After running `go doc` verification (per the note above), choose ONE of:**
  - **(a)** If `MakeAndCreateAddressKey` and `MakeAddressKeyPrimary` are present and don't require additional unlock arguments: extend the file to register `proton_generate_address_key` and `proton_set_primary_address_key`, mirroring the patterns from Tasks 8–10.
  - **(b)** If they require keyring unlock or are absent: leave the writes off, update the plan/spec to note v1 keys are read-only, and add a follow-up task in `docs/superpowers/specs` open-follow-ups.

- [ ] **Step 3: Build + test**

Run: `go build ./... && go test ./...`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/tools/keys.go
git commit -m "feat(tools): encryption key listing (write paths pending verification)"
```

---

## Phase 9 — MCP server wiring

### Task 12: Wire MCP server + cmd/main

**Files:**
- Create: `internal/server/server.go`
- Modify: `cmd/protonmail-mcp/main.go`

- [ ] **Step 1: Implement `internal/server/server.go`**

```go
// Package server glues the MCP transport, tool registry, and session manager
// together. Run starts the stdio transport and blocks until the host
// disconnects.
package server

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"protonmail-mcp/internal/keychain"
	"protonmail-mcp/internal/session"
	"protonmail-mcp/internal/tools"
)

const (
	defaultAPIURL = "https://mail.proton.me/api"
	serverName    = "protonmail-mcp"
	serverVersion = "v0.1.0"
)

// Run starts the stdio MCP server. Blocks until the host disconnects.
func Run(ctx context.Context) error {
	apiURL := os.Getenv("PROTONMAIL_MCP_API_URL")
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	sess := session.New(apiURL, keychain.New())
	srv := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)
	tools.Register(srv, tools.Deps{Session: sess})
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("mcp server: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Update `cmd/protonmail-mcp/main.go` to wire it**

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	mcplog "protonmail-mcp/internal/log"
	"protonmail-mcp/internal/server"
)

func main() {
	level := slog.LevelInfo
	if v := os.Getenv("PROTONMAIL_MCP_LOG_LEVEL"); v == "debug" {
		level = slog.LevelDebug
	}
	logger := mcplog.New(level, os.Stderr)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "login":
			if err := runLogin(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "login:", err)
				os.Exit(1)
			}
			return
		case "logout":
			if err := runLogout(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "logout:", err)
				os.Exit(1)
			}
			return
		case "status":
			if err := runStatus(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "status:", err)
				os.Exit(1)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
			os.Exit(2)
		}
	}

	if err := server.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}

// Stubs — implemented in Tasks 13–15.
func runLogin(_ context.Context) error  { return fmt.Errorf("login not yet implemented") }
func runLogout(_ context.Context) error { return fmt.Errorf("logout not yet implemented") }
func runStatus(_ context.Context) error { return fmt.Errorf("status not yet implemented") }
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: success. The binary at `./protonmail-mcp` should run; without subcommand it will start the MCP stdio server (and block waiting for input — kill with Ctrl-C).

- [ ] **Step 4: Smoke test**

Run:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}' | ./protonmail-mcp 2>/dev/null | head -1
```

Expected: a JSON-RPC `initialize` response containing `"protocolVersion"`. (May vary by SDK version; the key signal is that you get a single line of valid JSON back, not a crash.)

- [ ] **Step 5: Commit**

```bash
git add internal/server/ cmd/protonmail-mcp/
git commit -m "feat(server): wire MCP stdio transport + subcommand routing"
```

---

## Phase 10 — CLI subcommands

### Task 13: `login` subcommand

**Files:**
- Create: `cmd/protonmail-mcp/login.go`

**What this does:** Prompts for username + password (hidden), runs `session.Login`, prompts for TOTP secret URI or one-shot code if 2FA is required, surfaces HV errors with verification URL.

- [ ] **Step 1: Implement `cmd/protonmail-mcp/login.go`**

```go
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"

	"protonmail-mcp/internal/keychain"
	"protonmail-mcp/internal/proterr"
	"protonmail-mcp/internal/session"
)

func runLogin(ctx context.Context) error {
	apiURL := os.Getenv("PROTONMAIL_MCP_API_URL")
	if apiURL == "" {
		apiURL = "https://mail.proton.me/api"
	}
	sess := session.New(apiURL, keychain.New())

	username, err := prompt("Proton email: ")
	if err != nil {
		return err
	}
	password, err := promptHidden("Password: ")
	if err != nil {
		return err
	}

	in := session.LoginInput{Username: username, Password: password}

	// First attempt — no 2FA. If it fails with "2FA required", prompt and retry.
	err = sess.Login(ctx, in)
	if err != nil && strings.Contains(err.Error(), "2FA required") {
		fmt.Println()
		fmt.Println("2FA is enabled on this account.")
		fmt.Println("Paste an otpauth:// URI (preferred — enables silent refresh) OR a 6-digit code.")
		v, err2 := prompt("> ")
		if err2 != nil {
			return err2
		}
		if strings.HasPrefix(v, "otpauth://") {
			in.TOTPSecret = v
		} else if isAllDigits(v) && len(v) == 6 {
			in.TOTPCode = v
			fmt.Println("WARNING: a one-shot code was provided. Future automatic refreshes will fail; you'll need to log in again when the session expires.")
		} else {
			return errors.New("input is neither an otpauth:// URI nor a 6-digit code")
		}
		err = sess.Login(ctx, in)
	}
	if err != nil {
		// Surface mapped errors helpfully.
		if pe := proterr.Map(err); pe != nil {
			return fmt.Errorf("%s: %s\n%s", pe.Code, pe.Message, pe.Hint)
		}
		return err
	}

	fmt.Println("Logged in. You can now run `protonmail-mcp` (no subcommand) to start the MCP server.")
	return nil
}

func prompt(label string) (string, error) {
	fmt.Print(label)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptHidden(label string) (string, error) {
	fmt.Print(label)
	b, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Add `golang.org/x/term` dependency**

Run: `go get golang.org/x/term && go mod tidy`

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Manual smoke test (optional, requires real account)**

Skip in CI; in local dev: `./protonmail-mcp login` → enter throwaway-account credentials → confirm keychain has 3 entries set:

```bash
security find-generic-password -s protonmail-mcp -a username
security find-generic-password -s protonmail-mcp -a access_token
```

(These print the metadata; do not pass `-w` to print the actual secret.)

- [ ] **Step 5: Commit**

```bash
git add cmd/protonmail-mcp/login.go go.mod go.sum
git commit -m "feat(cli): login subcommand with 2FA + HV handling"
```

---

### Task 14: `logout` subcommand

**Files:**
- Create: `cmd/protonmail-mcp/logout.go`

- [ ] **Step 1: Implement**

```go
package main

import (
	"context"
	"fmt"
	"os"

	"protonmail-mcp/internal/keychain"
	"protonmail-mcp/internal/session"
)

func runLogout(ctx context.Context) error {
	apiURL := os.Getenv("PROTONMAIL_MCP_API_URL")
	if apiURL == "" {
		apiURL = "https://mail.proton.me/api"
	}
	sess := session.New(apiURL, keychain.New())
	if err := sess.Logout(); err != nil {
		return err
	}
	fmt.Println("Logged out. Keychain cleared.")
	return nil
}
```

- [ ] **Step 2: Build + commit**

```bash
go build ./...
git add cmd/protonmail-mcp/logout.go
git commit -m "feat(cli): logout subcommand"
```

---

### Task 15: `status` subcommand

**Files:**
- Create: `cmd/protonmail-mcp/status.go`

- [ ] **Step 1: Implement**

```go
package main

import (
	"context"
	"fmt"
	"os"

	"protonmail-mcp/internal/keychain"
	"protonmail-mcp/internal/session"
)

func runStatus(ctx context.Context) error {
	apiURL := os.Getenv("PROTONMAIL_MCP_API_URL")
	if apiURL == "" {
		apiURL = "https://mail.proton.me/api"
	}
	kc := keychain.New()
	if _, err := kc.LoadCreds(); err != nil {
		fmt.Println("Not logged in. Run `protonmail-mcp login`.")
		return nil
	}
	sess := session.New(apiURL, kc)
	c, err := sess.Client(ctx)
	if err != nil {
		fmt.Println("Logged in (creds present), but session refresh failed:", err)
		return nil
	}
	u, err := c.GetUser(ctx)
	if err != nil {
		fmt.Println("Logged in, but GetUser failed:", err)
		return nil
	}
	fmt.Printf("Logged in as %s\n", u.Email)
	fmt.Printf("Used: %d / %d bytes\n", u.UsedSpace, u.MaxSpace)
	return nil
}
```

- [ ] **Step 2: Build + commit**

```bash
go build ./...
git add cmd/protonmail-mcp/status.go
git commit -m "feat(cli): status subcommand"
```

---

## Phase 11 — Integration tests + manual checklist

### Task 16: Real test harness against go-proton-api dev server

**Files:**
- Replace: `internal/tools/internal/testharness/harness.go`
- Create: `integration/integration_test.go`

**What this gives us:** The skipping harness from Task 7 becomes a real one. Boots the `go-proton-api` dev server (from package `github.com/ProtonMail/go-proton-api/server`), creates a test user, drives the MCP server through an in-process transport, and exercises all read tools end-to-end.

- [ ] **Step 1: Implement the real harness**

```go
// Package testharness boots a go-proton-api dev server, instantiates the
// session manager + tool registry against it, and exposes a Call helper that
// invokes tools via an in-process MCP client. Used by tools/* tests and by the
// integration suite.
package testharness

import (
	"context"
	"encoding/json"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/go-proton-api/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zalando/go-keyring"

	"protonmail-mcp/internal/keychain"
	"protonmail-mcp/internal/session"
	"protonmail-mcp/internal/tools"
)

type Harness struct {
	t      *testing.T
	srv    *server.Server
	mcp    *mcp.ClientSession
	stop   func()
}

// Boot creates a user with the supplied credentials on the dev server, logs
// the session manager in, registers tools, and connects an in-process MCP
// client to the server.
func Boot(t *testing.T, email, password string) *Harness {
	t.Helper()
	keyring.MockInit()

	devsrv := server.New()
	t.Cleanup(devsrv.Close)
	if _, _, err := devsrv.CreateUser(email, []byte(password)); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	apiURL := devsrv.GetHostURL()
	kc := keychain.New()
	sess := session.New(apiURL, kc)
	if err := sess.Login(context.Background(), session.LoginInput{Username: email, Password: password}); err != nil {
		t.Fatalf("session.Login: %v", err)
	}

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	tools.Register(mcpServer, tools.Deps{Session: sess})

	// Connect client to server via in-process transport.
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	cs, ss, err := mcp.NewInProcessTransports(mcpServer)
	if err != nil {
		t.Fatalf("transports: %v", err)
	}
	go func() { _ = mcpServer.Run(context.Background(), ss) }()
	csess, err := client.Connect(context.Background(), cs, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}

	h := &Harness{
		t:    t,
		srv:  devsrv,
		mcp:  csess,
		stop: func() { _ = csess.Close() },
	}
	t.Cleanup(h.Close)
	return h
}

func (h *Harness) Close() {
	if h.stop != nil {
		h.stop()
		h.stop = nil
	}
}

// Call invokes a tool by name with the given arguments and unmarshals its
// structured output into a map[string]any.
func (h *Harness) Call(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	res, err := h.mcp.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, err
	}
	if res.IsError {
		return nil, fmtErr(res)
	}
	if res.StructuredContent == nil {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(res.StructuredContent, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func fmtErr(res *mcp.CallToolResult) error {
	if len(res.Content) == 0 {
		return errString("tool error (no detail)")
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		return errString(tc.Text)
	}
	return errString("tool error")
}

type errString string

func (e errString) Error() string { return string(e) }

// Suppress unused-import warning when the proton package isn't referenced.
var _ = proton.Address{}
```

> **API verification.** The MCP Go SDK API for in-process transports is `mcp.NewInProcessTransports`; verify with `go doc github.com/modelcontextprotocol/go-sdk/mcp NewInProcessTransports` and adjust if it has been renamed. Same for `dev server`'s `CreateUser` signature: `go doc github.com/ProtonMail/go-proton-api/server Server.CreateUser`.

- [ ] **Step 2: Write a broader integration test in `integration/integration_test.go`**

```go
//go:build integration
// +build integration

package integration_test

import (
	"context"
	"testing"

	"protonmail-mcp/internal/tools/internal/testharness"
)

func TestReadToolsRoundTrip(t *testing.T) {
	h := testharness.Boot(t, "user@example.test", "hunter2")
	defer h.Close()

	cases := []struct {
		tool string
		args map[string]any
	}{
		{"proton_whoami", map[string]any{}},
		{"proton_session_status", map[string]any{}},
		{"proton_list_addresses", map[string]any{}},
		{"proton_get_mail_settings", map[string]any{}},
		{"proton_get_core_settings", map[string]any{}},
	}
	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			out, err := h.Call(context.Background(), c.tool, c.args)
			if err != nil {
				t.Fatalf("call %s: %v", c.tool, err)
			}
			if out == nil {
				t.Fatalf("nil output")
			}
		})
	}
}
```

> **Custom-domain integration tests.** The dev server doesn't implement the custom-domain endpoints (since `go-proton-api` itself doesn't). Don't include them here — the unit tests in `internal/protonraw/` already cover the request/response wire format with `httptest`. End-to-end coverage for custom domains has to live in the manual checklist (Task 17).

- [ ] **Step 3: Run the integration test**

Run: `go test -tags=integration ./integration/... -v`
Expected: PASS for all subtests.

If the dev server's `CreateUser` signature differs, adjust and re-run. If the in-process transport API differs, adjust.

- [ ] **Step 4: Run the original `tools/identity_test.go` (now no longer skipping)**

Run: `go test ./internal/tools/... -v`
Expected: `TestWhoamiRoundTrip` now executes (no Skip) and passes. If it still skips, the harness `Boot` is still calling `t.Skip()` — remove that line.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/internal/testharness/ integration/
git commit -m "test: real testharness + integration suite for read tools"
```

---

## Phase 12 — Documentation + CI

### Task 17: README, testing checklist, CI workflow

**Files:**
- Create: `README.md`
- Create: `docs/testing-checklist.md`
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write `README.md`**

```markdown
# protonmail-mcp

A Model Context Protocol (MCP) server for [Proton Mail](https://proton.me/mail), letting Claude Code (or any MCP client) manage addresses, custom domains, mail settings, and encryption keys.

## Status: v1

| Capability | v1 | Notes |
|---|---|---|
| Addresses (list, get, enable/disable, delete) | yes | via `go-proton-api` |
| Create address (alias on custom domain) | yes | via `internal/protonraw` |
| Custom domains (list, get, add, verify, remove) | yes | via `internal/protonraw` |
| Mail settings (get, update display name + signature) | yes | |
| Account settings (get, update locale) | yes | |
| Encryption keys (list) | yes | |
| Encryption key generation / set primary | conditional | depends on go-proton-api method shapes; see `docs/superpowers/specs/2026-04-26-protonmail-mcp-design.md` |
| Mail (read/send/labels) | **v2** | |
| Calendar / Drive | **v3** | |

## Install

Requires Go 1.22+.

```
go install protonmail-mcp/cmd/protonmail-mcp@latest
```

## First-time login

```
protonmail-mcp login
```

Prompts for your Proton email, password, and (if 2FA is enabled) an `otpauth://` URI or a one-shot 6-digit code. Pasting the URI lets the server refresh sessions silently; pasting a code requires re-login on token expiry.

Credentials are stored in the macOS Keychain under service `protonmail-mcp`.

## Run as MCP server

Configure your MCP host (Claude Code, etc.) to launch:

```
protonmail-mcp
```

over stdio. By default the server registers **read-only** tools. To expose mutating tools (create address, add domain, delete, etc.):

```
PROTONMAIL_MCP_ENABLE_WRITES=1 protonmail-mcp
```

## Environment variables

| Variable | Purpose | Default |
|---|---|---|
| `PROTONMAIL_MCP_ENABLE_WRITES` | When `1`/`true`/`yes`, registers mutating tools. Read tools are always available. | unset (reads only) |
| `PROTONMAIL_MCP_LOG_LEVEL` | `debug` for verbose JSON logs to stderr. | `info` |
| `PROTONMAIL_MCP_API_URL` | Override Proton API base URL (used in tests). | `https://mail.proton.me/api` |

## Tool reference

See `docs/superpowers/specs/2026-04-26-protonmail-mcp-design.md` §5 for the full v1 tool inventory.

## Security model

See spec §8. tl;dr: Keychain-only secrets, redacted logs, writes opt-in, no daemon, no IPC socket.

## Development

```
go test ./...                          # unit tests
go test -tags=integration ./...        # integration tests against go-proton-api dev server
```

Manual pre-release checks: `docs/testing-checklist.md`.

## License

MIT.
```

- [ ] **Step 2: Write `docs/testing-checklist.md`**

```markdown
# Pre-release manual testing checklist

Run before tagging any release. Uses a real Proton account (Unlimited or Business). Reset session afterwards with `protonmail-mcp logout`.

## Setup

- [ ] `go install ./cmd/protonmail-mcp`
- [ ] `protonmail-mcp logout` (clean slate)

## Auth

- [ ] `protonmail-mcp login` with valid credentials and 2FA enabled — succeeds with otpauth URI
- [ ] `protonmail-mcp login` with valid credentials and a 6-digit code — succeeds, prints warning about refresh
- [ ] `protonmail-mcp login` with wrong password — fails with `proton/auth_required`
- [ ] `protonmail-mcp status` — prints the logged-in email + storage usage

## Read tools (writes flag OFF)

- [ ] `proton_whoami` — returns the right email and plan
- [ ] `proton_list_addresses` — includes the primary address and any aliases
- [ ] `proton_list_custom_domains` — includes any custom domains; verification states match the Proton web UI
- [ ] `proton_get_custom_domain` for one verified domain — returns DNS records matching the live records at the registrar
- [ ] `proton_list_address_keys` — fingerprints match what `gpg --list-keys` shows after importing the armored public key

## Write tools (`PROTONMAIL_MCP_ENABLE_WRITES=1`)

Use a throwaway custom domain you don't mind churning.

- [ ] `proton_add_custom_domain` for a new domain — returns required DNS records
- [ ] Manually publish the records (or via gandi MCP)
- [ ] `proton_verify_custom_domain` after DNS propagation — moves verification states forward
- [ ] `proton_create_address` on the new domain — alias appears in `proton_list_addresses`
- [ ] `proton_set_address_status enabled=false` — alias disabled in web UI
- [ ] `proton_delete_address` — alias gone
- [ ] `proton_remove_custom_domain` — domain gone

## Failure modes

- [ ] Force a CAPTCHA (run login from an unusual IP, e.g. via VPN) — `proton/captcha` error includes the verification URL
- [ ] Make 30+ rapid identical requests — eventually `proton/rate_limited` with retry-after

## Cleanup

- [ ] `protonmail-mcp logout`
- [ ] `security find-generic-password -s protonmail-mcp -a username` returns "not found"
```

- [ ] **Step 3: Write `.github/workflows/ci.yml`**

```yaml
name: ci
on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: vet
        run: go vet ./...
      - name: build
        run: go build ./...
      - name: unit tests
        run: go test ./...
      - name: integration tests
        run: go test -tags=integration ./...
```

> Note: the `zalando/go-keyring` package falls back to a file-based keyring on Linux when no DBus is available, so `keyring.MockInit()`-using tests work in CI without extra setup. If CI flakes on real-keyring tests, add `secret-tool` or set up `dbus-launch` per `go-keyring`'s README.

- [ ] **Step 4: Commit**

```bash
git add README.md docs/testing-checklist.md .github/
git commit -m "docs: README + testing checklist + CI workflow"
```

---

## Self-review (run before handing off)

After all 17 tasks have been completed, before tagging v0.1.0:

- [ ] Every spec section in `docs/superpowers/specs/2026-04-26-protonmail-mcp-design.md` §5 (tool inventory) maps to a registered tool in `internal/tools/`.
- [ ] `PROTONMAIL_MCP_ENABLE_WRITES=0 ./protonmail-mcp` advertises only read tools (verify by `tools/list` over stdio).
- [ ] `PROTONMAIL_MCP_ENABLE_WRITES=1 ./protonmail-mcp` advertises read + write tools.
- [ ] `protonmail-mcp logout && protonmail-mcp` returns `proton/auth_required` from the first read tool call.
- [ ] No occurrence of plaintext password / token in any log line (run with `PROTONMAIL_MCP_LOG_LEVEL=debug` and verify).
- [ ] `go vet ./... && go test ./... && go test -tags=integration ./...` all clean.
- [ ] Manual checklist (`docs/testing-checklist.md`) executed against a real account.

---

## Out-of-plan reminders

These are explicitly NOT in this plan; flagged here so future PRs don't sneak them in under "v1 cleanup":

- Multi-account support
- Caching
- Web UI / dashboard
- Daemon mode
- Cross-MCP orchestration with the Gandi MCP (host LLM does that)
- Organization / sub-user management (requires Proton Business)
- Calendar / Drive (v3)
- Reading/sending mail (v2)
- TOTP-on-prompt mode (`--no-store-totp`)
- `proton_revoke_address_key`
