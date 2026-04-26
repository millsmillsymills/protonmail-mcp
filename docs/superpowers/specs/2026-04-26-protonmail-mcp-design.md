# protonmail-mcp — Design

**Date:** 2026-04-26
**Status:** Approved (brainstorming complete; ready for implementation plan)
**Audience:** Implementation planner; future contributors

---

## 1. Context & goals

Build an MCP server that lets Claude Code manage a Proton Mail account: addresses and aliases, custom domains, mail settings, and encryption keys in v1; full mail (read/send/labels/contacts) in v2; calendar (and Drive if available) in v3.

The user's account is **Proton Unlimited** (single-user, up to 3 custom domains, 15 addresses), so all v1 surfaces are reachable. Organization / sub-user management requires Proton Business and is **out of scope**.

### Why this is feasible

Proton has never published an official public REST API. However, [`ProtonMail/go-proton-api`](https://github.com/ProtonMail/go-proton-api) (MIT-licensed, maintained by Proton AG) is a Go library implementing a client for "a subset of" the Proton REST API — the same surface their own clients use. It covers everything v1 needs:

- `address.go` — addresses & aliases
- `manager_domains.go` — custom domains
- `mail_settings.go`, `core_settings.go` — settings
- `keys.go`, `keyring.go` — encryption keys
- `auth.go`, `hv.go` — SRP auth, human verification
- `message.go`, `label.go`, `contact.go` — needed in v2
- `calendar.go` — needed in v3

It also ships a development server (referenced in its README) usable for integration tests.

The third-party Python (`protonmail-api-client`) and Node (`protonmail-api`) libraries cover roughly 30% of what v1 needs and are independently reverse-engineered. They are unsuitable as a foundation.

### Goals (v1)

- A single Go binary, installable locally, that runs as an MCP stdio server.
- 20 tools spanning identity/session, addresses, custom domains, settings, keys.
- Reads always available; writes opt-in via env flag.
- Credentials and TOTP secret stored in macOS Keychain.
- Clean, documented, testable; integration tests run against `go-proton-api`'s dev server.

### Non-goals (v1)

- Reading or sending mail (v2)
- Calendar / Drive (v3)
- Organization / sub-user management (requires Business plan)
- Multi-account support (one account per install)
- A daemon, web UI, or background service
- Caching of API reads
- Cross-MCP orchestration (Proton MCP returns DNS records as data; Claude Code hands them to the Gandi MCP separately)

---

## 2. Design decisions

These were settled during brainstorming and are inputs, not open questions.

| # | Decision | Rationale |
|---|---|---|
| 1 | Wrap `go-proton-api` directly in Go | Only first-party Proton library; phased v1→v3 needs full API surface. |
| 2 | Use [`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk); fall back to `mark3labs/mcp-go` if rough | Official SDK preferred; mature alternative as backup. |
| 3 | OS keychain (via `zalando/go-keyring`) for username, password, TOTP secret, and session tokens | Same threat model as proton-bridge; survives restarts. |
| 4 | Reads always on; writes gated by single env flag `PROTONMAIL_MCP_ENABLE_WRITES=1` | Matches user's Gandi MCP convention; defense-in-depth with Claude Code permission UI. |
| 5 | Single global write flag — no per-category sub-flags, including for key ops | Env flag + Claude permission UI is sufficient gating. |
| 6 | Single-responsibility — Proton MCP returns DNS records; host LLM hands them to Gandi MCP | Each MCP single-purpose; swappable if user changes registrar. |
| 7 | Dual-mode binary: `protonmail-mcp` (MCP server) + `login`/`logout`/`status` subcommands | Login can't happen over MCP stdio; colocating the CLI shares session-manager code. |
| 8 | One `*proton.Client` per process; lazy session bootstrap on first MCP call | Simpler than a worker pool; matches v3 needs without re-architecture. |
| 9 | Tool DTOs decoupled from `go-proton-api` types | Insulates tool schemas from upstream drift. |
| 10 | Future hook for `--no-store-totp` mode in v2+ | TOTP-on-disk is the user's stated future preference; design accommodates without building. |

---

## 3. Architecture

```
protonmail-mcp                       (default: MCP stdio server)
protonmail-mcp login                 (interactive: SRP + TOTP, writes keychain)
protonmail-mcp logout                (clears keychain entries)
protonmail-mcp status                (prints session state, no secrets)
```

**Layers, top to bottom:**

1. **MCP transport** (`internal/server`) — Go MCP SDK; JSON-RPC, tool registration, schemas, errors.
2. **Tool registry** (`internal/tools`) — declarative tools with name, description, JSON schema, handler. Read tools always registered; write tools registered only when `PROTONMAIL_MCP_ENABLE_WRITES=1`.
3. **Session manager** (`internal/session`) — owns the single `*proton.Client`. Loads session from keychain on first call; refreshes auth tokens on 401; surfaces clear errors when the session is unrecoverable.
4. **Keychain adapter** (`internal/keychain`) — wraps `zalando/go-keyring`. Service name `protonmail-mcp`.
5. **`go-proton-api`** — vendored dependency.

**One process, no background workers.** Session refresh happens lazily in the request path, not on a timer.

---

## 4. Components

| Component | Purpose | Depends on |
|---|---|---|
| `cmd/protonmail-mcp` | Entry point. Parses subcommand. Wires layers. | All below |
| `internal/server` | MCP transport. Registers tools, dispatches calls, formats responses. Does not touch Proton directly. | `tools`, MCP Go SDK |
| `internal/tools` | Tool definitions. One file per resource (`addresses.go`, `domains.go`, `settings.go`, `keys.go`, `identity.go`). Read tools registered always; writes gated by env flag at registration time. | `session`, `proterr` |
| `internal/session` | Owns `*proton.Client`. Methods: `Get(ctx)`, `Login(ctx, username, password, totp)`, `Logout()`. | `keychain`, `go-proton-api` |
| `internal/keychain` | Persistence for credentials and tokens. Six known keys under one service name. Methods: `LoadCreds`, `SaveCreds`, `LoadSession`, `SaveSession`, `Clear`. | `zalando/go-keyring` |
| `internal/proterr` | Translates `go-proton-api` errors and HTTP statuses into stable MCP error codes. | — |
| `internal/log` | `slog`-based logger with field-allowlist redaction handler. | — |

**Cross-cutting:**

- **Config** is env-var reads at startup, no config file. Knobs: `PROTONMAIL_MCP_ENABLE_WRITES`, `PROTONMAIL_MCP_LOG_LEVEL`, `PROTONMAIL_MCP_API_URL` (override default Proton API URL for testing).

**Boundaries that matter:**

- `tools` calls `session.Get(ctx)` and `proton.Client` methods. It never touches keychain. It maps `go-proton-api` returns into its own DTO types so v2 can swap the underlying client (or mock it) without touching tool definitions.
- `session` is the only place that knows about `go-proton-api`'s auth lifecycle.

---

## 5. v1 tool inventory

**20 tools total**, prefixed `proton_`. **R** = always registered. **W** = registered only when `PROTONMAIL_MCP_ENABLE_WRITES=1`.

### Identity & session (2)

| Tool | R/W | Purpose |
|---|---|---|
| `proton_whoami` | R | Current user identity, plan, storage usage |
| `proton_session_status` | R | Auth state, last refresh, account email |

### Addresses & aliases (6)

| Tool | R/W | Purpose |
|---|---|---|
| `proton_list_addresses` | R | List all addresses & aliases with status |
| `proton_get_address` | R | Single address detail |
| `proton_create_address` | W | Create alias on a custom domain |
| `proton_update_address` | W | Update display name / signature / order |
| `proton_set_address_status` | W | Enable / disable an address |
| `proton_delete_address` | W | **Destructive.** Permanently delete alias. |

### Custom domains (5)

| Tool | R/W | Purpose |
|---|---|---|
| `proton_list_custom_domains` | R | List custom domains + verification state |
| `proton_get_custom_domain` | R | Detail + required DNS records (MX, SPF, DKIM, DMARC, verify TXT) as structured JSON |
| `proton_add_custom_domain` | W | Add domain; returns required DNS records |
| `proton_verify_custom_domain` | W | Trigger Proton-side DNS verification |
| `proton_remove_custom_domain` | W | **Destructive.** Removes domain (orphans aliases). |

### Settings (4)

| Tool | R/W | Purpose |
|---|---|---|
| `proton_get_mail_settings` | R | Mail settings (signature, auto-reply, identity, etc.) |
| `proton_get_core_settings` | R | Account-level settings (locale, notifications) |
| `proton_update_mail_settings` | W | Patch one or more mail-settings fields |
| `proton_update_core_settings` | W | Patch core settings |

### Encryption keys (3)

| Tool | R/W | Purpose |
|---|---|---|
| `proton_list_address_keys` | R | List keys for an address (fingerprint, algo, primary flag, armored public key) |
| `proton_generate_address_key` | W | Generate new key for an address |
| `proton_set_primary_address_key` | W | Change which key is primary |

(No `proton_revoke_address_key` in v1; user can revoke via the Proton web UI when needed. Add in a later release if explicitly requested.)

---

## 6. Data flow

### 6.1 First-time login (`protonmail-mcp login`)

```
1. CLI prompts: username (visible), password (hidden)
2. Calls go-proton-api auth.NewClient → SRP exchange with Proton API
3. Server response forks:
   3a. Success                       → goto 5
   3b. 2FA required                  → CLI prompts for TOTP secret URI (otpauth://...)
                                       OR a 6-digit code (warns that future refreshes
                                       will fail without the seed)
   3c. CAPTCHA / HV required         → CLI prints verification URL; user solves in
                                       browser; pastes resulting token; retry
4. Submit TOTP → Proton returns access + refresh tokens
5. Keychain writes:
     username, password, totp_secret, refresh_token, access_token, last_refresh_at
6. CLI prints: "Logged in as <email>. Run 'protonmail-mcp' to start MCP server."
```

### 6.2 MCP request lifecycle

```
Claude Code           MCP server                  go-proton-api          Proton API
     │                     │                           │                      │
     │ tool call           │                           │                      │
     ├────────────────────►│                           │                      │
     │                     │ session.Get(ctx)          │                      │
     │                     ├──┐                        │                      │
     │                     │  │ load tokens (1st call) │                      │
     │                     │  │ from keychain          │                      │
     │                     ├──┘                        │                      │
     │                     │ client.ListAddresses(ctx) │                      │
     │                     ├──────────────────────────►│ HTTP GET             │
     │                     │                           ├─────────────────────►│
     │                     │                           │◄─────────────────────┤
     │                     │                           │ 401? auto-refresh    │
     │                     │                           │ tokens, retry once   │
     │                     │◄──────────────────────────┤                      │
     │                     │ shape DTO, return         │                      │
     │◄────────────────────┤                           │                      │
```

**Lazy session bootstrap.** First MCP call triggers keychain read + token validation. Subsequent calls reuse the in-memory client. `go-proton-api` handles 401 auto-refresh in its HTTP layer; if even refresh fails, the MCP returns `proton/auth_required`.

**Confirmation of destructive ops** is the host's job (Claude Code permission UI). The MCP does not implement its own confirm-token pattern.

---

## 7. Error handling

One taxonomy, one mapping point (`internal/proterr`), one rule: never leak credentials into error strings, even on debug logging.

| MCP error code | Triggered by | Tool response |
|---|---|---|
| `proton/auth_required` | No tokens in keychain, refresh failed, invalid_grant | "Session expired. Run `protonmail-mcp login` interactively, then retry." |
| `proton/2fa_required` | Auth response asks for TOTP and we have no TOTP secret stored | "TOTP required. Re-run `protonmail-mcp login` with the otpauth:// URI." |
| `proton/captcha` | Proton returns HV challenge | Returns the verification URL; instructs to solve in browser then re-login. |
| `proton/rate_limited` | HTTP 429 | Returns retry-after seconds; tool fails this call, does not auto-sleep. |
| `proton/writes_disabled` | Defense-in-depth: write tool somehow invoked while flag off | "Writes are disabled. Set `PROTONMAIL_MCP_ENABLE_WRITES=1` to enable." |
| `proton/not_found` | 404 on get/update/delete | Resource id echoed back. |
| `proton/conflict` | 409 (address already exists, domain already registered) | Echoes server's reason. |
| `proton/validation` | 4xx with field-level errors | Field list surfaced verbatim; no state change. |
| `proton/plan_required` | 402 / "feature not in plan" | "This requires a Proton Business plan." |
| `proton/upstream` | 5xx, network errors, EOF, TLS errors | Generic "Proton API unavailable." Includes request id if present. |
| `proton/internal` | Anything unanticipated | Stack trace at debug level; generic message returned. |

### Behavior rules

- **No retries inside the MCP.** `go-proton-api` already auto-refreshes 401 once; beyond that, surface the error.
- **No partial successes.** Update calls either apply or return an error and change nothing.
- **`proton/captcha` is sticky.** Once hit, in-memory client is marked "needs login"; subsequent calls return `auth_required` until login is rerun.
- **Redaction is in the slog handler**, not at call sites. Allowlist-based: any field name containing `password`, `token`, `secret`, `key`, `totp` is redacted.

### Deliberately not handled

- **Partial network outages mid-write.** Surfaced as `proton/upstream` with a hint to verify state. No idempotency keys / op log.
- **Concurrent MCP server instances.** Two Claude Code sessions racing on the same keychain refresh token is documented as a known limitation, not engineered around.

---

## 8. Security & threat model

### Assets

1. Long-lived credentials: username, password, TOTP secret
2. Session credentials: refresh + access tokens
3. Mailbox content (v2+; design-aware now)
4. Operational metadata: addresses, DNS records (low sensitivity)

### In scope

- Casual file-system access (other local user, accidental git commit): credentials never on disk in plaintext.
- Hostile prompt tricking the LLM into write tools: env flag (off by default), Claude Code permission UI, explicit destructive verbs in tool names.
- Logs in a bug report: redaction at the slog handler, allowlist-based.
- Compromised binary at install time: out of scope (same trust model as any Go binary install).

### Out of scope

- A user who has root on their own Mac extracting their own credentials (not an attacker).
- Network MITM (trusted to TLS + `go-proton-api` defaults; no cert pinning).
- Quantum / side-channel attacks on PGP key generation (trusted to `gopenpgp`).

### Controls

| Control | Implementation |
|---|---|
| Credentials at rest | macOS Keychain via `zalando/go-keyring`, service `protonmail-mcp`. Six entries, six keys, no JSON blobs. |
| TOTP-on-disk opt-out | Documented design hook for v2 (`--no-store-totp` flag triggers per-session prompt). Not built in v1; session manager interface accommodates. |
| Writes default off | `PROTONMAIL_MCP_ENABLE_WRITES` must be set; absence means write tools aren't even registered (LLM cannot see them). |
| Destructive op naming | Verbs in tool names: `delete_address`, `remove_custom_domain`. Lets host show meaningful prompts. |
| Log redaction | slog handler with field allowlist. |
| Public key handling | Armored public keys are not sensitive; surfaced freely. **Private keys never leave `go-proton-api`'s in-memory state.** No tool exports a private key. |
| Process surface | Single binary. No daemon, no IPC socket, no HTTP listener. Stdio only. |

### Explicitly not doing

- No anti-debug, no screen-capture detection, no keylogger detection.
- No "are you sure?" prompts inside the MCP (Claude Code permission UI handles this).
- No audit log file (stderr is the log).
- No "lock the MCP after N failed calls" (false-positive prone; Proton's rate limiter is the real defense).

---

## 9. Testing

`go-proton-api` ships its own development server, which is the cornerstone — we can run a fake Proton API in-process and test against it.

### Unit tests (`*_test.go`, alongside source)

- `internal/proterr` — error-code mapping; table-driven.
- `internal/log` — redaction; asserts `<redacted>` for sensitive fields, allowlist passes.
- `internal/keychain` — fake `keyring` backend (`keyring.MockInit()`); round-trip save/load/clear.
- `internal/tools` — registration logic (writes absent when flag off, present when on); JSON-schema validation of inputs.

### Integration tests (`integration/*_test.go`, build tag `integration`)

- Start `go-proton-api` dev server in-process; point session manager at it; exercise every tool.
- Auth flow: SRP login against the fake server, including the 2FA path.
- Session refresh: force token expiry; assert auto-refresh succeeds.

### Manual checklist (`docs/testing-checklist.md`, run before releases)

- Real `protonmail-mcp login` against the user's Proton Unlimited account, with TOTP. Captures HV / CAPTCHA / real DNS verification timing — paths the dev server can't fully simulate.
- One destructive write, verified manually: create + delete a throwaway alias on a custom domain.
- Confirm tool listing changes when toggling `PROTONMAIL_MCP_ENABLE_WRITES`.

### Not writing

- Mocks of `go-proton-api`. The dev server replaces them.
- E2E against real Proton in CI (rate-limit risk, secrets in CI).
- Property-based / fuzz tests in v1.

### CI

Single GitHub Actions workflow: `go vet`, `go build`, unit tests, integration tests with `-tags=integration`. No deploy step.

---

## 10. Project layout

```
protonmail-mcp/
├── go.mod
├── go.sum
├── README.md                       (install + login + flag reference)
├── LICENSE                         (MIT, matching go-proton-api)
├── cmd/
│   └── protonmail-mcp/
│       └── main.go                 (subcommand routing only)
├── internal/
│   ├── server/                     (MCP transport, registers tool set)
│   ├── tools/
│   │   ├── tools.go                (Tool struct + registry)
│   │   ├── identity.go             (whoami, session_status)
│   │   ├── addresses.go            (6 tools)
│   │   ├── domains.go              (5 tools)
│   │   ├── settings.go             (4 tools)
│   │   └── keys.go                 (3 tools)
│   ├── session/
│   │   ├── session.go              (Session, Get, Login, Logout)
│   │   └── refresh.go              (token refresh helpers)
│   ├── keychain/
│   │   └── keychain.go             (Save/Load/Clear)
│   ├── proterr/
│   │   └── proterr.go              (error mapping)
│   └── log/
│       └── log.go                  (slog handler with redaction)
├── integration/
│   ├── main_test.go                (boots go-proton-api dev server)
│   └── *_test.go                   (one file per tool category)
└── docs/
    ├── testing-checklist.md
    └── superpowers/
        └── specs/
            └── 2026-04-26-protonmail-mcp-design.md
```

---

## 11. Forward compatibility (v2/v3)

Decisions paying off in later phases:

1. **Tool DTOs decoupled from `go-proton-api` types.** Adding messages in v2 means a `tools/messages.go` shaping its own DTOs. API drift in `go-proton-api` doesn't break tool schemas.
2. **Tool registration is data-driven.** Adding v2 mail tools = appending entries + a one-line registration. Transport, error mapping, session, keychain don't change.
3. **Session manager owns one client, not a pool.** v3 calendar/drive use the same client.
4. **Single global write flag.** v2's `delete_message`, `mark_thread_read`, etc. ride the same flag.
5. **Stdio-only transport.** If MCP introduces SSE / WebSocket variants later, only `internal/server` changes.

---

## 12. Open follow-ups (not in v1)

- **TOTP-on-prompt mode** (`--no-store-totp`): user's stated future preference. Hook is in the session manager; build when requested.
- **Bridge coexistence note.** Running this MCP and proton-bridge against the same account simultaneously *should* work (different auth sessions), but Proton's anti-abuse layer might rate-limit or HV-challenge if patterns look unusual. Documented caveat, not engineered around.
- **`proton_revoke_address_key`** — deferred from v1; revisit if a concrete need surfaces.

---

## 13. Out-of-scope reminders (so they don't sneak in)

- Multi-account
- Plugin system for tools
- Web UI
- Caching of API reads
- Cross-MCP orchestration with Gandi (host LLM does the orchestration)
- Organization / sub-user management (Business plan only)
- Daemon mode / background workers
