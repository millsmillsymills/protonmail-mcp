// Package protonraw implements Proton API endpoints not exposed by
// go-proton-api: custom-domain CRUD and address creation.
//
// Endpoint paths and payload shapes are sourced from
// https://github.com/ProtonMail/WebClients (read-only reference). Each method
// links to its WebClients counterpart in a comment.
package protonraw

import (
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
	if err := json.Unmarshal(resp.Body(), &env); err == nil {
		if env.Error != "" {
			return fmt.Errorf("proton api: %s (code %d)", env.Error, env.Code)
		}
		// Proton's success code is 1000. Non-zero non-1000 codes signal a
		// business error that the API didn't accompany with a string Error.
		if env.Code != 0 && env.Code != 1000 {
			return fmt.Errorf("proton api: unexpected code %d", env.Code)
		}
	}
	if out != nil {
		if err := json.Unmarshal(resp.Body(), out); err != nil {
			return fmt.Errorf("decode body: %w", err)
		}
	}
	return nil
}
