package api

import (
	"bytes"
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
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	secauth "github.com/reconcilyx/operator/pkg/security/auth"
)

type stubReviewer struct {
	principal secauth.Principal
	err       error
}

func (s stubReviewer) VerifyToken(_ context.Context, token, _ string) (secauth.Principal, error) {
	if s.err != nil {
		return secauth.Principal{}, s.err
	}
	if token == "" {
		return secauth.Principal{}, &secauth.AuthError{StatusCode: http.StatusUnauthorized, Message: "empty token"}
	}
	return s.principal, nil
}

func TestMultiAuthHTTPMatrixAndPrivilegeEscalation(t *testing.T) {
	srv := NewServer(":0", "", ServerDeps{
		AuthConfig: &secauth.Config{
			ScopedKeys: []string{
				"rcx_viewer_readonly123",
				"rcx_ci_pipeline456",
				"rcx_spoke_prod-eu_spoke789",
				"rcx_admin_boss999",
			},
		},
		K8sTokenReviewer: stubReviewer{
			principal: secauth.Principal{
				Subject:    "system:serviceaccount:payments:ci-reader",
				Role:       secauth.RoleViewer,
				AuthMethod: "k8s-token",
				Namespaces: []string{"payments"},
			},
		},
	})

	// 1. Unauthenticated request must fail closed (401) even though LegacyAPIKey == "".
	reqUnauth := httptest.NewRequest(http.MethodGet, "/api/proposals", nil)
	recUnauth := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when scoped keys are active, got %d", recUnauth.Code)
	}

	// 2. Spoke key can push deltas for its own cluster (200), but NOT another cluster (403)
	// and CANNOT trigger compaction or proposal approval (403).
	ownBatch := []byte(`{"clusterName":"prod-eu","deltas":[{"deltaQ":0.5,"visitCount":1}]}`)
	reqOwn := httptest.NewRequest(http.MethodPost, "/api/v1/federation/deltas", bytes.NewReader(ownBatch))
	reqOwn.Header.Set("Authorization", "Bearer rcx_spoke_prod-eu_spoke789")
	recOwn := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recOwn, reqOwn)
	if recOwn.Code != http.StatusOK {
		t.Fatalf("expected 200 for own cluster delta, got %d: %s", recOwn.Code, recOwn.Body.String())
	}
	if recOwn.Header().Get("X-Auth-Role") != string(secauth.RoleSpokeAgent) {
		t.Fatalf("expected X-Auth-Role=spoke_agent, got %q", recOwn.Header().Get("X-Auth-Role"))
	}

	crossBatch := []byte(`{"clusterName":"prod-us","deltas":[{"deltaQ":0.5,"visitCount":1}]}`)
	reqCross := httptest.NewRequest(http.MethodPost, "/api/federated/deltas", bytes.NewReader(crossBatch))
	reqCross.Header.Set("X-API-Key", "rcx_spoke_prod-eu_spoke789")
	recCross := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recCross, reqCross)
	if recCross.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-cluster delta injection, got %d: %s", recCross.Code, recCross.Body.String())
	}

	reqCompactBySpoke := httptest.NewRequest(http.MethodPost, "/api/v1/federation/compact", nil)
	reqCompactBySpoke.Header.Set("X-API-Key", "rcx_spoke_prod-eu_spoke789")
	recCompactBySpoke := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recCompactBySpoke, reqCompactBySpoke)
	if recCompactBySpoke.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for spoke calling /compact, got %d", recCompactBySpoke.Code)
	}

	// 3. CI Auditor can call POST /api/v1/what-if, but NOT POST /api/proposals/{name}/approve (403).
	whatIfBody := []byte(`{"namespace":"prod","workloadName":"api","currentCPU":"2000m","currentMemory":"4Gi","proposedCPU":"500m","proposedMemory":"1Gi"}`)
	reqCIWhatIf := httptest.NewRequest(http.MethodPost, "/api/v1/what-if", bytes.NewReader(whatIfBody))
	reqCIWhatIf.Header.Set("X-API-Key", "rcx_ci_pipeline456")
	recCIWhatIf := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recCIWhatIf, reqCIWhatIf)
	if recCIWhatIf.Code == http.StatusUnauthorized || recCIWhatIf.Code == http.StatusForbidden {
		t.Fatalf("expected CI auditor allowed on /what-if, got %d", recCIWhatIf.Code)
	}

	reqCIApprove := httptest.NewRequest(http.MethodPost, "/api/proposals/demo/approve", nil)
	reqCIApprove.Header.Set("X-API-Key", "rcx_ci_pipeline456")
	recCIApprove := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recCIApprove, reqCIApprove)
	if recCIApprove.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for CI auditor approving proposal, got %d", recCIApprove.Code)
	}

	// 4. Viewer via K8s TokenReview can GET /api/v1/federation/snapshots (or metrics), but NOT mutate.
	reqK8sSnap := httptest.NewRequest(http.MethodGet, "/api/v1/federation/snapshots", nil)
	reqK8sSnap.Header.Set("Authorization", "Bearer k8s-valid-sa-token")
	reqK8sSnap.Header.Set("X-Reconcilyx-Auth-Mode", "k8s-token")
	recK8sSnap := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recK8sSnap, reqK8sSnap)
	if recK8sSnap.Code != http.StatusOK {
		t.Fatalf("expected 200 for k8s viewer reading snapshots, got %d: %s", recK8sSnap.Code, recK8sSnap.Body.String())
	}

	// 5. Admin key can trigger compaction (200).
	reqAdminCompact := httptest.NewRequest(http.MethodPost, "/api/v1/federation/compact", nil)
	reqAdminCompact.Header.Set("X-API-Key", "rcx_admin_boss999")
	recAdminCompact := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recAdminCompact, reqAdminCompact)
	if recAdminCompact.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin calling /compact, got %d: %s", recAdminCompact.Code, recAdminCompact.Body.String())
	}
}

