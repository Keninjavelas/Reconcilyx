package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// RecoverMiddleware converts handler panics into 500 JSON responses.
func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("API handler panicked", "panic", rec, "method", r.Method, "path", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"Internal Server Error","code":500}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware records request summary lines.
func LoggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		logger.Info("api request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", RequestIDFromContext(r.Context()),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// CORSMiddleware allows specified origins.
func CORSMiddleware(origins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := slices.Contains(origins, "*")
		if !allowed {
			for _, o := range origins {
				if strings.EqualFold(o, origin) {
					allowed = true
					break
				}
			}
		}
		if origin != "" && allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type,X-API-Key,X-Actor,Authorization,X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type reqIDCtxKey struct{}

func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(reqIDCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// RequestIDMiddleware injects (or accepts) a request id into context.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			buf := make([]byte, 12)
			_, _ = sha256Pool(buf, r.URL.Path+strconv.FormatInt(time.Now().UnixNano(), 10))
			id = hex.EncodeToString(buf)
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), reqIDCtxKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func sha256Pool(out []byte, in string) (int, error) {
	s := sha256.Sum256([]byte(in))
	return copy(out, s[:]), nil
}

var _ = hmac.Equal

// APIKeyAuthMiddleware gates non-public routes on X-API-Key when a key is configured.
func APIKeyAuthMiddleware(apiKey string, publicRoutes map[string]struct{}, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if _, ok := publicRoutes[path]; ok {
			next.ServeHTTP(w, r)
			return
		}
		// Assets served under /assets/ and /favicon.* are also public
		if strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/favicon") {
			next.ServeHTTP(w, r)
			return
		}
		// Client-side routes serve the public SPA shell; API data routes stay gated.
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && isSPARoute(path) {
			next.ServeHTTP(w, r)
			return
		}
		if apiKey == "" {
			// dev mode: unauthenticated, inject default actor and proceed
			ctx := context.WithValue(r.Context(), actorContextKey, "dev-mode-unauthenticated")
			w.Header().Set("X-Auth-Mode", "dev")
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		given := r.Header.Get("X-API-Key")
		if given == "" {
			if cookie, err := r.Cookie("reconcilyx_api_key"); err == nil && cookie.Value != "" {
				given = cookie.Value
			}
		}
		if !constantTimeEqual(given, apiKey) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Unauthorized","code":401,"details":"Missing or invalid X-API-Key header"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isSPARoute(path string) bool {
	for _, prefix := range []string{"/proposals", "/forecast", "/savings", "/policies", "/settings"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return hmac.Equal([]byte(a), []byte(b))
}
