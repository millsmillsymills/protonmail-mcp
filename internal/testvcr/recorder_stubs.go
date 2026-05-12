// Temporary stubs — deleted by T04 (saveHook) and T05 (BodyAwareMatcher).
package testvcr

import (
	"net/http"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
)

func saveHook(i *cassette.Interaction) error { return nil }

func BodyAwareMatcher(_ *http.Request, _ cassette.Request) bool { return true }
