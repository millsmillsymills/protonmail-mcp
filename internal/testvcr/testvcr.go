// Package testvcr will provide a thin wrapper around gopkg.in/dnaeon/go-vcr.v4
// for cassette-based tests. This file is a temporary placeholder; subsequent
// tasks (T03+) replace it with the real implementation.
package testvcr

// Blank import retains the go-vcr dependency through `go mod tidy` until T03
// adds a real consumer.
import _ "gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