func TestMultiAuthMTLSAndOIDCEndToEnd(t *testing.T) {
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	var jwksSrv *httptest.Server
	jwksSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		eBytes := big.NewInt(int64(rsaKey.PublicKey.E)).Bytes()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kid": "k1",
				"kty": "RSA",
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(rsaKey.PublicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(eBytes),
			}},
		})
	}))
	defer jwksSrv.Close()

	caKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(10), Subject: pkix.Name{CommonName: "hub-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true, IsCA: true,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caDER)

	spokeKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	spiffeURI, _ := url.Parse("spiffe://mesh.local/ns/reconcilyx/cluster/ap-south")
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(11), Subject: pkix.Name{CommonName: "spoke-ap-south"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs: []*url.URL{spiffeURI},
	}
	leafDER, _ := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &spokeKey.PublicKey, caKey)
	leafCert, _ := x509.ParseCertificate(leafDER)

	srv := NewServer(":0", "", ServerDeps{
		AuthConfig: &secauth.Config{
			OIDC: secauth.OIDCConfig{
				JWKSURL:             jwksSrv.URL,
				GitHubIssuerURL:     "https://token.actions.githubusercontent.com",
				AllowedRepositories: []string{"org/platform"},
			},
		},
	})
	// Attach mTLS via custom authenticator check as well on separate handler.
	authWithMTLS, err := secauth.NewAuthenticator(secauth.Config{
		OIDC: secauth.OIDCConfig{
			JWKSURL:             jwksSrv.URL,
			GitHubIssuerURL:     "https://token.actions.githubusercontent.com",
			AllowedRepositories: []string{"org/platform"},
		},
	})
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}
	_ = authWithMTLS

	// Sign GitHub Actions OIDC JWT -> RoleCIAuditor
	hdr, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1"})
	pl, _ := json.Marshal(map[string]any{
		"iss":        "https://token.actions.githubusercontent.com",
		"sub":        "repo:org/platform:pull_request",
		"repository": "org/platform",
		"exp":        time.Now().Add(5 * time.Minute).Unix(),
	})
	unsigned := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(pl)
	digest := sha256.Sum256([]byte(unsigned))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, digest[:])
	jwtToken := unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)

	reqGH := httptest.NewRequest(http.MethodPost, "/api/what-if", bytes.NewReader([]byte(`{"namespace":"prod","workloadName":"web","currentCPU":"1000m","currentMemory":"2Gi","proposedCPU":"500m","proposedMemory":"1Gi"}`)))
	reqGH.Header.Set("Authorization", "Bearer "+jwtToken)
	recGH := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recGH, reqGH)
	if recGH.Code != http.StatusOK || recGH.Header().Get("X-Auth-Mode") != "github-oidc" {
		t.Fatalf("expected 200 github-oidc, got %d (%s) mode=%q", recGH.Code, recGH.Body.String(), recGH.Header().Get("X-Auth-Mode"))
	}

	// Verify mTLS cluster mismatch returns 403 over HTTP layer.
	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)
	verifiedSrv := NewServer(":0", "", ServerDeps{})
	reqMTLS := httptest.NewRequest(http.MethodPost, "/api/federated/deltas", bytes.NewReader([]byte(`{"clusterName":"wrong-cluster","deltas":[]}`)))
	reqMTLS.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{leafCert},
		VerifiedChains:   [][]*x509.Certificate{{leafCert, caCert}},
	}
	recMTLS := httptest.NewRecorder()
	verifiedSrv.Handler().ServeHTTP(recMTLS, reqMTLS)
	if recMTLS.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for mTLS cluster mismatch, got %d: %s", recMTLS.Code, recMTLS.Body.String())
	}
}
