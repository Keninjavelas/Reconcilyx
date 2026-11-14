package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestRBACAuthorizeMatrix(t *testing.T) {
	cases := []struct {
		role   Role
		method string
		path   string
		want   bool
	}{
		// Viewer
		{RoleViewer, http.MethodGet, "/api/proposals", true},
		{RoleViewer, http.MethodGet, "/api/metrics/savings", true},
		{RoleViewer, http.MethodPost, "/api/chat", true},
		{RoleViewer, http.MethodPost, "/api/what-if", false},
		{RoleViewer, http.MethodPost, "/api/proposals/p1/approve", false},
		{RoleViewer, http.MethodPost, "/api/v1/federation/compact", false},

		// CI Auditor
		{RoleCIAuditor, http.MethodPost, "/api/what-if", true},
		{RoleCIAuditor, http.MethodPost, "/api/v1/what-if", true},
		{RoleCIAuditor, http.MethodPost, "/api/v1/diff", true},
		{RoleCIAuditor, http.MethodGet, "/api/proposals", true},
		{RoleCIAuditor, http.MethodPost, "/api/proposals/p1/approve", false},
		{RoleCIAuditor, http.MethodPost, "/api/federated/deltas", false},

		// Spoke Agent
		{RoleSpokeAgent, http.MethodPost, "/api/federated/deltas", true},
		{RoleSpokeAgent, http.MethodPost, "/api/v1/federation/deltas", true},
		{RoleSpokeAgent, http.MethodGet, "/api/federated/global-model", true},
		{RoleSpokeAgent, http.MethodGet, "/api/v1/federation/snapshots", true},
		{RoleSpokeAgent, http.MethodGet, "/api/proposals", false},
		{RoleSpokeAgent, http.MethodPost, "/api/v1/federation/compact", false},
		{RoleSpokeAgent, http.MethodPost, "/api/proposals/p1/approve", false},

		// Admin
		{RoleAdmin, http.MethodPost, "/api/proposals/p1/approve", true},
		{RoleAdmin, http.MethodPost, "/api/v1/federation/compact", true},
		{RoleAdmin, http.MethodDelete, "/api/federated/releases/canary", true},
	}

	for _, tc := range cases {
		if got := Authorize(tc.role, tc.method, tc.path); got != tc.want {
			t.Errorf("Authorize(%s, %s, %s) = %v, want %v", tc.role, tc.method, tc.path, got, tc.want)
		}
	}
}

func TestScopedAPIKeysAndExpiry(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	future := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)

	authenticator, err := NewAuthenticator(Config{
		ScopedKeys: []string{
			"rcx_ci_pipeline123@" + future,
			"rcx_spoke_prod-eu_spoke999",
			"rcx_admin_expired@" + past,
		},
		LegacyAPIKey: "legacy-secret-token",
	})
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}
	if !authenticator.HasDeprecatedLegacyKey() {
		t.Fatal("expected deprecated legacy key flag true")
	}

	// CI key
	reqCI := httptest.NewRequest(http.MethodPost, "/api/v1/what-if", nil)
	reqCI.Header.Set("X-API-Key", "rcx_ci_pipeline123")
	pCI, err := authenticator.Authenticate(reqCI)
	if err != nil || pCI.Role != RoleCIAuditor {
		t.Fatalf("expected RoleCIAuditor, got role=%s err=%v", pCI.Role, err)
	}

	// Spoke key binds cluster name
	reqSpoke := httptest.NewRequest(http.MethodPost, "/api/federated/deltas", nil)
	reqSpoke.Header.Set("Authorization", "Bearer rcx_spoke_prod-eu_spoke999")
	pSpoke, err := authenticator.Authenticate(reqSpoke)
	if err != nil || pSpoke.Role != RoleSpokeAgent || pSpoke.ClusterName != "prod-eu" {
		t.Fatalf("expected spoke prod-eu, got %+v err=%v", pSpoke, err)
	}

	// Expired key fails closed
	reqExp := httptest.NewRequest(http.MethodGet, "/api/proposals", nil)
	reqExp.Header.Set("X-API-Key", "rcx_admin_expired")
	if _, err := authenticator.Authenticate(reqExp); err == nil {
		t.Fatal("expected error for expired scoped key")
	}

	// Legacy key compatibility
	reqLegacy := httptest.NewRequest(http.MethodGet, "/api/proposals", nil)
	reqLegacy.Header.Set("X-API-Key", "legacy-secret-token")
	pLegacy, err := authenticator.Authenticate(reqLegacy)
	if err != nil || pLegacy.Role != RoleAdmin || pLegacy.AuthMethod != "legacy-api-key" {
		t.Fatalf("expected legacy admin, got %+v err=%v", pLegacy, err)
	}
}

