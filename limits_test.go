package ghtransport

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// https://api.github.com/rate_limit
const limitsResponse = `{
  "resources": {
    "core": {
      "limit": 5000,
      "used": 0,
      "remaining": 5000,
      "reset": 1745121612
    },
    "search": {
      "limit": 30,
      "used": 0,
      "remaining": 30,
      "reset": 1745118072
    },
    "graphql": {
      "limit": 5000,
      "used": 0,
      "remaining": 5000,
      "reset": 1745121612
    },
    "integration_manifest": {
      "limit": 5000,
      "used": 0,
      "remaining": 5000,
      "reset": 1745121612
    },
    "source_import": {
      "limit": 100,
      "used": 0,
      "remaining": 100,
      "reset": 1745118072
    },
    "code_scanning_upload": {
      "limit": 1000,
      "used": 0,
      "remaining": 1000,
      "reset": 1745121612
    },
    "code_scanning_autofix": {
      "limit": 10,
      "used": 0,
      "remaining": 10,
      "reset": 1745118072
    },
    "actions_runner_registration": {
      "limit": 10000,
      "used": 0,
      "remaining": 10000,
      "reset": 1745121612
    },
    "scim": {
      "limit": 15000,
      "used": 0,
      "remaining": 15000,
      "reset": 1745121612
    },
    "dependency_snapshots": {
      "limit": 100,
      "used": 0,
      "remaining": 100,
      "reset": 1745118072
    },
    "audit_log": {
      "limit": 1750,
      "used": 0,
      "remaining": 1750,
      "reset": 1745121612
    },
    "audit_log_streaming": {
      "limit": 15,
      "used": 0,
      "remaining": 15,
      "reset": 1745121612
    },
    "code_search": {
      "limit": 10,
      "used": 0,
      "remaining": 10,
      "reset": 1745118072
    }
  },
  "rate": {
    "limit": 5000,
    "used": 0,
    "remaining": 5000,
    "reset": 1745121612
  }
}`

func TestLimits_UnmarshalJSON(t *testing.T) {
	var resp struct {
		Resources Limits `json:"resources"`
	}
	err := json.Unmarshal([]byte(limitsResponse), &resp)
	assert.NoError(t, err, "json.Unmarshal failed")
	expected := map[Resource]*Rate{
		ResourceCore:                      NewRate(5000, 0, 5000, 1745121612),
		ResourceSearch:                    NewRate(30, 0, 30, 1745118072),
		ResourceGraphQL:                   NewRate(5000, 0, 5000, 1745121612),
		ResourceIntegrationManifest:       NewRate(5000, 0, 5000, 1745121612),
		ResourceSourceImport:              NewRate(100, 0, 100, 1745118072),
		ResourceCodeScanningUpload:        NewRate(1000, 0, 1000, 1745121612),
		ResourceCodeScanningAutofix:       NewRate(10, 0, 10, 1745118072),
		ResourceActionsRunnerRegistration: NewRate(10000, 0, 10000, 1745121612),
		ResourceSCIM:                      NewRate(15000, 0, 15000, 1745121612),
		ResourceDependencySnapshots:       NewRate(100, 0, 100, 1745118072),
		ResourceAuditLog:                  NewRate(1750, 0, 1750, 1745121612),
		ResourceAuditLogStreaming:         NewRate(15, 0, 15, 1745121612),
		ResourceCodeSearch:                NewRate(10, 0, 10, 1745118072),
	}
	actual := maps.Collect(resp.Resources.Iter())
	assert.Equal(t, len(expected), len(actual), "length mismatch")
	for resource, got := range actual {
		want := expected[resource]
		assert.True(t, want.Equal(got), "value mismatch, expected %s, got %s", want, got)
	}
}

func TestLimits_Parse(t *testing.T) {
	var limits Limits
	err := limits.Parse(http.Header{
		"X-Ratelimit-Limit":     []string{"5000"},
		"X-Ratelimit-Used":      []string{"0"},
		"X-Ratelimit-Remaining": []string{"5000"},
		"X-Ratelimit-Reset":     []string{"1745121612"},
		"X-Ratelimit-Resource":  []string{"core"},
	})
	assert.NoError(t, err, "(*Limits).Parse failed")
	assert.Equal(t, uint64(5000), limits.Core.Limit.Load(), "Limit mismatch")
	assert.Equal(t, uint64(0), limits.Core.Used.Load(), "Used mismatch")
	assert.Equal(t, uint64(5000), limits.Core.Remaining.Load(), "Remaining mismatch")
	assert.Equal(t, uint64(1745121612), limits.Core.Reset.Load(), "Reset mismatch")

	err = limits.Parse(http.Header{
		"X-Ratelimit-Limit":     []string{"invalid"},
		"X-Ratelimit-Used":      []string{"invalid"},
		"X-Ratelimit-Remaining": []string{"invalid"},
		"X-Ratelimit-Reset":     []string{"invalid"},
		"X-Ratelimit-Resource":  []string{"invalid"},
	})
	assert.Error(t, err, "expected error, got nil")
}
