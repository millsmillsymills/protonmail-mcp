package session

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
)

// recordingRT captures the x-pm-appversion header from the first request and
// returns a canned error so go-proton-api's auth flow aborts before parsing.
type recordingRT struct {
	captured string
}

func (r *recordingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if r.captured == "" {
		r.captured = req.Header.Get("x-pm-appversion")
	}
	return nil, errors.New("intercepted: no real network call")
}

// TestAppVersionHeaderSentByManager proves the proton.Manager configured by
// our session package actually transmits the appVersionHeader() value (not
// go-proton-api's default "go-proton-api"). This is the regression check
// for the 2026-04-26 fix where Proton's API rejected the default with
// code 2064 ("Platform `go` is not valid").
func TestAppVersionHeaderSentByManager(t *testing.T) {
	rt := &recordingRT{}
	mgr := proton.New(
		proton.WithHostURL("https://mail.proton.me/api"),
		proton.WithAppVersion(appVersionHeader()),
		proton.WithTransport(rt),
		proton.WithRetryCount(0),
	)
	defer mgr.Close()

	// Trigger any request. The RoundTripper intercepts and records the
	// header before returning an error.
	_, _, _ = mgr.NewClientWithLogin(context.Background(), "noone@example.test", []byte("x"))

	if rt.captured == "" {
		t.Fatalf("RoundTripper saw no x-pm-appversion header")
	}
	want := appVersionHeader()
	if rt.captured != want {
		t.Fatalf("appversion mismatch: sent %q, helper returned %q", rt.captured, want)
	}
	// Hard-pin the format so a future regression to go-proton-api defaults
	// is caught here rather than at runtime against real Proton.
	if !strings.Contains(rt.captured, "-bridge@") {
		t.Fatalf("appversion does not contain expected `-bridge@` segment: %q", rt.captured)
	}
}
