package ghratelimit

import (
	"io"
	"net/http"
	"net/url"
	"strings"
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

// closeTrackingBody wraps an io.Reader and records whether Close was called on it.
type closeTrackingBody struct {
	io.Reader
	closed bool
}

func (b *closeTrackingBody) Close() error {
	b.closed = true
	return nil
}

func TestTransport_RoundTrip_MalformedHeadersDoNotDropResponse(t *testing.T) {
	req := &http.Request{
		URL: &url.URL{
			Scheme: "https",
			Host:   "api.github.com",
			Path:   "/users/bored-engineer",
		},
		Method: http.MethodGet,
	}

	body := &closeTrackingBody{Reader: strings.NewReader("real payload")}
	transport := &Transport{
		Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					// X-Ratelimit-Resource is present but the other rate-limit headers are malformed,
					// so (*Limits).Parse will fail: that must not cause the response itself to be lost.
					"X-Ratelimit-Resource":  []string{"core"},
					"X-Ratelimit-Remaining": []string{"not-a-number"},
				},
				Body: body,
			}, nil
		}),
	}

	resp, err := transport.RoundTrip(req)
	assert.NoError(t, err, "(*Transport).RoundTrip should not fail because of malformed rate-limit headers")
	if assert.NotNil(t, resp, "the real response must still be returned") {
		got, readErr := io.ReadAll(resp.Body)
		assert.NoError(t, readErr)
		assert.Equal(t, "real payload", string(got), "the caller must still be able to read the real body")
		assert.NoError(t, resp.Body.Close())
		assert.True(t, body.closed, "the caller must be able to close the real body")
	}
}
