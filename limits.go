package ghtransport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
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
	Core                      Rate `json:"core"`
	Search                    Rate `json:"search"`
	GraphQL                   Rate `json:"graphql"`
	IntegrationManifest       Rate `json:"integration_manifest"`
	SourceImport              Rate `json:"source_import"`
	CodeScanningUpload        Rate `json:"code_scanning_upload"`
	CodeScanningAutofix       Rate `json:"code_scanning_autofix"`
	ActionsRunnerRegistration Rate `json:"actions_runner_registration"`
	SCIM                      Rate `json:"scim"`
	DependencySnapshots       Rate `json:"dependency_snapshots"`
	AuditLog                  Rate `json:"audit_log"`
	AuditLogStreaming         Rate `json:"audit_log_streaming"`
	CodeSearch                Rate `json:"code_search"`
}

// Rate returns the rate-limit for the given resource type.
func (l *Limits) Rate(resource Resource) *Rate {
	switch resource {
	case ResourceCore:
		return &l.Core
	case ResourceSearch:
		return &l.Search
	case ResourceGraphQL:
		return &l.GraphQL
	case ResourceIntegrationManifest:
		return &l.IntegrationManifest
	case ResourceSourceImport:
		return &l.SourceImport
	case ResourceCodeScanningUpload:
		return &l.CodeScanningUpload
	case ResourceCodeScanningAutofix:
		return &l.CodeScanningAutofix
	case ResourceActionsRunnerRegistration:
		return &l.ActionsRunnerRegistration
	case ResourceSCIM:
		return &l.SCIM
	case ResourceDependencySnapshots:
		return &l.DependencySnapshots
	case ResourceAuditLog:
		return &l.AuditLog
	case ResourceAuditLogStreaming:
		return &l.AuditLogStreaming
	case ResourceCodeSearch:
		return &l.CodeSearch
	default:
		return nil
	}
}

// Iter loops over the resource types and yields each resource type and its rate limit.
func (l *Limits) Iter() iter.Seq2[Resource, *Rate] {
	return func(yield func(Resource, *Rate) bool) {
		if !yield(ResourceCore, &l.Core) {
			return
		}
		if !yield(ResourceSearch, &l.Search) {
			return
		}
		if !yield(ResourceGraphQL, &l.GraphQL) {
			return
		}
		if !yield(ResourceIntegrationManifest, &l.IntegrationManifest) {
			return
		}
		if !yield(ResourceSourceImport, &l.SourceImport) {
			return
		}
		if !yield(ResourceCodeScanningUpload, &l.CodeScanningUpload) {
			return
		}
		if !yield(ResourceCodeScanningAutofix, &l.CodeScanningAutofix) {
			return
		}
		if !yield(ResourceActionsRunnerRegistration, &l.ActionsRunnerRegistration) {
			return
		}
		if !yield(ResourceSCIM, &l.SCIM) {
			return
		}
		if !yield(ResourceDependencySnapshots, &l.DependencySnapshots) {
			return
		}
		if !yield(ResourceAuditLog, &l.AuditLog) {
			return
		}
		if !yield(ResourceAuditLogStreaming, &l.AuditLogStreaming) {
			return
		}
		if !yield(ResourceCodeSearch, &l.CodeSearch) {
			return
		}
	}
}

// Parse updates the rate limits based on the provided HTTP headers.
func (l *Limits) Parse(headers http.Header) error {
	resource := ParseResource(headers)
	if resource == "" {
		return nil // possibly a error or an endpoint without a rate-limit
	}
	rate := l.Rate(resource)
	if rate == nil {
		return fmt.Errorf("unknown resource type: %s", resource)
	}
	return rate.Parse(headers)
}

// Update fetches the latest rate limits from the GitHub API and updates the Limits instance.
// If the provided URL is nil, it defaults to DefaultURL (https://api.github.com/rate_limit).
func (l *Limits) Update(ctx context.Context, transport http.RoundTripper, u *url.URL) error {
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
