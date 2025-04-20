package ghtransport

import (
	"net/http"
	"strings"
)

// InferResource guessed which rate-limit resource that will be consumed by the provided HTTP request.
func InferResource(req *http.Request) Resource {
	switch {
	case strings.HasPrefix(req.URL.Path, "/search/"):
		if req.URL.Path == "/search/code" {
			return ResourceCodeSearch
		}
		return ResourceSearch
	case req.URL.Path == "/graphql":
		return ResourceGraphQL
	case strings.HasPrefix(req.URL.Path, "/app-manifests/"):
		return ResourceIntegrationManifest
	case strings.HasPrefix(req.URL.Path, "/repos/") &&
		strings.HasSuffix(req.URL.Path, "/code-scanning/sarifs") &&
		req.Method == http.MethodPost:
		return ResourceCodeScanningUpload
	case strings.HasPrefix(req.URL.Path, "/repos/") &&
		strings.Contains(req.URL.Path, "/code-scanning/alerts/") &&
		strings.HasSuffix(req.URL.Path, "/autofix") &&
		req.Method == http.MethodPost:
		return ResourceCodeScanningAutofix
	case strings.HasPrefix(req.URL.Path, "/actions/runners/registration-token") &&
		req.Method == http.MethodPost:
		return ResourceActionsRunnerRegistration
	case strings.HasPrefix(req.URL.Path, "/scim/v2/"):
		return ResourceSCIM
	case strings.HasPrefix(req.URL.Path, "/repos/") &&
		strings.Contains(req.URL.Path, "/dependency-graph/"):
		return ResourceDependencySnapshots
	case (strings.HasPrefix(req.URL.Path, "/enterprises/") ||
		strings.HasPrefix(req.URL.Path, "/organizations/")) && strings.HasSuffix(req.URL.Path, "/audit-log"):
		return ResourceAuditLog
	case (strings.HasPrefix(req.URL.Path, "/enterprises/") ||
		strings.HasPrefix(req.URL.Path, "/organizations/")) && strings.Contains(req.URL.Path, "/audit-log/streams"):
		return ResourceAuditLogStreaming
	}

	// Everything else is assumed to be the core API.
	return ResourceCore
}
