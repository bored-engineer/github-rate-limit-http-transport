package ghratelimit

import (
	"context"
	"fmt"
	"log"
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
		logStrategyDecision("most_remaining", "candidate has more remaining", bestRem, 0, candidateRem, 0)
		return candidate
	}

	logStrategyDecision("most_remaining", "keeping current best (greater or equal remaining)", bestRem, 0, candidateRem, 0)
	return best
}

// StrategyResetTimeInPastAndMostRemaining prefers transports whose reset is already in the past,
// then earlier resets, and finally the one with the most remaining tokens. Returns nil when both// transports have zero remaining, signaling no immediate capacity.
func StrategyResetTimeInPastAndMostRemaining(resource Resource, best, candidate *Transport) *Transport {
	bestRem, bestReset := extractValues(resource, best)
	candidateRem, candidateReset := extractValues(resource, candidate)

	// Fast path: both have zero remaining, no usable transport right now.
	if bestRem == 0 && candidateRem == 0 {
		logStrategyDecision("reset_time_in_past_and_most_remaining", "both have zero remaining; returning nil", bestRem, bestReset, candidateRem, candidateReset)
		return nil
	}

	// If one transport has already reset (reset time in the past) and the other hasn't,
	// prefer the one that reset first because it can serve immediately.
	if resetIsInPastAndEarlierThanOther(candidateReset, bestReset) {
		logStrategyDecision("reset_time_in_past_and_most_remaining", "candidate reset is earlier and already past", bestRem, bestReset, candidateRem, candidateReset)
		return candidate
	}
	if resetIsInPastAndEarlierThanOther(bestReset, candidateReset) {
		logStrategyDecision("reset_time_in_past_and_most_remaining", "best reset is earlier and already past", bestRem, bestReset, candidateRem, candidateReset)
		return best
	}

	// When both resets are in the future (or zero), prefer the earlier reset if it also has capacity.
	if candidateReset != 0 && bestReset != 0 {
		if candidateReset < bestReset && candidateRem > 0 {
			logStrategyDecision("reset_time_in_past_and_most_remaining", "both in future; candidate resets sooner with capacity", bestRem, bestReset, candidateRem, candidateReset)
			return candidate
		}
		if bestReset < candidateReset && bestRem > 0 {
			logStrategyDecision("reset_time_in_past_and_most_remaining", "both in future; best resets sooner with capacity", bestRem, bestReset, candidateRem, candidateReset)
			return best
		}
	}

	// Fallback to the transport with more remaining tokens.
	if candidateRem > bestRem {
		logStrategyDecision("reset_time_in_past_and_most_remaining", "candidate has more remaining; fallback path", bestRem, bestReset, candidateRem, candidateReset)
		return candidate
	}

	logStrategyDecision("reset_time_in_past_and_most_remaining", "keeping current best; candidate not better", bestRem, bestReset, candidateRem, candidateReset)
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

// logStrategyDecision emits debug information for strategy choices. Kept lightweight to minimize
// overhead; callers pass already-read values to avoid extra lookups.
func logStrategyDecision(strategy, reason string, bestRem uint64, bestReset int64, candidateRem uint64, candidateReset int64) {
	log.Printf("[strategy=%s] %s bestRemaining=%d bestReset=%d candidateRemaining=%d candidateReset=%d", strategy, reason, bestRem, bestReset, candidateRem, candidateReset)
}
