package ghratelimit

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Transport updates the Limits field with the most recent rate-limit information as responses from GitHub are executed.
// It implements the http.RoundTripper interface, so it can be used as a base transport for http.Client.
type Transport struct {
	// Base is the base RoundTripper used to make HTTP requests.
	// If nil, http.DefaultTransport is used.
	Base http.RoundTripper
	// Limits is the most recent rate-limit information
	Limits Limits
	// Reserve, if true, proactively decrements the Limits.Remaining count (via Limits.Reserve) for the
	// inferred resource before the request is sent, rather than waiting for the response headers to be parsed.
	// This is useful (for example, alongside BalancingTransport) to avoid routing a burst of concurrent
	// requests to the same transport before its rate-limit headers have been updated by a response.
	Reserve bool
	// Spoof, if true, causes RoundTrip to return a synthetic HTTP 429 response mimicking a typical
	// GitHub rate-limit response, instead of actually executing the request, whenever the Limits already
	// indicate there is no Remaining quota for the inferred resource. This avoids sending a request that
	// is certain to be rejected by GitHub, at the cost of not observing whatever response GitHub itself
	// would have returned.
	Spoof bool
}

// RoundTrip implements http.RoundTripper
func (t *Transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	resource := InferResource(req)
	if t.Spoof {
		if rate := t.Limits.Load(resource); rate != nil && rate.Remaining == 0 {
			return spoofRateLimitResponse(req, resource, rate), nil
		}
	}
	if t.Reserve {
		t.Limits.Reserve(resource)
	}
	if t.Base == nil {
		resp, err = http.DefaultTransport.RoundTrip(req)
	} else {
		resp, err = t.Base.RoundTrip(req)
	}
	if resp != nil {
		// Parse failures (e.g. malformed rate-limit headers) must not discard an otherwise
		// valid response: doing so would drop resp.Body unclosed (leaking the connection) and
		// hide a real response from the caller just because of a header-parsing issue.
		_ = t.Limits.Parse(resp)
	}
	return
}

// spoofRateLimitResponse constructs a synthetic *http.Response mimicking the one GitHub returns when a
// request is rejected because a rate limit has been exhausted, without actually sending the request.
func spoofRateLimitResponse(req *http.Request, resource Resource, rate *Rate) *http.Response {
	body := fmt.Sprintf(
		`{"message":"API rate limit exceeded for resource %q.","documentation_url":"https://docs.github.com/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2022-11-28","status":"429"}`,
		resource,
	)
	header := http.Header{
		"Content-Type":          []string{"application/json; charset=utf-8"},
		"X-Ratelimit-Limit":     []string{strconv.FormatUint(rate.Limit, 10)},
		"X-Ratelimit-Used":      []string{strconv.FormatUint(rate.Used, 10)},
		"X-Ratelimit-Remaining": []string{"0"},
		"X-Ratelimit-Reset":     []string{strconv.FormatUint(rate.Reset, 10)},
		"X-Ratelimit-Resource":  []string{resource.String()},
	}
	if retryAfter := time.Until(time.Unix(int64(rate.Reset), 0)); retryAfter > 0 {
		header.Set("Retry-After", strconv.FormatFloat(retryAfter.Seconds(), 'f', 0, 64))
	}
	return &http.Response{
		Status:        "429 Too Many Requests",
		StatusCode:    http.StatusTooManyRequests,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

// Poll calls (*Transport).Limits.Update every interval, starting immediately.
func (t *Transport) Poll(ctx context.Context, interval time.Duration, u *url.URL) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := t.Limits.Fetch(ctx, t, u); err != nil {
			log.Printf("(*ghratelimit.Transport).Limits.Fetch failed: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
