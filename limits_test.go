package ghratelimit

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

func TestLimits_Store(t *testing.T) {
	var resp struct {
		Resources map[Resource]Rate `json:"resources"`
	}
	err := json.Unmarshal([]byte(limitsResponse), &resp)
	assert.NoError(t, err, "json.Unmarshal failed")
	var limits Limits
	for resource, rate := range resp.Resources {
		limits.Store(nil, resource, &rate)
	}
	assert.Equal(t, map[Resource]*Rate{
		ResourceCore:                      {Limit: 5000, Used: 0, Remaining: 5000, Reset: 1745121612},
		ResourceSearch:                    {Limit: 30, Used: 0, Remaining: 30, Reset: 1745118072},
		ResourceGraphQL:                   {Limit: 5000, Used: 0, Remaining: 5000, Reset: 1745121612},
		ResourceIntegrationManifest:       {Limit: 5000, Used: 0, Remaining: 5000, Reset: 1745121612},
		ResourceSourceImport:              {Limit: 100, Used: 0, Remaining: 100, Reset: 1745118072},
		ResourceCodeScanningUpload:        {Limit: 1000, Used: 0, Remaining: 1000, Reset: 1745121612},
		ResourceCodeScanningAutofix:       {Limit: 10, Used: 0, Remaining: 10, Reset: 1745118072},
		ResourceActionsRunnerRegistration: {Limit: 10000, Used: 0, Remaining: 10000, Reset: 1745121612},
		ResourceSCIM:                      {Limit: 15000, Used: 0, Remaining: 15000, Reset: 1745121612},
		ResourceDependencySnapshots:       {Limit: 100, Used: 0, Remaining: 100, Reset: 1745118072},
		ResourceAuditLog:                  {Limit: 1750, Used: 0, Remaining: 1750, Reset: 1745121612},
		ResourceAuditLogStreaming:         {Limit: 15, Used: 0, Remaining: 15, Reset: 1745121612},
		ResourceCodeSearch:                {Limit: 10, Used: 0, Remaining: 10, Reset: 1745118072},
	}, maps.Collect(limits.Iter()))
}

func TestLimits_Parse(t *testing.T) {
	var limits Limits
	err := limits.Parse(&http.Response{
		Header: http.Header{
			"X-Ratelimit-Limit":     []string{"5000"},
			"X-Ratelimit-Used":      []string{"0"},
			"X-Ratelimit-Remaining": []string{"5000"},
			"X-Ratelimit-Reset":     []string{"1745121612"},
			"X-Ratelimit-Resource":  []string{"core"},
		},
	})
	assert.NoError(t, err, "(*Limits).Parse failed")
	assert.Equal(t, &Rate{
		Limit:     5000,
		Used:      0,
		Remaining: 5000,
		Reset:     1745121612,
	}, limits.Load(ResourceCore))

	err = limits.Parse(&http.Response{
		Header: http.Header{
			"X-Ratelimit-Limit":     []string{"invalid"},
			"X-Ratelimit-Used":      []string{"invalid"},
			"X-Ratelimit-Remaining": []string{"invalid"},
			"X-Ratelimit-Reset":     []string{"invalid"},
			"X-Ratelimit-Resource":  []string{"invalid"},
		},
	})
	assert.Error(t, err, "expected error, got nil")
}
