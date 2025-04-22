package ghratelimit

import (
	"net/http"
	"strings"
)

// InferResource guessed which rate-limit resource that will be consumed by the provided HTTP request.
func InferResource(req *http.Request) Resource {
	path := strings.TrimPrefix(req.URL.Path, "/api/v3")
	switch {
	case strings.HasPrefix(path, "/search/"):
		if path == "/search/code" {
			return ResourceCodeSearch
		}
		return ResourceSearch
	case path == "/graphql":
		return ResourceGraphQL
	case strings.HasPrefix(path, "/app-manifests/"):
		return ResourceIntegrationManifest
	case strings.HasPrefix(path, "/repos/") &&
		strings.HasSuffix(path, "/code-scanning/sarifs") &&
		req.Method == http.MethodPost:
		return ResourceCodeScanningUpload
	case strings.HasPrefix(path, "/repos/") &&
		strings.Contains(path, "/code-scanning/alerts/") &&
		strings.HasSuffix(path, "/autofix") &&
		req.Method == http.MethodPost:
		return ResourceCodeScanningAutofix
	case strings.HasPrefix(path, "/actions/runners/registration-token") &&
		req.Method == http.MethodPost:
		return ResourceActionsRunnerRegistration
	case strings.HasPrefix(path, "/scim/v2/"):
		return ResourceSCIM
	case strings.HasPrefix(path, "/repos/") &&
		strings.Contains(path, "/dependency-graph/"):
		return ResourceDependencySnapshots
	case (strings.HasPrefix(path, "/enterprises/") ||
		strings.HasPrefix(path, "/organizations/")) && strings.HasSuffix(path, "/audit-log"):
		return ResourceAuditLog
	case (strings.HasPrefix(path, "/enterprises/") ||
		strings.HasPrefix(path, "/organizations/")) && strings.Contains(path, "/audit-log/streams"):
		return ResourceAuditLogStreaming
	}

	// Everything else is assumed to be the core API.
	return ResourceCore
}