func TestMTLSClientCertClusterBinding(t *testing.T) {
	caKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "reconcilyx-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	caCert, _ := x509.ParseCertificate(caDER)

	spokeKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	spiffeURL, _ := url.Parse("spiffe://cluster.local/ns/reconcilyx/sa/spoke/cluster/us-east-prod")
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "spoke-us-east-prod"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs:         []*url.URL{spiffeURL},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &spokeKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create spoke leaf: %v", err)
	}
	leafCert, _ := x509.ParseCertificate(leafDER)

	authenticator, err := NewAuthenticator(Config{TLSClientCAPEM: caPEM})
	if err != nil {
		t.Fatalf("NewAuthenticator mTLS: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/deltas", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{leafCert},
	}
	p, err := authenticator.Authenticate(req)
	if err != nil {
		t.Fatalf("mTLS auth failed: %v", err)
	}
	if p.Role != RoleSpokeAgent || p.ClusterName != "us-east-prod" || p.AuthMethod != "mtls" {
		t.Fatalf("unexpected mTLS principal: %+v", p)
	}

	// Untrusted self-signed cert must fail closed.
	untrustedCert, _ := x509.CreateCertificate(rand.Reader, leafTmpl, leafTmpl, &spokeKey.PublicKey, spokeKey)
	badLeaf, _ := x509.ParseCertificate(untrustedCert)
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/federation/deltas", nil)
	badReq.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{badLeaf}}
	if _, err := authenticator.Authenticate(badReq); err == nil {
		t.Fatal("expected untrusted leaf cert to be rejected")
	}
}

