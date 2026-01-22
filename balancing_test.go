package ghratelimit

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

type mockBalancingRoundTripper struct {
	resp      *http.Response
	roundTrip func(*http.Request) (*http.Response, error)
	err       error
	req       *http.Request
}

func (m *mockBalancingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.req = req
	if m.roundTrip != nil {
		return m.roundTrip(req)
	}
	return m.resp, m.err
}

func TestBalancingTransport_RoundTrip(t *testing.T) {
	// Helper to create a dummy request
	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)

	t.Run("no transports", func(t *testing.T) {
		bt := NewBalancingTransport(nil)
		_, err := bt.RoundTrip(req)
		if err == nil {
			t.Error("expected error when no transports available, got nil")
		}
	})

	t.Run("selects transport with highest remaining limit", func(t *testing.T) {
		m1 := &mockBalancingRoundTripper{resp: &http.Response{StatusCode: 200}}
		t1 := NewTransport(m1)
		t1.Limits.Store(nil, ResourceCore, &Rate{Remaining: 10})

		m2 := &mockBalancingRoundTripper{resp: &http.Response{StatusCode: 200}}
		t2 := NewTransport(m2)
		t2.Limits.Store(nil, ResourceCore, &Rate{Remaining: 100}) // Highest

		m3 := &mockBalancingRoundTripper{resp: &http.Response{StatusCode: 200}}
		t3 := NewTransport(m3)
		t3.Limits.Store(nil, ResourceCore, &Rate{Remaining: 50})

		bt := NewBalancingTransport([]*Transport{t1, t2, t3})

		resp, err := bt.RoundTrip(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		if m2.req == nil {
			t.Error("expected transport 2 to be used, but it wasn't")
		}
		if m1.req != nil {
			t.Error("transport 1 should not be used")
		}
		if m3.req != nil {
			t.Error("transport 3 should not be used")
		}
	})

	t.Run("fallbacks to random when no limits known", func(t *testing.T) {
		m1 := &mockBalancingRoundTripper{resp: &http.Response{StatusCode: 200}}
		t1 := NewTransport(m1)

		m2 := &mockBalancingRoundTripper{resp: &http.Response{StatusCode: 200}}
		t2 := NewTransport(m2)

		bt := NewBalancingTransport([]*Transport{t1, t2})

		// Since it's random, we can't be deterministic which one is picked,
		// but one of them MUST be picked.
		resp, err := bt.RoundTrip(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		if m1.req == nil && m2.req == nil {
			t.Error("expected at least one transport to be used")
		}
		if m1.req != nil && m2.req != nil {
			t.Error("only one transport should be used")
		}
	})

	t.Run("handles mixed known and unknown limits", func(t *testing.T) {
		// Transport 1: no info
		m1 := &mockBalancingRoundTripper{resp: &http.Response{StatusCode: 200}}
		t1 := NewTransport(m1)

		// Transport 2: 10 remaining
		m2 := &mockBalancingRoundTripper{resp: &http.Response{StatusCode: 200}}
		t2 := NewTransport(m2)
		t2.Limits.Store(nil, ResourceCore, &Rate{Remaining: 10})

		bt := NewBalancingTransport([]*Transport{t1, t2})

		// It should pick T2 because it has a known positive limit which is implicitly better than unknown?
		// Let's check logic:
		// bestRemaining starts at 0.
		// T1: load -> nil.
		// T2: load -> 10. 10 > 0. bestTransport = T2.
		// Should pick T2.

		resp, err := bt.RoundTrip(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		if m2.req == nil {
			t.Error("expected transport 2 (known limit) to be used over unknown")
		}
	})

	t.Run("resource inference", func(t *testing.T) {
		// Test that it uses the correct resource limit for decision
		// /search/users -> ResourceSearch
		searchReq, _ := http.NewRequest("GET", "https://api.github.com/search/users?q=foo", nil)

		m1 := &mockBalancingRoundTripper{resp: &http.Response{StatusCode: 200}}
		t1 := NewTransport(m1)
		t1.Limits.Store(nil, ResourceCore, &Rate{Remaining: 100})  // high core
		t1.Limits.Store(nil, ResourceSearch, &Rate{Remaining: 10}) // low search

		m2 := &mockBalancingRoundTripper{resp: &http.Response{StatusCode: 200}}
		t2 := NewTransport(m2)
		t2.Limits.Store(nil, ResourceCore, &Rate{Remaining: 10})    // low core
		t2.Limits.Store(nil, ResourceSearch, &Rate{Remaining: 100}) // high search

		bt := NewBalancingTransport([]*Transport{t1, t2})

		_, err := bt.RoundTrip(searchReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// API is search, so should use ResourceSearch limits.
		// T1 Search: 10. T2 Search: 100.
		// Should pick T2.
		if m2.req == nil {
			t.Error("expected transport 2 (better search limit) to be used")
		}
	})
}

func TestBalancingTransport_Poll(t *testing.T) {
	// This test just ensures no panic or hang.
	// We use a mock transport to avoid making real network requests and to ensure
	// Fetch returns successfully and quickly, so we don't trigger "context canceled" errors
	// during the Fetch call when we cancel the context.

	mockTransport := &mockBalancingRoundTripper{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString(`{"resources": {"core": {"limit": 5000, "remaining": 4999}}}`)),
				Header:     make(http.Header),
			}, nil
		},
	}

	// We need to provide a body literal for JSON unmarshalling if Limits.Fetch expects it.
	// Limits.Fetch expects `resources` key.
	// However, if we just want to avoid error in the network call layer:
	// If the body is empty, Parse might fail, but that's a different error.
	// Let's provide a minimal valid body.
	// But `mockBalancingRoundTripper` doesn't support Body content easily in the struct I defined?
	// Wait, I defined it with *http.Request/Response.
	// I can modify the test to return a body.

	bt := NewBalancingTransport([]*Transport{
		NewTransport(mockTransport),
		NewTransport(mockTransport),
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool)
	go func() {
		bt.Poll(ctx, time.Hour, nil)
		close(done)
	}()

	// Give Poll a tiny bit of time to start and hit the select,
	// although strictly not required if we just want to ensure it finishes.
	// But if we want to avoid the "context canceled" log from Fetch,
	// we rely on Fetch being fast (mocked) and finishing before cancel() is called.
	// The mock above returns immediately.
	// But we are in a race.
	// To be absolutely sure, we could wait a tiny bit, or just rely on the scheduler.
	// Using a minimal sleep increases reliability of "silence" but correctneess is guaranteed by `done`.
	time.Sleep(1 * time.Millisecond)

	cancel()
	select {
	case <-done:
		// success
	case <-time.After(time.Second):
		t.Error("Poll did not return after context cancellation")
	}
}

func TestNewBalancingTransport(t *testing.T) {
	t.Run("default strategy", func(t *testing.T) {
		bt := NewBalancingTransport(nil)
		if bt.strategy == nil {
			t.Error("expected default strategy to be set")
		}
	})

	t.Run("WithStrategy", func(t *testing.T) {
		called := false
		customStrategy := func(resource Resource, currentBest, candidate *Transport) *Transport {
			called = true
			return candidate
		}

		m := &mockBalancingRoundTripper{resp: &http.Response{StatusCode: 200}}
		bt := NewBalancingTransport([]*Transport{NewTransport(m)}, WithStrategy(customStrategy))

		req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
		_, _ = bt.RoundTrip(req)

		if !called {
			t.Error("custom strategy was not used")
		}
	})
}

// Ensure mockRoundTripper implements http.RoundTripper
var _ http.RoundTripper = &mockRoundTripper{}
