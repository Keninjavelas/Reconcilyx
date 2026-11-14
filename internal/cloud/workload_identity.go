package cloud

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/reconcilyx/operator/pkg/httpretry"
	"github.com/reconcilyx/operator/pkg/security/credentials"
)

const (
	defaultAWSSTSEndpoint   = "https://sts.amazonaws.com/"
	defaultGCPMetadataToken = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"
)

// AWSWebIdentityProvider exchanges an IRSA / OIDC projected service-account token
// via STS AssumeRoleWithWebIdentity for short-lived SigV4 credentials stored in SecretString.
type AWSWebIdentityProvider struct {
	RoleARN     string
	TokenFile   string
	STSEndpoint string
	SessionName string
	HTTPClient  *http.Client

	mu           sync.Mutex
	accessKeyID  credentials.SecretString
	secretKey    credentials.SecretString
	sessionToken credentials.SecretString
	expiresAt    time.Time
}

type stsAssumeRoleResponse struct {
	Result struct {
		Credentials struct {
			AccessKeyID     string `xml:"AccessKeyId"`
			SecretAccessKey string `xml:"SecretAccessKey"`
			SessionToken    string `xml:"SessionToken"`
			Expiration      string `xml:"Expiration"`
		} `xml:"Credentials"`
	} `xml:"AssumeRoleWithWebIdentityResult"`
}

// Resolve returns valid temporary AWS credentials, refreshing via STS before expiry.
func (p *AWSWebIdentityProvider) Resolve(ctx context.Context) (string, string, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.accessKeyID.IsEmpty() && !p.secretKey.IsEmpty() && time.Now().Add(2*time.Minute).Before(p.expiresAt) {
		return p.accessKeyID.Expose(), p.secretKey.Expose(), p.sessionToken.Expose(), nil
	}

	rawJWT, err := os.ReadFile(p.TokenFile)
	if err != nil {
		return "", "", "", fmt.Errorf("read AWS web identity token file: %w", err)
	}
	webToken := strings.TrimSpace(string(rawJWT))
	if webToken == "" {
		return "", "", "", fmt.Errorf("AWS web identity token file %q is empty", p.TokenFile)
	}

	endpoint := strings.TrimSpace(p.STSEndpoint)
	if endpoint == "" {
		endpoint = defaultAWSSTSEndpoint
	}
	session := strings.TrimSpace(p.SessionName)
	if session == "" {
		session = "reconcilyx-pricing"
	}

	form := url.Values{}
	form.Set("Action", "AssumeRoleWithWebIdentity")
	form.Set("Version", "2011-06-15")
	form.Set("RoleArn", p.RoleARN)
	form.Set("RoleSessionName", session)
	form.Set("WebIdentityToken", webToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", "", fmt.Errorf("build AWS STS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := p.HTTPClient
	if client == nil {
		client = newHTTPClientWithCustomCA(10 * time.Second)
	}
	resp, err := httpretry.WrapClient(client, "cloud_workload_identity").Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("call AWS STS AssumeRoleWithWebIdentity: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return "", "", "", fmt.Errorf("read AWS STS response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", "", fmt.Errorf("AWS STS status %d: %s", resp.StatusCode, truncate(string(body), 256))
	}

	var parsed stsAssumeRoleResponse
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return "", "", "", fmt.Errorf("parse AWS STS XML response: %w", err)
	}
	creds := parsed.Result.Credentials
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return "", "", "", fmt.Errorf("AWS STS response missing temporary credentials")
	}

	exp := time.Now().Add(15 * time.Minute)
	if parsedExp, parseErr := time.Parse(time.RFC3339, creds.Expiration); parseErr == nil {
		exp = parsedExp
	}

	p.accessKeyID = credentials.NewSecretString(creds.AccessKeyID)
	p.secretKey = credentials.NewSecretString(creds.SecretAccessKey)
	p.sessionToken = credentials.NewSecretString(creds.SessionToken)
	p.expiresAt = exp

	return p.accessKeyID.Expose(), p.secretKey.Expose(), p.sessionToken.Expose(), nil
}

// GCPMetadataTokenSource fetches short-lived Workload Identity / GCE metadata
// OAuth2 Bearer tokens and caches them wrapped in SecretString.
type GCPMetadataTokenSource struct {
	MetadataURL string
	HTTPClient  *http.Client

	mu        sync.Mutex
	token     credentials.SecretString
	expiresAt time.Time
}

// Token returns a valid OAuth2 access token from the GCP metadata server.
func (g *GCPMetadataTokenSource) Token(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.token.IsEmpty() && time.Now().Add(1*time.Minute).Before(g.expiresAt) {
		return g.token.Expose(), nil
	}

	endpoint := strings.TrimSpace(g.MetadataURL)
	if endpoint == "" {
		endpoint = defaultGCPMetadataToken
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("build GCP metadata request: %w", err)
	}
	req.Header.Set("Metadata-Flavor", "Google")

	client := g.HTTPClient
	if client == nil {
		client = newHTTPClientWithCustomCA(5 * time.Second)
	}
	resp, err := httpretry.WrapClient(client, "cloud_workload_identity").Do(req)
	if err != nil {
		return "", fmt.Errorf("query GCP metadata server: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", fmt.Errorf("read GCP metadata response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GCP metadata status %d: %s", resp.StatusCode, truncate(string(body), 256))
	}

	var doc struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("decode GCP metadata token JSON: %w", err)
	}
	if strings.TrimSpace(doc.AccessToken) == "" {
		return "", fmt.Errorf("GCP metadata response missing access_token")
	}

	ttl := time.Duration(doc.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	g.token = credentials.NewSecretString(doc.AccessToken)
	g.expiresAt = time.Now().Add(ttl)

	return g.token.Expose(), nil
}
