GO ?= go

.PHONY: verify-cassettes
verify-cassettes:
	$(GO) run ./cmd/testvcr-lint \
		internal/tools/testdata/cassettes \
		internal/session/testdata/cassettes \
		internal/server/testdata/cassettes \
		internal/testharness/testdata/cassettes \
		cmd/protonmail-mcp/testdata/cassettes
