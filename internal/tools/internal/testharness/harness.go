// Package testharness is a temporary stub. The real implementation lands in
// Task 16; for now it lets tool tests compile while skipping at runtime.
package testharness

import (
	"context"
	"testing"
)

type Harness struct{}

func (h *Harness) Close() {}

func (h *Harness) Call(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return nil, nil
}

func Boot(t *testing.T, _ string, _ string) *Harness {
	t.Skip("test harness implemented in Task 16; skipped for now")
	return &Harness{}
}
