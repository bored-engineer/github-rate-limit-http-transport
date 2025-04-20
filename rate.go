package ghratelimit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
)

// Rate represents the rate limit information for a given resource type.
type Rate struct {
	// The maximum number of requests that you can make per hour.
	Limit atomic.Uint64
	// The number of requests you have made in the current rate limit window.
	Used atomic.Uint64
	// The number of requests remaining in the current rate limit window.
	Remaining atomic.Uint64
	// The time at which the current rate limit window resets, in UTC epoch seconds.
	Reset atomic.Uint64
}

// UnmarshalJSON implements json.Unmarshaler
func (r *Rate) UnmarshalJSON(data []byte) error {
	var parsed struct {
		Limit     uint64 `json:"limit"`
		Used      uint64 `json:"used"`
		Remaining uint64 `json:"remaining"`
		Reset     uint64 `json:"reset"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	r.Limit.Store(parsed.Limit)
	r.Used.Store(parsed.Used)
	r.Remaining.Store(parsed.Remaining)
	r.Reset.Store(parsed.Reset)
	return nil
}

// MarshalJSON implements json.Marshaler
func (r *Rate) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Limit     uint64 `json:"limit"`
		Used      uint64 `json:"used"`
		Remaining uint64 `json:"remaining"`
		Reset     uint64 `json:"reset"`
	}{
		Limit:     r.Limit.Load(),
		Used:      r.Used.Load(),
		Remaining: r.Remaining.Load(),
		Reset:     r.Reset.Load(),
	})
}

// String implements fmt.Stringer
func (r *Rate) String() string {
	b, _ := r.MarshalJSON()
	return string(b)
}

// Parse extracts the rate limit information from the HTTP response headers.
func (r *Rate) Parse(headers http.Header) error {
	if val, err := strconv.ParseUint(headers.Get("X-RateLimit-Limit"), 10, 64); err != nil {
		return fmt.Errorf("failed to parse X-RateLimit-Limit header: %w", err)
	} else {
		r.Limit.Store(val)
	}
	if val, err := strconv.ParseUint(headers.Get("X-RateLimit-Used"), 10, 64); err != nil {
		return fmt.Errorf("failed to parse X-RateLimit-Used header: %w", err)
	} else {
		r.Used.Store(val)
	}
	if val, err := strconv.ParseUint(headers.Get("X-RateLimit-Remaining"), 10, 64); err != nil {
		return fmt.Errorf("failed to parse X-RateLimit-Remaining header: %w", err)
	} else {
		r.Remaining.Store(val)
	}
	if val, err := strconv.ParseUint(headers.Get("X-RateLimit-Reset"), 10, 64); err != nil {
		return fmt.Errorf("failed to parse X-RateLimit-Reset header: %w", err)
	} else {
		r.Reset.Store(val)
	}
	return nil
}

// Equal compares the values of two Rate instances for equality.
func (a *Rate) Equal(b *Rate) bool {
	return a.Limit.Load() == b.Limit.Load() &&
		a.Used.Load() == b.Used.Load() &&
		a.Remaining.Load() == b.Remaining.Load() &&
		a.Reset.Load() == b.Reset.Load()
}

// NewRate creates a new Rate instance with the given values.
func NewRate(limit, used, remaining, reset uint64) *Rate {
	var rate Rate
	rate.Limit.Store(limit)
	rate.Used.Store(used)
	rate.Remaining.Store(remaining)
	rate.Reset.Store(reset)
	return &rate
}
