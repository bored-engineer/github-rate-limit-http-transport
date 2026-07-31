package ghratelimit

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"time"
)

// BalancingTransport distributes requests across multiple transports, preferring whichever has the
// highest rate of "remaining" requests per second until its rate limit resets. This naturally favors
// transports whose limits are about to reset (since any unused remaining requests are otherwise wasted)
// while still accounting for how much remaining capacity each transport actually has.
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

	now := time.Now()
	var bestTransport *Transport
	var bestScore float64
	for _, transport := range bt {
		rate := transport.Limits.Load(resource)
		if rate == nil || rate.Remaining == 0 {
			continue
		}
		// score is the rate of remaining requests per second until the limit resets, so transports
		// whose limits are about to reset (and would otherwise waste their remaining requests) are
		// preferred over transports with more remaining requests but a more distant reset.
		score := math.Inf(1)
		if secondsUntilReset := time.Unix(int64(rate.Reset), 0).Sub(now).Seconds(); secondsUntilReset > 0 {
			score = float64(rate.Remaining) / secondsUntilReset
		}
		if bestTransport == nil || score > bestScore {
			bestScore = score
			bestTransport = transport
		}
	}

	if bestTransport == nil {
		return bt[rand.Intn(len(bt))].RoundTrip(req)
	}
	return bestTransport.RoundTrip(req)
}
