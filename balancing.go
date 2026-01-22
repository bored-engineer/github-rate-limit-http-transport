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
// Uses only one lookup per transport and avoids time conversions to minimize overhead.
func StrategyMostRemaining(resource Resource, best, candidate *Transport) *Transport {
	bestRem, _ := extractValues(resource, best)
	candidateRem, _ := extractValues(resource, candidate)

	if candidateRem > bestRem {
		return candidate
	}
	return best
}

// StrategyResetTimeInPastAndMostRemaining prefers transports whose reset is already in the past,
// then earlier resets, and finally the one with the most remaining tokens. Returns nil when both// transports have zero remaining, signaling no immediate capacity.
func StrategyResetTimeInPastAndMostRemaining(resource Resource, best, candidate *Transport) *Transport {
	bestRem, bestReset := extractValues(resource, best)
	candidateRem, candidateReset := extractValues(resource, candidate)

	// Fast path: both have zero remaining, no usable transport right now.
	if bestRem == 0 && candidateRem == 0 {
		return nil
	}

	// If one transport has already reset (reset time in the past) and the other hasn't,
	// prefer the one that reset first because it can serve immediately.
	if resetIsInPastAndEarlierThanOther(candidateReset, bestReset) {
		return candidate
	}
	if resetIsInPastAndEarlierThanOther(bestReset, candidateReset) {
		return best
	}

	// When both resets are in the future (or zero), prefer the earlier reset if it also has capacity.
	if candidateReset != 0 && bestReset != 0 {
		if candidateReset < bestReset && candidateRem > 0 {
			return candidate
		}
		if bestReset < candidateReset && bestRem > 0 {
			return best
		}
	}

	// Fallback to the transport with more remaining tokens.
	if candidateRem > bestRem {
		return candidate
	}
	return best
}

// extractValues reads remaining tokens and reset epoch seconds for a transport.
// Returns zero values when the transport or limit is missing (no allocation occurs).
func extractValues(resource Resource, t *Transport) (uint64, int64) {
	if t != nil {
		if r := t.Limits.Load(resource); r != nil {
			return r.Remaining, int64(r.Reset)
		}
	}
	return 0, 0
}

// resetIsInPastAndEarlierThanOther returns true when `reset` is non-zero, already in the past
// relative to `now`, and either the other reset is zero or occurs later.
func resetIsInPastAndEarlierThanOther(reset, otherReset int64) bool {
	return reset != 0 && reset < time.Now().Unix() && (otherReset == 0 || reset < otherReset)
}
