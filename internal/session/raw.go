package session

import (
	"sync"

	"github.com/go-resty/resty/v2"
)

// rawClient is a thin resty wrapper that shares its bearer token with the
// owning Session. setBearer is called from Session under its own mutex; the
// inner mutex here serializes header swaps against in-flight R() calls.
type rawClient struct {
	mu   sync.RWMutex
	rc   *resty.Client
	bear string
}

func newRawClient(baseURL string) *rawClient {
	rc := resty.New().
		SetBaseURL(baseURL).
		SetHeader("Accept", "application/vnd.protonmail.v1+json").
		SetHeader("x-pm-appversion", appVersionHeader())
	return &rawClient{rc: rc}
}

func (r *rawClient) setBearer(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bear = token
	if token == "" {
		r.rc.Header.Del("Authorization")
	} else {
		r.rc.SetHeader("Authorization", "Bearer "+token)
	}
}

func (r *rawClient) R() *resty.Request {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.rc.R()
}

// hasBearer reports whether a non-empty bearer token is currently set.
func (r *rawClient) hasBearer() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bear != ""
}
