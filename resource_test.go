package ghratelimit

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseResource(t *testing.T) {
	resource := ParseResource(http.Header{
		"X-Ratelimit-Resource": []string{"core"},
	})
	assert.Equal(t, ResourceCore, resource, "mismatch")
}
