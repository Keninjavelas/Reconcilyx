package cmd

import (
	"net/http"
	"os"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
)

const inClusterTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

var kubeconfigTokenResolver = defaultKubeconfigTokenResolver

func defaultKubeconfigTokenResolver() string {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	if restCfg, err := cfg.ClientConfig(); err == nil && restCfg != nil {
		if tok := strings.TrimSpace(restCfg.BearerToken); tok != "" {
			return tok
		}
		if restCfg.BearerTokenFile != "" {
			if raw, readErr := os.ReadFile(restCfg.BearerTokenFile); readErr == nil {
				if tok := strings.TrimSpace(string(raw)); tok != "" {
					return tok
				}
			}
		}
	}
	if raw, err := os.ReadFile(inClusterTokenPath); err == nil {
		if tok := strings.TrimSpace(string(raw)); tok != "" {
			return tok
		}
	}
	return ""
}

// applyHubAuthHeaders attaches scoped API keys/JWTs from --token/RECONCILYX_API_KEY
// or falls back to forwarding the active kubeconfig Bearer token via TokenReview.
func applyHubAuthHeaders(req *http.Request) {
	if req == nil {
		return
	}
	tok := strings.TrimSpace(apiToken)
	if tok != "" {
		if strings.HasPrefix(tok, "rcx_") || strings.Count(tok, ".") == 2 {
			req.Header.Set("Authorization", "Bearer "+tok)
		} else {
			req.Header.Set("X-API-Key", tok)
		}
		return
	}
	if k8sTok := strings.TrimSpace(kubeconfigTokenResolver()); k8sTok != "" {
		req.Header.Set("Authorization", "Bearer "+k8sTok)
		req.Header.Set("X-Reconcilyx-Auth-Mode", "k8s-token")
		if strings.TrimSpace(namespace) != "" {
			req.Header.Set("X-Reconcilyx-Namespace", strings.TrimSpace(namespace))
		}
	}
}
