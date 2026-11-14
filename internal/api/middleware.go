package api

import (
	"container/list"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/reconcilyx/operator/pkg/tracing"
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

// LoggingMiddleware records request summary lines enriched with correlation IDs and W3C trace fields.
func LoggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", RequestIDFromContext(r.Context()),
		}
		if tc, ok := tracing.FromContext(r.Context()); ok {
			attrs = append(attrs, "trace_id", tc.TraceID, "span_id", tc.SpanID)
			if tc.ParentSpanID != "" {
				attrs = append(attrs, "parent_span_id", tc.ParentSpanID)
			}
		}
		logger.Info("api request", attrs...)
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
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type,X-API-Key,X-Actor,Authorization,X-Request-ID,traceparent,tracestate")
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

// RequestIDMiddleware injects (or accepts) W3C traceparent and X-Request-ID into context and response headers.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, tc := tracing.StartServerSpan(r.Context(), r.Header)
		w.Header().Set(tracing.HeaderRequestID, tc.RequestID)
		if tp := tc.Traceparent(); tp != "" {
			w.Header().Set(tracing.HeaderTraceparent, tp)
		}
		if ts := r.Header.Get(tracing.HeaderTracestate); ts != "" {
			w.Header().Set(tracing.HeaderTracestate, ts)
		}
		ctx = context.WithValue(ctx, reqIDCtxKey{}, tc.RequestID)
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
	sumA := sha256.Sum256([]byte(a))
	sumB := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(sumA[:], sumB[:]) == 1 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// SecurityHeadersMiddleware applies browser hardening headers on every response.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self' https:; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// BodyLimitMiddleware caps incoming request bodies to prevent memory exhaustion.
func BodyLimitMiddleware(maxBytes int64, next http.Handler) http.Handler {
	if maxBytes <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

const (
	defaultMaxRateLimiterBuckets = 10000
	rateLimiterStaleTTL          = 10 * time.Minute
)

type tokenBucket struct {
	key      string
	tokens   float64
	lastSeen time.Time
	elem     *list.Element
}

// RateLimiter enforces per-IP token bucket limits with stricter quotas on sensitive routes,
// bounded cardinality (LRU + stale eviction), and trusted-proxy CIDR validation.
type RateLimiter struct {
	mu              sync.Mutex
	buckets         map[string]*tokenBucket
	order           *list.List
	maxBuckets      int
	trustedProxies  []*net.IPNet
	rps             float64
	burst           float64
	sensitivePerMin float64
	sensitiveBurst  float64
}

func NewRateLimiter(rps float64, burst int, sensitivePerMin int) *RateLimiter {
	return NewRateLimiterWithOptions(rps, burst, sensitivePerMin, defaultMaxRateLimiterBuckets, os.Getenv("RECONCILYX_TRUSTED_PROXIES"))
}

// NewRateLimiterWithOptions configures explicit bucket capacity and trusted proxy CIDRs.
func NewRateLimiterWithOptions(rps float64, burst int, sensitivePerMin int, maxBuckets int, trustedProxiesCSV string) *RateLimiter {
	if rps <= 0 {
		rps = 30
	}
	if burst <= 0 {
		burst = 60
	}
	if sensitivePerMin <= 0 {
		sensitivePerMin = 60
	}
	if maxBuckets <= 0 {
		maxBuckets = defaultMaxRateLimiterBuckets
	}
	return &RateLimiter{
		buckets:         make(map[string]*tokenBucket),
		order:           list.New(),
		maxBuckets:      maxBuckets,
		trustedProxies:  parseTrustedProxies(trustedProxiesCSV),
		rps:             rps,
		burst:           float64(burst),
		sensitivePerMin: float64(sensitivePerMin),
		sensitiveBurst:  float64(sensitivePerMin),
	}
}

func parseTrustedProxies(csv string) []*net.IPNet {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	var out []*net.IPNet
	for _, raw := range strings.Split(csv, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, cidr, err := net.ParseCIDR(entry); err == nil {
				out = append(out, cidr)
			}
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
		}
	}
	return out
}

func isTrustedProxyIP(ipStr string, nets []*net.IPNet) bool {
	if len(nets) == 0 {
		return false
	}
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func remotePeerIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil && host != "" {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}

func clientIP(r *http.Request, trustedProxies []*net.IPNet) string {
	peer := remotePeerIP(r)
	if !isTrustedProxyIP(peer, trustedProxies) {
		return peer
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		hops := strings.Split(xff, ",")
		// Walk right-to-left and pick the rightmost hop not belonging to a trusted proxy.
		for i := len(hops) - 1; i >= 0; i-- {
			hop := strings.TrimSpace(hops[i])
			if net.ParseIP(hop) == nil {
				continue
			}
			if !isTrustedProxyIP(hop, trustedProxies) {
				return hop
			}
		}
		if first := strings.TrimSpace(hops[0]); net.ParseIP(first) != nil {
			return first
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" && net.ParseIP(xri) != nil {
		return xri
	}
	return peer
}

func isSensitiveRoute(r *http.Request) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodDelete {
		return false
	}
	p := r.URL.Path
	return strings.HasPrefix(p, "/api/chat") ||
		strings.Contains(p, "/approve") ||
		strings.Contains(p, "/reject") ||
		strings.Contains(p, "/rollback") ||
		strings.HasPrefix(p, "/api/v1/federation/compact") ||
		strings.HasPrefix(p, "/api/federated/delta")
}

func (rl *RateLimiter) allow(key string, ratePerSec, capacity float64) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.buckets == nil {
		rl.buckets = make(map[string]*tokenBucket)
	}
	if rl.order == nil {
		rl.order = list.New()
	}
	capLimit := rl.maxBuckets
	if capLimit <= 0 {
		capLimit = defaultMaxRateLimiterBuckets
	}

	b, ok := rl.buckets[key]
	if !ok {
		// Evict stale entries from the LRU tail first before hard-cap LRU eviction.
		cutoff := now.Add(-rateLimiterStaleTTL)
		for elem := rl.order.Back(); elem != nil; {
			prev := elem.Prev()
			tb := elem.Value.(*tokenBucket)
			if !tb.lastSeen.Before(cutoff) && len(rl.buckets) < capLimit {
				break
			}
			if tb.lastSeen.Before(cutoff) {
				rl.order.Remove(elem)
				delete(rl.buckets, tb.key)
			} else {
				break
			}
			elem = prev
		}
		for len(rl.buckets) >= capLimit {
			oldest := rl.order.Back()
			if oldest == nil {
				break
			}
			tb := oldest.Value.(*tokenBucket)
			rl.order.Remove(oldest)
			delete(rl.buckets, tb.key)
		}

		b = &tokenBucket{key: key, tokens: capacity, lastSeen: now}
		b.elem = rl.order.PushFront(b)
		rl.buckets[key] = b
	} else {
		rl.order.MoveToFront(b.elem)
	}

	elapsed := now.Sub(b.lastSeen).Seconds()
	if elapsed > 0 {
		b.tokens = min(capacity, b.tokens+elapsed*ratePerSec)
		b.lastSeen = now
	}
	if b.tokens < 1.0 {
		return false
	}
	b.tokens -= 1.0
	return true
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, rl.trustedProxies)
		allowed := rl.allow("global:"+ip, rl.rps, rl.burst)
		if allowed && isSensitiveRoute(r) {
			allowed = rl.allow("sensitive:"+ip, rl.sensitivePerMin/60.0, rl.sensitiveBurst)
		}
		if !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"Too Many Requests","code":429,"details":"Rate limit exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
