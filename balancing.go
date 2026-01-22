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
func StrategyMostRemaining(resource Resource, currentBest, candidate *Transport) *Transport {
	var currentRem, candidateRem uint64
	if currentBest != nil {
		if rate := currentBest.Limits.Load(resource); rate != nil {
			currentRem = rate.Remaining
		}
	}

	if rate := candidate.Limits.Load(resource); rate != nil {
		candidateRem = rate.Remaining
	}

	if candidateRem > currentRem {
		return candidate
	}
	return currentBest
}
