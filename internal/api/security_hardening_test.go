package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	srv := securityTestServer(t, "s3cret", "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp := doRequest(t, ts, http.MethodGet, "/api/v1/healthz", nil)
	_ = readResponseBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
	assert.Contains(t, resp.Header.Get("Content-Security-Policy"), "default-src 'self'")
	assert.Equal(t, "", resp.Header.Get("Strict-Transport-Security"), "HSTS must not be emitted on plain HTTP")

	// Simulate TLS request to verify HSTS emission
	req := httptest.NewRequest(http.MethodGet, "https://hub.example.test/healthz", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assert.Contains(t, rec.Header().Get("Strict-Transport-Security"), "max-age=31536000")
}

func TestRateLimiterRejectsBurstWith429(t *testing.T) {
	rl := NewRateLimiter(1, 2, 1)
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/proposals", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/proposals", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "60", rec.Header().Get("Retry-After"))
}

func TestBodyLimitMiddlewareCapsPayloads(t *testing.T) {
	srv := securityTestServer(t, "s3cret", "")
	oversize := `{"message":"` + strings.Repeat("a", (1<<20)+128) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(oversize))
	req.Header.Set("X-API-Key", "s3cret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusOK, rec.Code)
}
