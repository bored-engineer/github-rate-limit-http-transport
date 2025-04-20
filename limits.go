package ghratelimit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// DefaultURL is the default URL used to poll rate limits.
// It is set to https://api.github.com/rate_limit.
var DefaultURL = &url.URL{
	Scheme: "https",
	Host:   "api.github.com",
	Path:   "/rate_limit",
}

// Limits represents the rate limits for all known resource types.
type Limits struct {
	m sync.Map
	// Notify is called when a new rate limit is stored.
	// It can be a useful hook to update metric gauges.
	Notify func(Resource, *Rate)
}

// UnmarshalJSON implements json.Unmarshaler.
func (l *Limits) UnmarshalJSON(data []byte) error {
	var resources map[Resource]Rate
	if err := json.Unmarshal(data, &resources); err != nil {
		return err
	}
	for resource, rate := range resources {
		l.Store(resource, &rate)
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (l *Limits) MarshalJSON() ([]byte, error) {
	resources := make(map[Resource]*Rate)
	for resource, rate := range l.Iter() {
		resources[resource] = rate
	}
	return json.Marshal(resources)
}

// Store the rate limit for the given resource type.
func (l *Limits) Store(resource Resource, rate *Rate) {
	l.m.Store(resource, rate)
	if l.Notify != nil {
		l.Notify(resource, rate)
	}
}

// Load the rate-limit for the given resource type.
func (l *Limits) Load(resource Resource) *Rate {
	val, ok := l.m.Load(resource)
	if !ok {
		return nil
	}
	r, ok := val.(*Rate)
	if !ok {
		return nil
	}
	return r
}

// Iter loops over the resource types and yields each resource type and its rate limit.
func (l *Limits) Iter() iter.Seq2[Resource, *Rate] {
	return func(yield func(Resource, *Rate) bool) {
		l.m.Range(func(key, value any) bool {
			resource, ok := key.(Resource)
			if !ok {
				return false
			}
			rate, ok := value.(*Rate)
			if !ok {
				return false
			}
			return yield(resource, rate)
		})
	}
}

// String implements fmt.Stringer
func (l *Limits) String() string {
	var sb strings.Builder
	sb.WriteString("Limits{")
	first := true
	for resource, rate := range l.Iter() {
		if !first {
			sb.WriteString(", ")
		}
		first = false
		sb.WriteString(resource.String())
		sb.WriteString(": ")
		sb.WriteString(rate.String())
	}
	sb.WriteString("}")
	return sb.String()
}

// Parse updates the rate limits based on the provided HTTP headers.
func (l *Limits) Parse(headers http.Header) error {
	resource := ParseResource(headers)
	if resource == "" {
		return nil // possibly a error or an endpoint without a rate-limit
	}
	rate, err := ParseRate(headers)
	if err != nil {
		return err
	}
	l.Store(resource, &rate)
	return nil
}

// Fetch the latest rate limits from the GitHub API and update the Limits instance.
// If the provided URL is nil, it defaults to DefaultURL (https://api.github.com/rate_limit).
func (l *Limits) Fetch(ctx context.Context, transport http.RoundTripper, u *url.URL) error {
	if u == nil {
		u = DefaultURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("http.NewRequestWithContext for %q failed: %w", u, err)
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("(http.RoundTripper).RoundTrip for %q failed: %w", u, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("(*http.Response).Body.Read for %q failed: %w", u, err)
	}
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("(*http.Response).Body.Close for %q failed: %w", u, err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("(http.RoundTripper).RoundTrip for %q failed (%d): %s", u, resp.StatusCode, string(body))
	}

	if err := json.Unmarshal(body, &struct {
		Resources *Limits `json:"resources"`
	}{
		Resources: l,
	}); err != nil {
		return fmt.Errorf("json.Unmarshal for %q failed: %w", u, err)
	}

	return nil
}
