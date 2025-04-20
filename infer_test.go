package ghtransport

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInferResource(t *testing.T) {
	assert.Equal(t, ResourceCodeSearch, InferResource(&http.Request{
		URL: &url.URL{
			Scheme: "https",
			Host:   "api.github.com",
			Path:   "/search/code",
		},
		Method: http.MethodGet,
	}), "mismatch 'code_search'")
	assert.Equal(t, ResourceCore, InferResource(&http.Request{
		URL: &url.URL{
			Scheme: "https",
			Host:   "api.github.com",
			Path:   "/users/bored-engineer",
		},
		Method: http.MethodGet,
	}), "mismatch  'core'")
}
