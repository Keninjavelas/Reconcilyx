package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestApplyHubAuthHeadersForwardsKubeconfigTokenWhenNoAPIKey(t *testing.T) {
	origToken := apiToken
	origNS := namespace
	origResolver := kubeconfigTokenResolver
	defer func() {
		apiToken = origToken
		namespace = origNS
		kubeconfigTokenResolver = origResolver
	}()

	apiToken = ""
	namespace = "checkout"
	kubeconfigTokenResolver = func() string { return "k8s-kubeconfig-sa-token" }

	req := httptest.NewRequest(http.MethodGet, "https://hub.example.com/api/proposals", nil)
	applyHubAuthHeaders(req)

	if got := req.Header.Get("Authorization"); got != "Bearer k8s-kubeconfig-sa-token" {
		t.Fatalf("Authorization = %q, want Bearer k8s-kubeconfig-sa-token", got)
	}
	if got := req.Header.Get("X-Reconcilyx-Auth-Mode"); got != "k8s-token" {
		t.Fatalf("X-Reconcilyx-Auth-Mode = %q, want k8s-token", got)
	}
	if got := req.Header.Get("X-Reconcilyx-Namespace"); got != "checkout" {
		t.Fatalf("X-Reconcilyx-Namespace = %q, want checkout", got)
	}

	// Explicit scoped API key takes precedence over kubeconfig fallback.
	apiToken = "rcx_ci_pipeline777"
	reqScoped := httptest.NewRequest(http.MethodPost, "https://hub.example.com/api/v1/what-if", nil)
	applyHubAuthHeaders(reqScoped)
	if got := reqScoped.Header.Get("Authorization"); got != "Bearer rcx_ci_pipeline777" {
		t.Fatalf("scoped key Authorization = %q", got)
	}
	if got := reqScoped.Header.Get("X-Reconcilyx-Auth-Mode"); got != "" {
		t.Fatalf("expected no k8s-token mode header when scoped token is explicit, got %q", got)
	}
}
