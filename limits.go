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
	m sync.Map // Resource -> *Rate
	// Notify is called when a new rate limit is stored.
	// It can be a useful hook to update metric gauges.
	Notify func(*http.Response, Resource, *Rate)
}

// Store the rate limit for the given resource type, unless doing so would regress to an earlier
// reset window: an update reporting an older Reset than what's already known is assumed to be a
// stale or out-of-order read (e.g. from an eventually consistent /rate_limit response, or from a
// concurrent request whose response simply arrived after a fresher one) and is dropped. Notify is
// not invoked when the update is dropped, since nothing changed.
//
// Within the same (or a later) window, any update is accepted unconditionally - including one
// reporting more Remaining than what's already known. This intentionally doesn't try to protect
// against every kind of same-window staleness (Remaining can jitter non-monotonically as
// concurrent responses complete out of order, and Reserve's own optimistic decrements are no
// longer risk of being mistaken for one), in exchange for a much simpler implementation.
func (l *Limits) Store(resp *http.Response, resource Resource, rate *Rate) {
	for {
		existing := l.Load(resource)
		if existing == nil {
			if _, loaded := l.m.LoadOrStore(resource, rate); !loaded {
				break
			}
			continue
		}
		if rate.Reset < existing.Reset {
			return
		}
		if l.m.CompareAndSwap(resource, existing, rate) {
			break
		}
	}
	if l.Notify != nil {
		l.Notify(resp, resource, rate)
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

// Reserve proactively decrements the Remaining count (and increments Used) by one for the given resource, if known.
// This is useful to optimistically account for an in-flight request before its response (and updated rate-limit
// headers) has been received, to avoid a burst of concurrent requests overrunning the actual rate limit.
// A CompareAndSwap loop is used so concurrent calls don't lose updates to one another.
// Unlike Store, this does not invoke Notify, since the estimate is superseded by the real rate limit once available.
func (l *Limits) Reserve(resource Resource) {
	for {
		rate := l.Load(resource)
		if rate == nil || rate.Remaining == 0 {
			return
		}
		updated := &Rate{
			Limit:     rate.Limit,
			Used:      rate.Used + 1,
			Remaining: rate.Remaining - 1,
			Reset:     rate.Reset,
		}
		if l.m.CompareAndSwap(resource, rate, updated) {
			return
		}
	}
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

// Parse updates the rate limits based on the provided HTTP response.
func (l *Limits) Parse(resp *http.Response) error {
	resource := ParseResource(resp.Header)
	if resource == "" {
		return nil // possibly a error or an endpoint without a rate-limit
	}
	rate, err := ParseRate(resp.Header)
	if err != nil {
		return err
	}
	l.Store(resp, resource, &rate)
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
	req.Header.Set("User-Agent", "github.com/bored-engineer/github-rate-limit-http-transport")
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
		return fmt.Errorf("(*http.Response).StatusCode(%d) != 200 for %q: %s", resp.StatusCode, u, string(body))
	}

	var limits struct {
		Resources map[Resource]Rate `json:"resources"`
	}

	if err := json.Unmarshal(body, &limits); err != nil {
		return fmt.Errorf("json.Unmarshal for %q failed: %w", u, err)
	}

	for resource, rate := range limits.Resources {
		l.Store(resp, resource, &rate)
	}

	return nil
}
