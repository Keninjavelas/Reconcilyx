package auth

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Role identifies the authorization scope granted to an authenticated actor.
type Role string

const (
	// RoleViewer grants read-only access to proposals, metrics, forecasts, and audit logs.
	RoleViewer Role = "viewer"
	// RoleCIAuditor grants viewer read access plus shift-left CI analysis (/what-if, /diff).
	RoleCIAuditor Role = "ci_auditor"
	// RoleSpokeAgent grants federated delta upload and global snapshot download scoped to ClusterName.
	RoleSpokeAgent Role = "spoke_agent"
	// RoleAdmin grants full control-plane administration including approvals, rollbacks, policies, and compaction.
	RoleAdmin Role = "admin"
)

// Principal represents a verified inbound caller attached to the request context.
type Principal struct {
	Subject     string    `json:"subject"`
	Role        Role      `json:"role"`
	AuthMethod  string    `json:"authMethod"`
	ClusterName string    `json:"clusterName,omitempty"`
	Namespaces  []string  `json:"namespaces,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt,omitempty"`
}

type contextKey string

const principalContextKey contextKey = "reconcilyx.auth.principal"

// WithPrincipal stores the authenticated Principal on ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, p)
}

// PrincipalFromContext retrieves the authenticated Principal if present.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalContextKey).(Principal)
	return p, ok
}

// Authorize evaluates the fail-closed RBAC matrix for role against method and path.
func Authorize(role Role, method, path string) bool {
	if role == RoleAdmin {
		return true
	}

	normalizedMethod := strings.ToUpper(strings.TrimSpace(method))
	normalizedPath := normalizePath(path)

	// Health endpoints are handled before auth, but allow read-only if queried directly.
	if normalizedMethod == http.MethodGet && (normalizedPath == "/healthz" || normalizedPath == "/api/health" || normalizedPath == "/api/v1/healthz") {
		return true
	}

	switch role {
	case RoleSpokeAgent:
		// Spoke agents may ONLY upload deltas for their own cluster (checked in handler)
		// and fetch the global federated model / snapshot.
		if normalizedMethod == http.MethodPost &&
			(normalizedPath == "/api/federated/deltas" || normalizedPath == "/api/v1/federation/deltas") {
			return true
		}
		if normalizedMethod == http.MethodGet &&
			(normalizedPath == "/api/federated/global-model" || normalizedPath == "/api/v1/federation/snapshots") {
			return true
		}
		return false

	case RoleCIAuditor:
		if normalizedMethod == http.MethodPost &&
			(normalizedPath == "/api/what-if" || normalizedPath == "/api/v1/what-if" ||
				normalizedPath == "/api/diff" || normalizedPath == "/api/v1/diff") {
			return true
		}
		return isViewerReadable(normalizedMethod, normalizedPath)

	case RoleViewer:
		if normalizedMethod == http.MethodPost && normalizedPath == "/api/chat" {
			return true
		}
		return isViewerReadable(normalizedMethod, normalizedPath)

	default:
		// Unknown or unset roles always fail closed.
		return false
	}
}

func isViewerReadable(method, path string) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	if !strings.HasPrefix(path, "/api/") {
		// Non-API UI static assets are readable once authenticated.
		return true
	}
	switch {
	case path == "/api/proposals" || strings.HasPrefix(path, "/api/proposals/") ||
		path == "/api/v1/proposals" || strings.HasPrefix(path, "/api/v1/proposals/"):
		return true
	case path == "/api/policies" || strings.HasPrefix(path, "/api/policies/") ||
		path == "/api/v1/policies" || strings.HasPrefix(path, "/api/v1/policies/"):
		return true
	case path == "/api/metrics/savings" || path == "/api/anomalies" || path == "/api/clusters" || path == "/api/audit":
		return true
	case strings.HasPrefix(path, "/api/forecast/"):
		return true
	case path == "/api/federated/global-model" || path == "/api/federated/releases" ||
		path == "/api/v1/federation/snapshots":
		return true
	default:
		return false
	}
}

func normalizePath(path string) string {
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		path = path[:idx]
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return path
}
