package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimiterIgnoresSpoofedForwardedHeadersUnlessTrusted(t *testing.T) {
	rl := NewRateLimiterWithOptions(1, 2, 1, 100, "")
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Burst 3 requests from same RemoteAddr with spoofed X-Forwarded-For / X-Real-IP headers.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/proposals", nil)
		req.RemoteAddr = "198.51.100.5:41000"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i+1))
		req.Header.Set("X-Real-IP", fmt.Sprintf("192.0.2.%d", i+1))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d", i, rec.Code)
		}
	}

	// 3rd request from same RemoteAddr must still be rate-limited despite spoofed XFF.
	reqLimited := httptest.NewRequest(http.MethodGet, "/api/proposals", nil)
	reqLimited.RemoteAddr = "198.51.100.5:41001"
	reqLimited.Header.Set("X-Forwarded-For", "203.0.113.99")
	recLimited := httptest.NewRecorder()
	handler.ServeHTTP(recLimited, reqLimited)
	if recLimited.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 when spoofed X-Forwarded-For comes from untrusted RemoteAddr, got %d", recLimited.Code)
	}

	// Now trust 10.0.0.0/8 proxy CIDR -> distinct client IPs in XFF get distinct buckets.
	rlTrusted := NewRateLimiterWithOptions(1, 1, 1, 100, "10.0.0.0/8")
	trustedHandler := rlTrusted.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, client := range []string{"203.0.113.10", "203.0.113.11"} {
		req := httptest.NewRequest(http.MethodGet, "/api/proposals", nil)
		req.RemoteAddr = "10.1.2.3:54321"
		req.Header.Set("X-Forwarded-For", client+", 10.1.2.9")
		rec := httptest.NewRecorder()
		trustedHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("trusted proxy client %s expected 200, got %d", client, rec.Code)
		}
	}
}

func TestRateLimiterEnforcesBoundedCardinality(t *testing.T) {
	rl := NewRateLimiterWithOptions(10, 10, 10, 10000, "")
	for i := 0; i < 12000; i++ {
		rl.allow(fmt.Sprintf("global:10.%d.%d.%d", (i>>16)&255, (i>>8)&255, i&255), 10, 10)
	}
	rl.mu.Lock()
	count := len(rl.buckets)
	orderLen := rl.order.Len()
	rl.mu.Unlock()
	if count > 10000 || orderLen > 10000 {
		t.Fatalf("expected buckets capped at 10000, got map=%d list=%d", count, orderLen)
	}
	if count != 10000 {
		t.Fatalf("expected full LRU capacity 10000, got %d", count)
	}
}
