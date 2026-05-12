// Temporary stub — deleted by T05 (BodyAwareMatcher).
package testvcr

import (
	"net/http"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
)

func BodyAwareMatcher(_ *http.Request, _ cassette.Request) bool { return true }
