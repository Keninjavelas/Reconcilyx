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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v63/github"
	"github.com/reconcilyx/operator/pkg/httpretry"
	"github.com/reconcilyx/operator/pkg/security/credentials"
	"golang.org/x/oauth2"
)

const defaultGitHubAPIBaseURL = "https://api.github.com"

// GitHubAppTokenSource implements oauth2.TokenSource using GitHub App RS256
// JWT assertions exchanged for short-lived (1h) scoped installation tokens.
type GitHubAppTokenSource struct {
	AppID          int64
	InstallationID int64
	PrivateKey     *rsa.PrivateKey
	APIBaseURL     string
	HTTPClient     *http.Client

	mu        sync.RWMutex
	refreshMu sync.Mutex
	cachedTok credentials.SecretString
	expiresAt time.Time
}

// ParseRSAPrivateKeyPEM parses PKCS#1 or PKCS#8 RSA private keys from PEM bytes.
func ParseRSAPrivateKeyPEM(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("invalid PEM block for GitHub App private key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub App private key: %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("GitHub App private key is not an RSA key")
	}
	return rsaKey, nil
}

// Token returns a valid short-lived GitHub App installation token, reusing cached
// credentials via RWMutex until 5 minutes before expires_at and serializing refreshes.
func (s *GitHubAppTokenSource) Token() (*oauth2.Token, error) {
	s.mu.RLock()
	if !s.cachedTok.IsEmpty() && time.Now().Add(5*time.Minute).Before(s.expiresAt) {
		tok := &oauth2.Token{
			AccessToken: s.cachedTok.Expose(),
			TokenType:   "Bearer",
			Expiry:      s.expiresAt,
		}
		s.mu.RUnlock()
		return tok, nil
	}
	s.mu.RUnlock()

	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	s.mu.RLock()
	if !s.cachedTok.IsEmpty() && time.Now().Add(5*time.Minute).Before(s.expiresAt) {
		tok := &oauth2.Token{
			AccessToken: s.cachedTok.Expose(),
			TokenType:   "Bearer",
			Expiry:      s.expiresAt,
		}
		s.mu.RUnlock()
		return tok, nil
	}
	s.mu.RUnlock()

	if s.AppID <= 0 || s.InstallationID <= 0 || s.PrivateKey == nil {
		return nil, errors.New("GitHub App requires AppID, InstallationID, and RSA PrivateKey")
	}

	appJWT, err := s.mintAppJWT(time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("mint GitHub App JWT: %w", err)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(s.APIBaseURL), "/")
	if baseURL == "" {
		baseURL = defaultGitHubAPIBaseURL
	}
	exchangeURL := fmt.Sprintf("%s/app/installations/%d/access_tokens", baseURL, s.InstallationID)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, exchangeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build GitHub App token exchange request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := httpretry.WrapClient(client, "gitops_github_app").Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange GitHub App installation token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, fmt.Errorf("read GitHub App response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub App installation token API status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var doc struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode GitHub App token response: %w", err)
	}
	if strings.TrimSpace(doc.Token) == "" {
		return nil, errors.New("GitHub App response missing installation token")
	}
	exp := doc.ExpiresAt
	if exp.IsZero() {
		exp = time.Now().Add(55 * time.Minute)
	}

	s.mu.Lock()
	s.cachedTok = credentials.NewSecretString(doc.Token)
	s.expiresAt = exp
	s.mu.Unlock()

	return &oauth2.Token{
		AccessToken: doc.Token,
		TokenType:   "Bearer",
		Expiry:      exp,
	}, nil
}

func (s *GitHubAppTokenSource) mintAppJWT(now time.Time) (string, error) {
	hdr, _ := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	claims, _ := json.Marshal(map[string]any{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": strconv.FormatInt(s.AppID, 10),
	})
	unsigned := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.PrivateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// NewGitHubProviderWithApp creates a GitHubProvider authenticating via ephemeral
// GitHub App installation tokens rather than static personal access tokens.
func NewGitHubProviderWithApp(appID, installationID int64, privateKeyPEM []byte, apiBaseURL, owner, repo string) (*GitHubProvider, error) {
	privKey, err := ParseRSAPrivateKeyPEM(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	ts := &GitHubAppTokenSource{
		AppID:          appID,
		InstallationID: installationID,
		PrivateKey:     privKey,
		APIBaseURL:     apiBaseURL,
	}
	httpClient := oauth2.NewClient(context.Background(), oauth2.ReuseTokenSource(nil, ts))
	httpClient = httpretry.WrapClient(httpClient, "gitops_github")
	ghClient := github.NewClient(httpClient)
	if trimmed := strings.TrimSpace(apiBaseURL); trimmed != "" {
		if u, err := url.Parse(strings.TrimRight(trimmed, "/") + "/"); err == nil {
			ghClient.BaseURL = u
		}
	}
	return &GitHubProvider{client: ghClient, owner: owner, repo: repo}, nil
}
