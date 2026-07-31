package ghratelimit

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"time"
)

// Transport updates the Limits field with the most recent rate-limit information as responses from GitHub are executed.
// It implements the http.RoundTripper interface, so it can be used as a base transport for http.Client.
type Transport struct {
	// Base is the base RoundTripper used to make HTTP requests.
	// If nil, http.DefaultTransport is used.
	Base http.RoundTripper
	// Limits is the most recent rate-limit information
	Limits Limits
	// Reserve, if true, proactively decrements the Limits.Remaining count (via Limits.Reserve) for the
	// inferred resource before the request is sent, rather than waiting for the response headers to be parsed.
	// This is useful (for example, alongside BalancingTransport) to avoid routing a burst of concurrent
	// requests to the same transport before its rate-limit headers have been updated by a response.
	Reserve bool
}

// RoundTrip implements http.RoundTripper
func (t *Transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	if t.Reserve {
		t.Limits.Reserve(InferResource(req))
	}
	if t.Base == nil {
		resp, err = http.DefaultTransport.RoundTrip(req)
	} else {
		resp, err = t.Base.RoundTrip(req)
	}
	if resp != nil {
		if err := t.Limits.Parse(resp); err != nil {
			return nil, err
		}
	}
	return
}

// Poll calls (*Transport).Limits.Update every interval, starting immediately.
func (t *Transport) Poll(ctx context.Context, interval time.Duration, u *url.URL) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := t.Limits.Fetch(ctx, t, u); err != nil {
			log.Printf("(*ghratelimit.Transport).Limits.Fetch failed: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
