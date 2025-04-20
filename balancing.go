package ghratelimit

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"time"
)

// BalancingTransport distributes requests to the transport with the highest "remaining" rate limit to execute the request.
// This can be used to distributes requests across multiple GitHub authentication tokens or applications.
type BalancingTransport []*Transport

// Poll calls (*Transport).Poll for every transport
func (bt BalancingTransport) Poll(ctx context.Context, interval time.Duration, u *url.URL) {
	for _, transport := range bt {
		go transport.Poll(ctx, interval, u)
	}
	<-ctx.Done()
}

// RoundTrip implements http.RoundTripper
func (bt BalancingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(bt) == 0 {
		return nil, fmt.Errorf("no transports available")
	}

	resource := InferResource(req)
	if resource == "" {
		return nil, fmt.Errorf("unknown resource for request: %q", req.URL)
	}

	var bestTransport *Transport
	var bestRemaining uint64
	for _, transport := range bt {
		if rate := transport.Limits.Load(resource); rate != nil {
			if rate.Remaining > bestRemaining {
				bestRemaining = rate.Remaining
				bestTransport = transport
			}
		}
	}

	if bestTransport == nil {
		return bt[rand.Intn(len(bt))].RoundTrip(req)
	}
	return bestTransport.RoundTrip(req)
}
