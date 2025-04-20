package ghratelimit

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRate_UnmarshalJSON(t *testing.T) {
	var rate Rate
	data := []byte(`{"limit":5000,"used":1000,"remaining":4000,"reset":1633036800}`)
	err := json.Unmarshal(data, &rate)
	assert.NoError(t, err, "json.Unmarshal failed")
	assert.Equal(t, rate.Limit.Load(), uint64(5000), "Limit mismatch")
	assert.Equal(t, rate.Used.Load(), uint64(1000), "Used mismatch")
	assert.Equal(t, rate.Remaining.Load(), uint64(4000), "Remaining mismatch")
	assert.Equal(t, rate.Reset.Load(), uint64(1633036800), "Reset mismatch")
}

func TestRate_Parse(t *testing.T) {
	var rate Rate
	err := rate.Parse(http.Header{
		"X-Ratelimit-Limit":     []string{"5000"},
		"X-Ratelimit-Used":      []string{"1000"},
		"X-Ratelimit-Remaining": []string{"4000"},
		"X-Ratelimit-Reset":     []string{"1633036800"},
	})
	assert.NoError(t, err, "(*Rate).Parse failed")
	assert.Equal(t, rate.Limit.Load(), uint64(5000), "Limit mismatch")
	assert.Equal(t, rate.Used.Load(), uint64(1000), "Used mismatch")
	assert.Equal(t, rate.Remaining.Load(), uint64(4000), "Remaining mismatch")
	assert.Equal(t, rate.Reset.Load(), uint64(1633036800), "Reset mismatch")

	err = rate.Parse(http.Header{
		"X-Ratelimit-Limit":     []string{"invalid"},
		"X-Ratelimit-Used":      []string{"invalid"},
		"X-Ratelimit-Remaining": []string{"invalid"},
		"X-Ratelimit-Reset":     []string{"invalid"},
	})
	assert.Error(t, err, "expected error, got nil")
}
