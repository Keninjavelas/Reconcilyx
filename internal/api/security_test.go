package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
)

func securityTestServer(t *testing.T, apiKey, corsOrigins string) *Server {
	t.Helper()
	t.Setenv("RECONCILYX_CORS_ORIGINS", corsOrigins)
	scheme := runtime.NewScheme()
	require.NoError(t, optimizationv1alpha1.AddToScheme(scheme))
	return NewServer(":0", apiKey, ServerDeps{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	})
}

func doRequest(t *testing.T, ts *httptest.Server, method, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, nil)
	require.NoError(t, err)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	return resp
}

func readResponseBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

func TestAPIKeyAuthOverHTTP(t *testing.T) {
	server := securityTestServer(t, "s3cret", "https://console.example.com")
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	t.Run("missing key", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/api/proposals", nil)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		body := readResponseBody(t, resp)
		assert.Contains(t, body, "Missing or invalid X-API-Key")
	})

	t.Run("wrong key", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/api/proposals", map[string]string{"X-API-Key": "wrong"})
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		_ = readResponseBody(t, resp)
	})

	t.Run("header key", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/api/proposals", map[string]string{"X-API-Key": "s3cret"})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body := readResponseBody(t, resp)
		assert.True(t, strings.HasPrefix(body, "{"), "expected JSON body, got %q", body)
	})

	t.Run("cookie key", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/proposals", nil)
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{Name: "reconcilyx_api_key", Value: "s3cret"})
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		_ = readResponseBody(t, resp)
	})

	t.Run("healthz is public", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/healthz", nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, readResponseBody(t, resp), `"status":"ok"`)
	})
}

func TestDevModeAuthHeader(t *testing.T) {
	server := securityTestServer(t, "", "http://localhost:5173")
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp := doRequest(t, ts, http.MethodGet, "/api/proposals", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "dev", resp.Header.Get("X-Auth-Mode"))
	_ = readResponseBody(t, resp)
}

func TestCORSHeadersAndPreflight(t *testing.T) {
	server := securityTestServer(t, "s3cret", "https://console.example.com, https://admin.example.com/")
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	t.Run("allowed origin", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/api/proposals", map[string]string{
			"Origin":    "https://console.example.com",
			"X-API-Key": "s3cret",
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "https://console.example.com", resp.Header.Get("Access-Control-Allow-Origin"))
		assert.Contains(t, resp.Header.Get("Access-Control-Allow-Headers"), "X-API-Key")
		assert.Contains(t, resp.Header.Get("Vary"), "Origin")
		_ = readResponseBody(t, resp)
	})

	t.Run("configured origin with trailing slash still matches", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/api/proposals", map[string]string{
			"Origin":    "https://admin.example.com",
			"X-API-Key": "s3cret",
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "https://admin.example.com", resp.Header.Get("Access-Control-Allow-Origin"))
		_ = readResponseBody(t, resp)
	})

	t.Run("disallowed origin gets no CORS grant", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/api/proposals", map[string]string{
			"Origin":    "https://evil.example.com",
			"X-API-Key": "s3cret",
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
		_ = readResponseBody(t, resp)
	})

	t.Run("preflight is answered without credentials", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodOptions, "/api/proposals", map[string]string{
			"Origin":                         "https://console.example.com",
			"Access-Control-Request-Method":  "POST",
			"Access-Control-Request-Headers": "X-API-Key,Content-Type",
		})
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
		assert.Equal(t, "https://console.example.com", resp.Header.Get("Access-Control-Allow-Origin"))
		assert.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "POST")
		assert.Contains(t, resp.Header.Get("Access-Control-Allow-Headers"), "X-API-Key")
		_ = readResponseBody(t, resp)
	})

	t.Run("preflight from disallowed origin has no CORS grant", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodOptions, "/api/proposals", map[string]string{
			"Origin":                        "https://evil.example.com",
			"Access-Control-Request-Method": "POST",
		})
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
		assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
		_ = readResponseBody(t, resp)
	})
}

func TestParseCORSOrigins(t *testing.T) {
	assert.Equal(t,
		[]string{"https://a.example.com", "https://b.example.com", "*"},
		parseCORSOrigins(" https://a.example.com , https://b.example.com/, * ,"),
	)
	assert.Equal(t,
		[]string{"http://localhost:5173", "http://localhost:8082"},
		parseCORSOrigins(""),
	)
}
