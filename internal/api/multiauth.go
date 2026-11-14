package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	secauth "github.com/reconcilyx/operator/pkg/security/auth"
)

// MultiAuthMiddleware enforces Zero-Trust multi-identity authentication (K8s TokenReview,
// OIDC/GitHub Actions JWKS JWT, Spoke mTLS SPIFFE, and hashed scoped API keys) plus
// fail-closed route-level RBAC.
func MultiAuthMiddleware(authenticator *secauth.Authenticator, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Public liveness / readiness probes never require authentication.
			if path == "/healthz" || path == "/api/health" || path == "/api/v1/healthz" || path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			// Allow GET/HEAD static SPA shell and assets to load before the browser acquires a token.
			if (r.Method == http.MethodGet || r.Method == http.MethodHead) && !strings.HasPrefix(path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}

			principal, err := authenticator.Authenticate(r)
			if err != nil {
				status := http.StatusUnauthorized
				msg := "Unauthorized: Missing or invalid X-API-Key or Bearer token"
				var ae *secauth.AuthError
				if errors.As(err, &ae) {
					status = ae.StatusCode
					if status == http.StatusUnauthorized {
						msg = "Unauthorized: Missing or invalid X-API-Key (" + ae.Message + ")"
					} else {
						msg = ae.Message
					}
				}
				if logger != nil {
					logger.Warn("Inbound authentication failed",
						"status", status,
						"method", r.Method,
						"path", path,
						"reason", msg,
					)
				}
				writeAuthErrorJSON(w, status, msg)
				return
			}

			if !secauth.Authorize(principal.Role, r.Method, path) {
				if logger != nil {
					logger.Warn("Inbound RBAC authorization denied",
						"subject", principal.Subject,
						"role", principal.Role,
						"auth_method", principal.AuthMethod,
						"method", r.Method,
						"path", path,
					)
				}
				writeAuthErrorJSON(w, http.StatusForbidden, "forbidden: insufficient role permissions for "+r.Method+" "+path)
				return
			}

			w.Header().Set("X-Auth-Mode", principal.AuthMethod)
			w.Header().Set("X-Auth-Role", string(principal.Role))
			ctx := secauth.WithPrincipal(r.Context(), principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ValidateSpokeClusterBinding ensures a RoleSpokeAgent caller can only write deltas
// for its own cryptographically bound ClusterName.
func ValidateSpokeClusterBinding(ctx context.Context, payloadCluster string) (int, string, bool) {
	p, ok := secauth.PrincipalFromContext(ctx)
	if !ok || p.Role != secauth.RoleSpokeAgent || strings.TrimSpace(p.ClusterName) == "" {
		return 0, "", true
	}
	if !strings.EqualFold(strings.TrimSpace(payloadCluster), strings.TrimSpace(p.ClusterName)) {
		return http.StatusForbidden, "forbidden: spoke identity for cluster " + p.ClusterName + " cannot submit deltas for cluster " + payloadCluster, false
	}
	return 0, "", true
}

// ConfigureServerTLS returns a TLSConfig requiring and verifying client certs
// when an mTLS client CA pool is configured on authenticator.
func ConfigureServerTLS(authenticator *secauth.Authenticator) *tls.Config {
	if authenticator == nil || authenticator.ClientCAPool() == nil {
		return nil
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientCAs:  authenticator.ClientCAPool(),
		ClientAuth: tls.VerifyClientCertIfGiven,
	}
}

func writeAuthErrorJSON(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  msg,
		"status": status,
	})
}
