package ghratelimit

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBalancingTransport_RoundTrip(t *testing.T) {
	req := &http.Request{
		URL: &url.URL{
			Scheme: "https",
			Host:   "api.github.com",
			Path:   "/users/bored-engineer",
		},
		Method: http.MethodGet,
	}

	now := time.Now()

	newTransport := func(remaining uint64, reset time.Time) *Transport {
		transport := &Transport{}
		transport.Limits.Store(nil, ResourceCore, &Rate{
			Limit:     5000,
			Remaining: remaining,
			Reset:     uint64(reset.Unix()),
		})
		return transport
	}

	// Fewer remaining requests, but resets much sooner: higher requests/second, so it should win.
	soonButSmall := newTransport(10, now.Add(1*time.Second))
	// More remaining requests, but resets much later: lower requests/second.
	laterButLarge := newTransport(4000, now.Add(1*time.Hour))

	bt := BalancingTransport{laterButLarge, soonButSmall}
	var used *Transport
	for _, transport := range bt {
		transport.Base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			used = transport
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}}, nil
		})
	}

	_, err := bt.RoundTrip(req)
	assert.NoError(t, err, "(BalancingTransport).RoundTrip failed")
	assert.Same(t, soonButSmall, used, "expected the transport with the higher remaining/second rate to be used")
}

func TestBalancingTransport_RoundTrip_SkipsExhausted(t *testing.T) {
	req := &http.Request{
		URL: &url.URL{
			Scheme: "https",
			Host:   "api.github.com",
			Path:   "/users/bored-engineer",
		},
		Method: http.MethodGet,
	}

	now := time.Now()

	exhausted := &Transport{}
	exhausted.Limits.Store(nil, ResourceCore, &Rate{Limit: 5000, Remaining: 0, Reset: uint64(now.Add(time.Second).Unix())})

	available := &Transport{}
	available.Limits.Store(nil, ResourceCore, &Rate{Limit: 5000, Remaining: 1, Reset: uint64(now.Add(time.Hour).Unix())})

	bt := BalancingTransport{exhausted, available}
	var used *Transport
	for _, transport := range bt {
		transport.Base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			used = transport
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}}, nil
		})
	}

	_, err := bt.RoundTrip(req)
	assert.NoError(t, err, "(BalancingTransport).RoundTrip failed")
	assert.Same(t, available, used, "expected the exhausted transport to be skipped")
}

func TestBalancingTransport_RoundTrip_NoData(t *testing.T) {
	req := &http.Request{
		URL: &url.URL{
			Scheme: "https",
			Host:   "api.github.com",
			Path:   "/users/bored-engineer",
		},
		Method: http.MethodGet,
	}

	transport := &Transport{}
	transport.Base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}}, nil
	})

	bt := BalancingTransport{transport}
	_, err := bt.RoundTrip(req)
	assert.NoError(t, err, "(BalancingTransport).RoundTrip failed")
}

func TestBalancingTransport_RoundTrip_NoTransports(t *testing.T) {
	req := &http.Request{
		URL: &url.URL{
			Scheme: "https",
			Host:   "api.github.com",
			Path:   "/users/bored-engineer",
		},
		Method: http.MethodGet,
	}

	var bt BalancingTransport
	_, err := bt.RoundTrip(req)
	assert.Error(t, err, "expected an error when no transports are available")
}