func TestOIDCAndGitHubActionsJWKS(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	var jwksSrv *httptest.Server
	jwksSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"jwks_uri": jwksSrv.URL + "/jwks.json",
			})
		case "/jwks.json":
			eBytes := big.NewInt(int64(rsaKey.PublicKey.E)).Bytes()
			_ = json.NewEncoder(w).Encode(jwksDocument{
				Keys: []jwkKey{{
					Kid: "kid-1",
					Kty: "RSA",
					Alg: "RS256",
					N:   base64.RawURLEncoding.EncodeToString(rsaKey.PublicKey.N.Bytes()),
					E:   base64.RawURLEncoding.EncodeToString(eBytes),
				}},
			})
		}
	}))
	defer jwksSrv.Close()

	signJWT := func(claims map[string]any) string {
		hdr, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "kid-1", "typ": "JWT"})
		pl, _ := json.Marshal(claims)
		unsigned := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(pl)
		digest := sha256.Sum256([]byte(unsigned))
		sig, _ := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, digest[:])
		return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)
	}

	authenticator, err := NewAuthenticator(Config{
		OIDC: OIDCConfig{
			IssuerURL:           jwksSrv.URL,
			ClientID:            "reconcilyx-ui",
			JWKSURL:             jwksSrv.URL + "/jwks.json",
			GitHubIssuerURL:     jwksSrv.URL,
			AllowedRepositories: []string{"reconcilyx/infra"},
		},
	})
	if err != nil {
		t.Fatalf("NewAuthenticator OIDC: %v", err)
	}

	// Corporate admin token
	adminJWT := signJWT(map[string]any{
		"iss":    jwksSrv.URL,
		"aud":    "reconcilyx-ui",
		"sub":    "alice@corp.io",
		"groups": []string{"reconcilyx-admins"},
		"exp":    time.Now().Add(10 * time.Minute).Unix(),
	})
	reqAdmin := httptest.NewRequest(http.MethodPost, "/api/proposals/p1/approve", nil)
	reqAdmin.Header.Set("Authorization", "Bearer "+adminJWT)
	pAdmin, err := authenticator.Authenticate(reqAdmin)
	if err != nil || pAdmin.Role != RoleAdmin || pAdmin.Subject != "alice@corp.io" {
		t.Fatalf("expected OIDC admin, got %+v err=%v", pAdmin, err)
	}

	// GitHub Actions OIDC CI token -> RoleCIAuditor
	ghJWT := signJWT(map[string]any{
		"iss":        jwksSrv.URL,
		"aud":        "https://github.com/reconcilyx",
		"sub":        "repo:reconcilyx/infra:ref:refs/heads/main",
		"repository": "reconcilyx/infra",
		"exp":        time.Now().Add(10 * time.Minute).Unix(),
	})
	// Note: when IssuerURL equals GitHubIssuerURL in same mock server, let's test dedicated GitHub-only config below as well.
	reqGH := httptest.NewRequest(http.MethodPost, "/api/v1/what-if", nil)
	reqGH.Header.Set("Authorization", "Bearer "+ghJWT)
	if _, err := authenticator.Authenticate(reqGH); err != nil {
		t.Fatalf("expected OIDC auth success on mock token, got %v", err)
	}

	// Dedicated GitHub Actions OIDC verifier -> repository allowlist enforcement.
	ghAuth, err := NewAuthenticator(Config{
		OIDC: OIDCConfig{
			JWKSURL:             jwksSrv.URL + "/jwks.json",
			GitHubIssuerURL:     jwksSrv.URL,
			AllowedRepositories: []string{"reconcilyx/infra"},
		},
	})
	if err != nil {
		t.Fatalf("NewAuthenticator GitHub OIDC: %v", err)
	}
	pGH, err := ghAuth.Authenticate(reqGH)
	if err != nil || pGH.Role != RoleCIAuditor || pGH.AuthMethod != "github-oidc" {
		t.Fatalf("expected RoleCIAuditor from github-oidc, got %+v err=%v", pGH, err)
	}

	// Disallowed GitHub repo fails with 403.
	badGHJWT := signJWT(map[string]any{
		"iss":        jwksSrv.URL,
		"sub":        "repo:evil/fork:ref:refs/heads/main",
		"repository": "evil/fork",
		"exp":        time.Now().Add(10 * time.Minute).Unix(),
	})
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/what-if", nil)
	badReq.Header.Set("Authorization", "Bearer "+badGHJWT)
	if _, err := ghAuth.Authenticate(badReq); err == nil {
		t.Fatal("expected unapproved GitHub repo to be rejected")
	}
}

type mockK8sReviewer struct {
	fn func(ctx context.Context, token, ns string) (Principal, error)
}

func (m mockK8sReviewer) VerifyToken(ctx context.Context, token, ns string) (Principal, error) {
	return m.fn(ctx, token, ns)
}

func TestK8sTokenReviewStrategy(t *testing.T) {
	authenticator, err := NewAuthenticator(Config{
		K8sReviewer: mockK8sReviewer{
			fn: func(ctx context.Context, token, ns string) (Principal, error) {
				if token != "k8s-sa-jwt" {
					return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "invalid k8s token"}
				}
				return Principal{
					Subject:    "system:serviceaccount:" + ns + ":dev",
					Role:       RoleViewer,
					AuthMethod: "k8s-token",
					Namespaces: []string{ns},
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewAuthenticator k8s: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/proposals", nil)
	req.Header.Set("Authorization", "Bearer k8s-sa-jwt")
	req.Header.Set("X-Reconcilyx-Auth-Mode", "k8s-token")
	req.Header.Set("X-Reconcilyx-Namespace", "payments")
	p, err := authenticator.Authenticate(req)
	if err != nil {
		t.Fatalf("k8s token auth failed: %v", err)
	}
	if p.Role != RoleViewer || len(p.Namespaces) != 1 || p.Namespaces[0] != "payments" {
		t.Fatalf("unexpected k8s principal: %+v", p)
	}
}
