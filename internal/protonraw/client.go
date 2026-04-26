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

// attachCtx is a tiny helper kept for symmetry with the planning notes.
// (Currently unused; resty.Request.SetContext is called directly at the call site.)
var _ = func(req *resty.Request, ctx context.Context) *resty.Request {
	return req.SetContext(ctx)
}
