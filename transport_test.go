package ghratelimit

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

// roundTripFunc lets a http.RoundTripper be implemented inline for tests.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTransport_Reserve(t *testing.T) {
	req := &http.Request{
		URL: &url.URL{
			Scheme: "https",
			Host:   "api.github.com",
			Path:   "/users/bored-engineer",
		},
		Method: http.MethodGet,
	}

	var remainingDuringRoundTrip uint64
	transport := &Transport{Reserve: true}
	transport.Base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		remainingDuringRoundTrip = transport.Limits.Load(ResourceCore).Remaining
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"X-Ratelimit-Limit":     []string{"5000"},
				"X-Ratelimit-Used":      []string{"1"},
				"X-Ratelimit-Remaining": []string{"4999"},
				"X-Ratelimit-Reset":     []string{"1745121612"},
				"X-Ratelimit-Resource":  []string{"core"},
			},
		}, nil
	})
	transport.Limits.Store(nil, ResourceCore, &Rate{Limit: 5000, Used: 0, Remaining: 5000, Reset: 1745121612})

	_, err := transport.RoundTrip(req)
	assert.NoError(t, err, "(*Transport).RoundTrip failed")

	// The remaining count should have already been decremented before the base RoundTrip executed.
	assert.Equal(t, uint64(4999), remainingDuringRoundTrip)
	// Once the (real) response headers are parsed, they take precedence.
	assert.Equal(t, &Rate{Limit: 5000, Used: 1, Remaining: 4999, Reset: 1745121612}, transport.Limits.Load(ResourceCore))
}
