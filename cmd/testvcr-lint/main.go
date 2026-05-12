package main

import (
	"fmt"
	"os"

	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{
			"internal/tools/testdata/cassettes",
			"internal/session/testdata/cassettes",
			"internal/server/testdata/cassettes",
			"internal/testharness/testdata/cassettes",
			"cmd/protonmail-mcp/testdata/cassettes",
		}
	}
	findings := testvcr.Scan(roots...)
	if len(findings) == 0 {
		return
	}
	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "%s:%d [%s] %s\n", f.Path, f.Line, f.Rule, f.Hit)
	}
	os.Exit(1)
}
