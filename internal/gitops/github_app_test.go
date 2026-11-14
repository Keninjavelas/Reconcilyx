package gitops

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGitHubAppJWTAndInstallationTokenExchange(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	})

	var verifiedAppJWT, prAuthHeader string
	ghMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/installations/98765/access_tokens":
			authz := r.Header.Get("Authorization")
			jwtStr := strings.TrimPrefix(authz, "Bearer ")
			parts := strings.Split(jwtStr, ".")
			if len(parts) != 3 {
				t.Fatalf("expected 3-part app JWT, got %q", jwtStr)
			}
			// Verify RS256 signature with public key.
			sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
			if err != nil {
				t.Fatalf("decode signature: %v", err)
			}
			digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
			if err := rsa.VerifyPKCS1v15(&privKey.PublicKey, crypto.SHA256, digest[:], sigBytes); err != nil {
				t.Fatalf("invalid RS256 GitHub App JWT signature: %v", err)
			}
			claimBytes, _ := base64.RawURLEncoding.DecodeString(parts[1])
			var claims map[string]any
			_ = json.Unmarshal(claimBytes, &claims)
			if claims["iss"] != "12345" {
				t.Fatalf("expected iss=12345, got %v", claims["iss"])
			}
			verifiedAppJWT = jwtStr
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":      "ghs_ephemeral_install_tok",
				"expires_at": time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
			})

		case "/repos/org/repo/git/ref/heads/main":
			prAuthHeader = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ref": "refs/heads/main",
				"object": map[string]any{
					"sha": "deadbeef123",
				},
			})

		case "/repos/org/repo/git/refs":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"ref": "refs/heads/reconcilyx/opt-1"})

		case "/repos/org/repo/contents/manifests/app.yaml":
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"content": map[string]any{"name": "app.yaml"}})

		case "/repos/org/repo/pulls":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 42, "html_url": "https://github.com/org/repo/pull/42"})

		default:
			t.Fatalf("unexpected mock GitHub path: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ghMock.Close()

	secDir := t.TempDir()
	ghDir := filepath.Join(secDir, "github")
	_ = os.MkdirAll(ghDir, 0o755)
	_ = os.WriteFile(filepath.Join(ghDir, "app_private_key.pem"), privPEM, 0o600)

	t.Setenv("RECONCILYX_SECRETS_DIR", secDir)
	t.Setenv("RECONCILYX_GITOPS_GITHUB_APP_ID", "12345")
	t.Setenv("RECONCILYX_GITOPS_GITHUB_INSTALLATION_ID", "98765")
	t.Setenv("RECONCILYX_GITOPS_GITHUB_API_URL", ghMock.URL)

	prov, err := NewGitHubProviderFromEnv("org", "repo")
	if err != nil {
		t.Fatalf("NewGitHubProviderFromEnv failed: %v", err)
	}

	prURL, err := prov.CreatePullRequest(context.Background(), &PullRequest{
		Title:      "chore: rightsize app",
		FilePath:   "manifests/app.yaml",
		BaseBranch: "main",
		HeadBranch: "reconcilyx/opt-1",
		Diff:       "resources:\n  requests:\n    cpu: 250m\n",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest via GitHub App failed: %v", err)
	}
	if prURL != "42" {
		t.Fatalf("unexpected PR number/id: %s", prURL)
	}
	if verifiedAppJWT == "" {
		t.Fatal("expected app JWT exchange")
	}
	if prAuthHeader != "Bearer ghs_ephemeral_install_tok" {
		t.Fatalf("expected ephemeral installation token on GitHub API call, got %q", prAuthHeader)
	}
}

func TestGitHubAppConcurrentTokenReuse(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	var exchangeCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchangeCalls.Add(1)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":      "ghs_cached_concurrent_tok",
			"expires_at": time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	ts := &GitHubAppTokenSource{
		AppID:          111,
		InstallationID: 222,
		PrivateKey:     privKey,
		APIBaseURL:     srv.URL,
		HTTPClient:     srv.Client(),
	}

	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, tokErr := ts.Token()
			if tokErr != nil {
				t.Errorf("Token() error: %v", tokErr)
				return
			}
			if tok.AccessToken != "ghs_cached_concurrent_tok" {
				t.Errorf("unexpected token: %q", tok.AccessToken)
			}
		}()
	}
	wg.Wait()

	if calls := exchangeCalls.Load(); calls != 1 {
		t.Fatalf("expected exactly 1 installation token exchange across 25 concurrent callers, got %d", calls)
	}
}
