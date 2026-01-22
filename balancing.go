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
type BalancingTransport struct {
	transports []*Transport
	strategy   func(resource Resource, currentBest *Transport, candidate *Transport) *Transport
}

// BalancingOption configures the BalancingTransport
type BalancingOption func(*BalancingTransport)

// NewBalancingTransport creates a new BalancingTransport with the provided transports and options.
func NewBalancingTransport(transports []*Transport, opts ...BalancingOption) *BalancingTransport {
	bt := &BalancingTransport{
		transports: transports,
		strategy:   StrategyMostRemaining,
	}
	for _, opt := range opts {
		opt(bt)
	}
	return bt
}

// WithStrategy configures the strategy used to select the best transport.
func WithStrategy(strategy func(resource Resource, currentBest *Transport, candidate *Transport) *Transport) BalancingOption {
	return func(bt *BalancingTransport) {
		bt.strategy = strategy
	}
}

// Poll calls (*Transport).Poll for every transport
func (bt *BalancingTransport) Poll(ctx context.Context, interval time.Duration, u *url.URL) {
	for _, transport := range bt.transports {
		go transport.Poll(ctx, interval, u)
	}
	<-ctx.Done()
}

// RoundTrip implements http.RoundTripper
func (bt *BalancingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(bt.transports) == 0 {
		return nil, fmt.Errorf("no transports available")
	}

	resource := InferResource(req)
	if resource == "" {
		return nil, fmt.Errorf("unknown resource for request: %q", req.URL)
	}

	var bestTransport *Transport
	for _, t := range bt.transports {
		bestTransport = bt.strategy(resource, bestTransport, t)
	}

	if bestTransport == nil {
		bestTransport = bt.transports[rand.Intn(len(bt.transports))]
	}

	return bestTransport.RoundTrip(req)
}

// StrategyMostRemaining selects the transport with the highest remaining rate limit.
func StrategyMostRemaining(resource Resource, best, candidate *Transport) *Transport {
	bestRem, _ := extractValues(resource, best)
	candidateRem, _ := extractValues(resource, candidate)

	if candidateRem > bestRem {
		return candidate
	}
	return best
}

func StrategyResetTimeInPastAndMostRemaining(resource Resource, best, candidate *Transport) *Transport {
	bestRem, bestReset := extractValues(resource, best)
	candidateRem, candidateReset := extractValues(resource, candidate)
	if candidate != nil {
		if cRate := candidate.Limits.Load(resource); cRate != nil {
			candidateRem = cRate.Remaining
			candidateReset = time.Unix(int64(cRate.Reset), 0)
		}
	}
	// if both are zero remaining, return nil to indicate no best transport
	if bestRem == 0 && candidateRem == 0 {
		return nil
	}
	// prefer the one that is already reset because it can serve more requests now
	if isTimeANonZeroAndBeforeNowAndB(candidateReset, bestReset) {
		return candidate
	}
	if isTimeANonZeroAndBeforeNowAndB(bestReset, candidateReset) {
		return best
	}
	// Otherwise, prefer the non-zero remaining that resets sooner, or the higher remaining if both reset at the same time
	if (candidateReset.Before(bestReset) && candidateRem > 0) || candidateRem > bestRem {
		return candidate
	}
	return best
}

func extractValues(resource Resource, t *Transport) (uint64, time.Time) {
	if t != nil {
		if r := t.Limits.Load(resource); r != nil {
			currentRem := r.Remaining
			currentReset := time.Unix(int64(r.Reset), 0)
			return currentRem, currentReset
		}
	}
	return 0, time.Time{}
}

func isTimeANonZeroAndBeforeNowAndB(a, b time.Time) bool {
	return !a.IsZero() && a.Before(time.Now()) && a.Before(b)
}
