package auth

import (
	"container/list"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// AuthError distinguishes unauthenticated (401) from forbidden (403) failures.
type AuthError struct {
	StatusCode int
	Message    string
}

func (e *AuthError) Error() string {
	return e.Message
}

// K8sTokenReviewer verifies Kubernetes Bearer tokens via TokenReview and checks
// permissions via SubjectAccessReview.
type K8sTokenReviewer interface {
	VerifyToken(ctx context.Context, bearerToken, namespace string) (Principal, error)
}

const (
	defaultK8sTokenCacheSize   = 1024
	defaultK8sPositiveTTL      = 45 * time.Second
	defaultK8sNegativeTTL      = 5 * time.Second
	defaultK8sReviewTimeout    = 3 * time.Second
	defaultJWKSRefreshCooldown = 60 * time.Second
)

type tokenCacheItem struct {
	key       string
	principal Principal
	authErr   *AuthError
	expiresAt time.Time
}

// KubernetesTokenReviewer implements K8sTokenReviewer using the live cluster API
// with a bounded LRU + TTL cache so burst requests never starve API QPS.
type KubernetesTokenReviewer struct {
	Client         kubernetes.Interface
	MaxCacheSize   int
	PositiveTTL    time.Duration
	NegativeTTL    time.Duration
	ReviewTimeout  time.Duration
	Now            func() time.Time
	cacheMu        sync.Mutex
	cacheItems     map[string]*list.Element
	cacheOrder     *list.List
}

func (k *KubernetesTokenReviewer) now() time.Time {
	if k.Now != nil {
		return k.Now()
	}
	return time.Now()
}

func (k *KubernetesTokenReviewer) positiveTTL() time.Duration {
	if k.PositiveTTL > 0 {
		return k.PositiveTTL
	}
	return defaultK8sPositiveTTL
}

func (k *KubernetesTokenReviewer) negativeTTL() time.Duration {
	if k.NegativeTTL > 0 {
		return k.NegativeTTL
	}
	return defaultK8sNegativeTTL
}

func (k *KubernetesTokenReviewer) reviewTimeout() time.Duration {
	if k.ReviewTimeout > 0 {
		return k.ReviewTimeout
	}
	return defaultK8sReviewTimeout
}

func (k *KubernetesTokenReviewer) maxCacheSize() int {
	if k.MaxCacheSize > 0 {
		return k.MaxCacheSize
	}
	return defaultK8sTokenCacheSize
}

func tokenCacheKey(bearerToken, namespace string) string {
	sum := sha256.Sum256([]byte(bearerToken))
	return hex.EncodeToString(sum[:]) + "\x00" + namespace
}

func (k *KubernetesTokenReviewer) lookupCache(key string, now time.Time) (Principal, *AuthError, bool) {
	k.cacheMu.Lock()
	defer k.cacheMu.Unlock()
	if k.cacheItems == nil {
		return Principal{}, nil, false
	}
	elem, ok := k.cacheItems[key]
	if !ok {
		return Principal{}, nil, false
	}
	entry := elem.Value.(*tokenCacheItem)
	if now.After(entry.expiresAt) {
		k.cacheOrder.Remove(elem)
		delete(k.cacheItems, key)
		return Principal{}, nil, false
	}
	k.cacheOrder.MoveToFront(elem)
	return entry.principal, entry.authErr, true
}

func (k *KubernetesTokenReviewer) storeCache(key string, p Principal, ae *AuthError, ttl time.Duration, now time.Time) {
	maxCap := k.maxCacheSize()
	k.cacheMu.Lock()
	defer k.cacheMu.Unlock()
	if k.cacheItems == nil {
		k.cacheItems = make(map[string]*list.Element, maxCap)
		k.cacheOrder = list.New()
	}
	if elem, exists := k.cacheItems[key]; exists {
		item := elem.Value.(*tokenCacheItem)
		item.principal = p
		item.authErr = ae
		item.expiresAt = now.Add(ttl)
		k.cacheOrder.MoveToFront(elem)
		return
	}
	for k.cacheOrder.Len() >= maxCap {
		oldest := k.cacheOrder.Back()
		if oldest == nil {
			break
		}
		k.cacheOrder.Remove(oldest)
		delete(k.cacheItems, oldest.Value.(*tokenCacheItem).key)
	}
	item := &tokenCacheItem{
		key:       key,
		principal: p,
		authErr:   ae,
		expiresAt: now.Add(ttl),
	}
	k.cacheItems[key] = k.cacheOrder.PushFront(item)
}

// VerifyToken runs cached TokenReview + SubjectAccessReview against the Kubernetes API server
// with a strict 3-second RPC timeout and bounded LRU/TTL verdict cache.
func (k *KubernetesTokenReviewer) VerifyToken(ctx context.Context, bearerToken, namespace string) (Principal, error) {
	if k == nil || k.Client == nil {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "kubernetes token reviewer unavailable"}
	}
	if strings.TrimSpace(bearerToken) == "" {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "missing kubernetes bearer token"}
	}

	now := k.now()
	cacheKey := tokenCacheKey(bearerToken, namespace)
	if p, ae, hit := k.lookupCache(cacheKey, now); hit {
		if ae != nil {
			errCopy := *ae
			return Principal{}, &errCopy
		}
		return p, nil
	}

	reviewCtx, cancel := context.WithTimeout(ctx, k.reviewTimeout())
	defer cancel()

	p, ae := k.verifyTokenLive(reviewCtx, bearerToken, namespace)
	if ae != nil {
		errCopy := *ae
		k.storeCache(cacheKey, Principal{}, &errCopy, k.negativeTTL(), k.now())
		return Principal{}, ae
	}
	k.storeCache(cacheKey, p, nil, k.positiveTTL(), k.now())
	return p, nil
}

func (k *KubernetesTokenReviewer) verifyTokenLive(ctx context.Context, bearerToken, namespace string) (Principal, *AuthError) {
	tr, err := k.Client.AuthenticationV1().TokenReviews().Create(ctx, &authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{Token: bearerToken},
	}, metav1.CreateOptions{})
	if err != nil {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: fmt.Sprintf("token review failed: %v", err)}
	}
	if !tr.Status.Authenticated {
		msg := tr.Status.Error
		if msg == "" {
			msg = "kubernetes token not authenticated"
		}
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: msg}
	}

	userInfo := tr.Status.User
	isAdmin, err := k.checkSAR(ctx, userInfo, namespace, "update", "pods", "resize")
	if err == nil && isAdmin {
		return Principal{
			Subject:    userInfo.Username,
			Role:       RoleAdmin,
			AuthMethod: "k8s-token",
			Namespaces: nonEmptySlice(namespace),
		}, nil
	}

	isViewer, err := k.checkSAR(ctx, userInfo, namespace, "get", "pods", "")
	if err != nil || !isViewer {
		return Principal{}, &AuthError{StatusCode: http.StatusForbidden, Message: "kubernetes subject lacks permission to inspect pods"}
	}

	return Principal{
		Subject:    userInfo.Username,
		Role:       RoleViewer,
		AuthMethod: "k8s-token",
		Namespaces: nonEmptySlice(namespace),
	}, nil
}

func (k *KubernetesTokenReviewer) checkSAR(ctx context.Context, user authv1.UserInfo, namespace, verb, resource, subresource string) (bool, error) {
	extra := make(map[string]authorizationv1.ExtraValue, len(user.Extra))
	for key, vals := range user.Extra {
		extra[key] = authorizationv1.ExtraValue(vals)
	}
	sar, err := k.Client.AuthorizationV1().SubjectAccessReviews().Create(ctx, &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   user.Username,
			Groups: user.Groups,
			UID:    user.UID,
			Extra:  extra,
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace:   namespace,
				Verb:        verb,
				Group:       "",
				Resource:    resource,
				Subresource: subresource,
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	return sar.Status.Allowed, nil
}

// ScopedKeyRecord stores a SHA-256 hashed scoped API key and optional expiration.
type ScopedKeyRecord struct {
	ID          string
	SHA256Hex   string
	Role        Role
	ClusterName string
	ExpiresAt   time.Time
}

// ParseScopedKeySpec parses a raw or hashed key definition into a ScopedKeyRecord.
// Supported raw prefixes:
//   - rcx_admin_<rand>
//   - rcx_ci_<rand>
//   - rcx_viewer_<rand>
//   - rcx_spoke_<cluster>_<rand>
func ParseScopedKeySpec(raw string) (ScopedKeyRecord, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ScopedKeyRecord{}, false
	}

	var expiresAt time.Time
	keyPart := raw
	if atIdx := strings.LastIndex(raw, "@"); atIdx > 0 {
		candidateTS := raw[atIdx+1:]
		if parsed, err := time.Parse(time.RFC3339, candidateTS); err == nil {
			keyPart = raw[:atIdx]
			expiresAt = parsed
		}
	}

	sum := sha256.Sum256([]byte(keyPart))
	digest := hex.EncodeToString(sum[:])

	switch {
	case strings.HasPrefix(keyPart, "rcx_admin_"):
		return ScopedKeyRecord{ID: prefixID(keyPart), SHA256Hex: digest, Role: RoleAdmin, ExpiresAt: expiresAt}, true
	case strings.HasPrefix(keyPart, "rcx_ci_"):
		return ScopedKeyRecord{ID: prefixID(keyPart), SHA256Hex: digest, Role: RoleCIAuditor, ExpiresAt: expiresAt}, true
	case strings.HasPrefix(keyPart, "rcx_viewer_"):
		return ScopedKeyRecord{ID: prefixID(keyPart), SHA256Hex: digest, Role: RoleViewer, ExpiresAt: expiresAt}, true
	case strings.HasPrefix(keyPart, "rcx_spoke_"):
		rest := strings.TrimPrefix(keyPart, "rcx_spoke_")
		sep := strings.LastIndex(rest, "_")
		if sep <= 0 || sep == len(rest)-1 {
			return ScopedKeyRecord{}, false
		}
		cluster := rest[:sep]
		return ScopedKeyRecord{
			ID:          prefixID(keyPart),
			SHA256Hex:   digest,
			Role:        RoleSpokeAgent,
			ClusterName: cluster,
			ExpiresAt:   expiresAt,
		}, true
	default:
		return ScopedKeyRecord{}, false
	}
}

func prefixID(raw string) string {
	if len(raw) <= 16 {
		return raw
	}
	return raw[:16] + "..."
}

// ScopedKeyStore verifies hashed scoped keys and optional legacy static key.
type ScopedKeyStore struct {
	records        []ScopedKeyRecord
	legacyKeyHash  string
	now            func() time.Time
	hasLegacyKey   bool
}

// NewScopedKeyStore builds a store from comma-separated scoped keys plus optional legacyKey.
func NewScopedKeyStore(scopedKeys []string, legacyKey string) *ScopedKeyStore {
	s := &ScopedKeyStore{
		now: time.Now,
	}
	for _, spec := range scopedKeys {
		if rec, ok := ParseScopedKeySpec(spec); ok {
			s.records = append(s.records, rec)
		}
	}
	if trimmed := strings.TrimSpace(legacyKey); trimmed != "" {
		if rec, ok := ParseScopedKeySpec(trimmed); ok {
			s.records = append(s.records, rec)
		} else {
			sum := sha256.Sum256([]byte(trimmed))
			s.legacyKeyHash = hex.EncodeToString(sum[:])
			s.hasLegacyKey = true
		}
	}
	return s
}

// Verify compares candidate against configured hashed scoped keys and legacy key.
func (s *ScopedKeyStore) Verify(candidate string) (Principal, bool, error) {
	if s == nil {
		return Principal{}, false, nil
	}
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return Principal{}, false, nil
	}
	sum := sha256.Sum256([]byte(candidate))
	candHex := hex.EncodeToString(sum[:])

	now := s.now()
	for _, rec := range s.records {
		if subtle.ConstantTimeCompare([]byte(candHex), []byte(rec.SHA256Hex)) == 1 {
			if !rec.ExpiresAt.IsZero() && now.After(rec.ExpiresAt) {
				return Principal{}, true, &AuthError{StatusCode: http.StatusUnauthorized, Message: "scoped API key expired"}
			}
			return Principal{
				Subject:     rec.ID,
				Role:        rec.Role,
				AuthMethod:  "scoped-api-key",
				ClusterName: rec.ClusterName,
				ExpiresAt:   rec.ExpiresAt,
			}, true, nil
		}
	}

	if s.hasLegacyKey && subtle.ConstantTimeCompare([]byte(candHex), []byte(s.legacyKeyHash)) == 1 {
		return Principal{
			Subject:    "legacy-api-key",
			Role:       RoleAdmin,
			AuthMethod: "legacy-api-key",
		}, true, nil
	}

	return Principal{}, false, nil
}

// OIDCConfig configures stateless JWKS verification for corporate OIDC and GitHub Actions CI OIDC.
type OIDCConfig struct {
	IssuerURL           string
	ClientID            string
	JWKSURL             string // Optional override; derived via /.well-known/openid-configuration if empty.
	AdminGroups         []string
	CIGroups            []string
	ViewerGroups        []string
	GitHubIssuerURL     string // Defaults to https://token.actions.githubusercontent.com when AllowedRepositories is non-empty.
	AllowedRepositories []string
	HTTPClient          *http.Client
	CacheTTL            time.Duration
	RefreshCooldown     time.Duration
}

type oidcProvider struct {
	cfg              OIDCConfig
	httpClient       *http.Client
	mu               sync.RWMutex
	refreshMu        sync.Mutex
	keys             map[string]cachedJWK
	fetchedAt        time.Time
	lastFetchAttempt time.Time
	refreshCooldown  time.Duration
}

type cachedJWK struct {
	kty string
	alg string
	pub any
}

type jwksDocument struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func newOIDCProvider(cfg OIDCConfig) *oidcProvider {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 5 * time.Minute
	}
	if len(cfg.AdminGroups) == 0 {
		cfg.AdminGroups = []string{"reconcilyx-admins", "platform-admins"}
	}
	if len(cfg.CIGroups) == 0 {
		cfg.CIGroups = []string{"reconcilyx-ci", "ci-auditors"}
	}
	if cfg.GitHubIssuerURL == "" && len(cfg.AllowedRepositories) > 0 {
		cfg.GitHubIssuerURL = "https://token.actions.githubusercontent.com"
	}
	cooldown := cfg.RefreshCooldown
	if cooldown <= 0 {
		cooldown = defaultJWKSRefreshCooldown
	}
	return &oidcProvider{
		cfg:             cfg,
		httpClient:      client,
		keys:            make(map[string]cachedJWK),
		refreshCooldown: cooldown,
	}
}

func (p *oidcProvider) enabled() bool {
	return p != nil && (strings.TrimSpace(p.cfg.IssuerURL) != "" || len(p.cfg.AllowedRepositories) > 0)
}

func (p *oidcProvider) VerifyJWT(ctx context.Context, rawJWT string) (Principal, error) {
	parts := strings.Split(rawJWT, ".")
	if len(parts) != 3 {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "malformed JWT"}
	}

	hdrBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "invalid JWT header encoding"}
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "invalid JWT header"}
	}
	// Strict asymmetric algorithm pinning: explicitly reject "none", "HS*", etc.
	if hdr.Alg != "RS256" && hdr.Alg != "ES256" {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: fmt.Sprintf("unsupported or disallowed JWT algorithm %q", hdr.Alg)}
	}

	claimBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "invalid JWT claims encoding"}
	}
	var claims map[string]any
	if err := json.Unmarshal(claimBytes, &claims); err != nil {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "invalid JWT claims"}
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "invalid JWT signature encoding"}
	}

	iss, _ := claims["iss"].(string)
	isGitHub := p.cfg.GitHubIssuerURL != "" && strings.EqualFold(iss, p.cfg.GitHubIssuerURL)
	isCorp := p.cfg.IssuerURL != "" && strings.EqualFold(iss, p.cfg.IssuerURL)
	if !isGitHub && !isCorp {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: fmt.Sprintf("untrusted JWT issuer %q", iss)}
	}

	if err := verifyJWTTimestamps(claims, time.Now()); err != nil {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: err.Error()}
	}

	_, hasRepoClaim := claims["repository"].(string)
	treatAsGitHub := isGitHub && (!isCorp || hasRepoClaim)

	if !treatAsGitHub && isCorp && p.cfg.ClientID != "" && !matchesAudience(claims["aud"], p.cfg.ClientID) {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "JWT audience mismatch"}
	}

	jwk, err := p.resolveKey(ctx, iss, hdr.Kid)
	if err != nil {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: err.Error()}
	}
	if jwk.alg != "" && jwk.alg != hdr.Alg {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: fmt.Sprintf("JWT algorithm %q does not match JWK alg %q", hdr.Alg, jwk.alg)}
	}

	signingInput := []byte(parts[0] + "." + parts[1])
	if err := verifyJWTSignature(hdr.Alg, jwk.pub, signingInput, sigBytes); err != nil {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: fmt.Sprintf("invalid JWT signature: %v", err)}
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "JWT missing sub claim"}
	}

	if treatAsGitHub {
		repo, _ := claims["repository"].(string)
		if !matchesAny(repo, p.cfg.AllowedRepositories) {
			return Principal{}, &AuthError{StatusCode: http.StatusForbidden, Message: fmt.Sprintf("repository %q is not in allowlist", repo)}
		}
		return Principal{
			Subject:    "github:" + sub,
			Role:       RoleCIAuditor,
			AuthMethod: "github-oidc",
		}, nil
	}

	groups := extractStringSlice(claims["groups"])
	role := p.mapGroupsToRole(groups)
	if role == "" {
		return Principal{}, &AuthError{StatusCode: http.StatusForbidden, Message: "OIDC subject groups do not map to any Reconcilyx role"}
	}
	return Principal{
		Subject:    sub,
		Role:       role,
		AuthMethod: "oidc",
	}, nil
}

func (p *oidcProvider) mapGroupsToRole(groups []string) Role {
	for _, g := range groups {
		if matchesAny(g, p.cfg.AdminGroups) {
			return RoleAdmin
		}
	}
	for _, g := range groups {
		if matchesAny(g, p.cfg.CIGroups) {
			return RoleCIAuditor
		}
	}
	if len(p.cfg.ViewerGroups) == 0 {
		return RoleViewer
	}
	for _, g := range groups {
		if matchesAny(g, p.cfg.ViewerGroups) {
			return RoleViewer
		}
	}
	return ""
}

func (p *oidcProvider) lookupCachedJWK(kid string) (cachedJWK, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if time.Since(p.fetchedAt) >= p.cfg.CacheTTL {
		return cachedJWK{}, false
	}
	if k, ok := p.keys[kid]; ok {
		return k, true
	}
	if kid == "" && len(p.keys) == 1 {
		for _, k := range p.keys {
			return k, true
		}
	}
	return cachedJWK{}, false
}

func (p *oidcProvider) resolveKey(ctx context.Context, issuer, kid string) (cachedJWK, error) {
	if k, ok := p.lookupCachedJWK(kid); ok {
		return k, nil
	}

	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()

	// Re-check cache after acquiring refreshMu in case another goroutine just refreshed.
	if k, ok := p.lookupCachedJWK(kid); ok {
		return k, nil
	}

	// Rate-limit remote JWKS re-fetches on unknown kid values to protect the IdP.
	if !p.lastFetchAttempt.IsZero() && time.Since(p.lastFetchAttempt) < p.refreshCooldown {
		return cachedJWK{}, fmt.Errorf("JWKS key id %q not found (refresh cooldown active)", kid)
	}
	p.lastFetchAttempt = time.Now()

	if err := p.refreshJWKS(ctx, issuer); err != nil {
		return cachedJWK{}, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	if k, ok := p.keys[kid]; ok {
		return k, nil
	}
	if kid == "" && len(p.keys) == 1 {
		for _, k := range p.keys {
			return k, nil
		}
	}
	return cachedJWK{}, fmt.Errorf("JWKS key id %q not found", kid)
}

func (p *oidcProvider) refreshJWKS(ctx context.Context, issuer string) error {
	jwksURL := strings.TrimSpace(p.cfg.JWKSURL)
	if jwksURL == "" {
		discoURL := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoURL, nil)
		if err != nil {
			return err
		}
		resp, err := p.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("fetch OIDC discovery: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("OIDC discovery returned %d", resp.StatusCode)
		}
		var disco struct {
			JWKSURI string `json:"jwks_uri"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&disco); err != nil {
			return fmt.Errorf("decode OIDC discovery: %w", err)
		}
		jwksURL = disco.JWKSURI
	}
	if jwksURL == "" {
		return errors.New("OIDC JWKS URL could not be resolved")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	var doc jwksDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("decode JWKS: %w", err)
	}

	parsed := make(map[string]cachedJWK, len(doc.Keys))
	for _, k := range doc.Keys {
		pub, err := parseJWKPublicKey(k)
		if err != nil {
			continue
		}
		parsed[k.Kid] = cachedJWK{kty: k.Kty, alg: k.Alg, pub: pub}
	}
	if len(parsed) == 0 {
		return errors.New("JWKS contained no supported RSA/EC keys")
	}

	p.mu.Lock()
	p.keys = parsed
	p.fetchedAt = time.Now()
	p.mu.Unlock()
	return nil
}

func parseJWKPublicKey(k jwkKey) (any, error) {
	switch k.Kty {
	case "RSA":
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, err
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, err
		}
		e := 0
		for _, b := range eBytes {
			e = e<<8 + int(b)
		}
		if e <= 0 {
			return nil, errors.New("invalid RSA exponent")
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil

	case "EC":
		if k.Crv != "P-256" {
			return nil, fmt.Errorf("unsupported EC curve %q", k.Crv)
		}
		xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			return nil, err
		}
		yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			return nil, err
		}
		return &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(xBytes),
			Y:     new(big.Int).SetBytes(yBytes),
		}, nil

	default:
		return nil, fmt.Errorf("unsupported JWK kty %q", k.Kty)
	}
}

func verifyJWTSignature(alg string, pub any, signingInput, sig []byte) error {
	digest := sha256.Sum256(signingInput)
	switch alg {
	case "RS256":
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return errors.New("key type mismatch for RS256")
		}
		return rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, digest[:], sig)

	case "ES256":
		ecPub, ok := pub.(*ecdsa.PublicKey)
		if !ok {
			return errors.New("key type mismatch for ES256")
		}
		if len(sig) != 64 {
			return errors.New("invalid ES256 raw signature length")
		}
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		if !ecdsa.Verify(ecPub, digest[:], r, s) {
			return errors.New("ECDSA verification failed")
		}
		return nil

	default:
		return fmt.Errorf("unsupported JWT signing algorithm %q", alg)
	}
}

func verifyJWTTimestamps(claims map[string]any, now time.Time) error {
	skew := 30 * time.Second
	exp, hasExp := numericClaim(claims["exp"])
	if !hasExp {
		return errors.New("JWT missing required exp claim")
	}
	if now.Unix() > exp+int64(skew.Seconds()) {
		return errors.New("JWT token expired")
	}
	if nbf, hasNbf := numericClaim(claims["nbf"]); hasNbf {
		if now.Unix()+int64(skew.Seconds()) < nbf {
			return errors.New("JWT token not yet valid")
		}
	}
	return nil
}

func numericClaim(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}

// AuthenticateMTLS extracts a verified Spoke identity from TLS client peer certificates.
// It inspects SPIFFE URI SANs (spiffe://<domain>/.../cluster/<clusterName>) or leaf CN.
func AuthenticateMTLS(r *http.Request, clientCAPool *x509.CertPool) (Principal, bool, error) {
	if r == nil || r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return Principal{}, false, nil
	}
	leaf := r.TLS.PeerCertificates[0]

	// If PeerCertificates were not already verified against clientCAPool by the TLS listener,
	// or if a pool is explicitly provided here, verify the leaf chain deterministically.
	if clientCAPool != nil && len(r.TLS.VerifiedChains) == 0 {
		intermediates := x509.NewCertPool()
		for _, cert := range r.TLS.PeerCertificates[1:] {
			intermediates.AddCert(cert)
		}
		opts := x509.VerifyOptions{
			Roots:         clientCAPool,
			Intermediates: intermediates,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		if _, err := leaf.Verify(opts); err != nil {
			return Principal{}, true, &AuthError{StatusCode: http.StatusUnauthorized, Message: fmt.Sprintf("client certificate verification failed: %v", err)}
		}
	}

	clusterName := extractClusterFromCert(leaf)
	if clusterName == "" {
		return Principal{}, true, &AuthError{StatusCode: http.StatusForbidden, Message: "client certificate missing SPIFFE cluster URI SAN or CN"}
	}

	subject := leaf.Subject.CommonName
	for _, u := range leaf.URIs {
		if u.Scheme == "spiffe" {
			subject = u.String()
			break
		}
	}

	return Principal{
		Subject:     subject,
		Role:        RoleSpokeAgent,
		AuthMethod:  "mtls",
		ClusterName: clusterName,
	}, true, nil
}

func extractClusterFromCert(cert *x509.Certificate) string {
	for _, u := range cert.URIs {
		if u.Scheme == "spiffe" {
			trimmed := strings.Trim(u.Path, "/")
			parts := strings.Split(trimmed, "/")
			for i := 0; i+1 < len(parts); i++ {
				if parts[i] == "cluster" && parts[i+1] != "" {
					return parts[i+1]
				}
			}
			if len(parts) > 0 && parts[len(parts)-1] != "" {
				return parts[len(parts)-1]
			}
		}
	}
	return strings.TrimSpace(cert.Subject.CommonName)
}

// Config configures all multi-identity authentication strategies for the Hub API server.
type Config struct {
	LegacyAPIKey     string
	ScopedKeys       []string
	K8sReviewer      K8sTokenReviewer
	OIDC             OIDCConfig
	TLSClientCAPEM   []byte
	TLSClientCAFile  string
	AllowUnauthDev   bool
}

// ConfigFromEnv populates auth Config from RECONCILYX_* environment variables.
func ConfigFromEnv(legacyAPIKey string) Config {
	cfg := Config{
		LegacyAPIKey: legacyAPIKey,
		OIDC: OIDCConfig{
			IssuerURL:           strings.TrimSpace(os.Getenv("RECONCILYX_OIDC_ISSUER_URL")),
			ClientID:            strings.TrimSpace(os.Getenv("RECONCILYX_OIDC_CLIENT_ID")),
			JWKSURL:             strings.TrimSpace(os.Getenv("RECONCILYX_OIDC_JWKS_URL")),
			AdminGroups:         splitCSV(os.Getenv("RECONCILYX_OIDC_ADMIN_GROUPS")),
			CIGroups:            splitCSV(os.Getenv("RECONCILYX_OIDC_CI_GROUPS")),
			ViewerGroups:        splitCSV(os.Getenv("RECONCILYX_OIDC_VIEWER_GROUPS")),
			GitHubIssuerURL:     strings.TrimSpace(os.Getenv("RECONCILYX_GITHUB_OIDC_ISSUER")),
			AllowedRepositories: splitCSV(os.Getenv("RECONCILYX_GITHUB_OIDC_REPOS")),
		},
		TLSClientCAFile: strings.TrimSpace(os.Getenv("RECONCILYX_TLS_CLIENT_CA")),
	}
	if rawScoped := strings.TrimSpace(os.Getenv("RECONCILYX_SCOPED_API_KEYS")); rawScoped != "" {
		cfg.ScopedKeys = splitCSV(rawScoped)
	}
	return cfg
}

// Authenticator validates inbound HTTP requests across K8s TokenReview, OIDC JWKS,
// mTLS SPIFFE client certs, and hashed scoped API keys.
type Authenticator struct {
	keyStore     *ScopedKeyStore
	k8sReviewer  K8sTokenReviewer
	oidc         *oidcProvider
	clientCAPool *x509.CertPool
	allowDevMode bool
	legacyActive bool
}

// NewAuthenticator constructs a multi-strategy Authenticator.
func NewAuthenticator(cfg Config) (*Authenticator, error) {
	ks := NewScopedKeyStore(cfg.ScopedKeys, cfg.LegacyAPIKey)
	var caPool *x509.CertPool
	pemBytes := cfg.TLSClientCAPEM
	if len(pemBytes) == 0 && cfg.TLSClientCAFile != "" {
		readBytes, err := os.ReadFile(cfg.TLSClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("read TLS client CA file: %w", err)
		}
		pemBytes = readBytes
	}
	if len(pemBytes) > 0 {
		caPool = x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(pemBytes) {
			return nil, errors.New("invalid TLS client CA PEM bundle")
		}
	}

	oidc := newOIDCProvider(cfg.OIDC)

	anyAuthConfigured := len(ks.records) > 0 || ks.hasLegacyKey || cfg.K8sReviewer != nil || oidc.enabled() || caPool != nil
	allowDev := cfg.AllowUnauthDev && !anyAuthConfigured

	return &Authenticator{
		keyStore:     ks,
		k8sReviewer:  cfg.K8sReviewer,
		oidc:         oidc,
		clientCAPool: caPool,
		allowDevMode: allowDev,
		legacyActive: ks.hasLegacyKey,
	}, nil
}

// HasDeprecatedLegacyKey reports whether static single-key compatibility mode is active.
func (a *Authenticator) HasDeprecatedLegacyKey() bool {
	return a != nil && a.legacyActive
}

// IsDevMode reports whether unauthenticated dev fallback is active (no auth configured).
func (a *Authenticator) IsDevMode() bool {
	return a != nil && a.allowDevMode
}

// ClientCAPool returns the configured mTLS client CA pool, if any.
func (a *Authenticator) ClientCAPool() *x509.CertPool {
	if a == nil {
		return nil
	}
	return a.clientCAPool
}

// Authenticate verifies caller credentials on r and returns a verified Principal.
func (a *Authenticator) Authenticate(r *http.Request) (Principal, error) {
	if a == nil {
		return Principal{Subject: "dev-local", Role: RoleAdmin, AuthMethod: "dev"}, nil
	}

	// Strategy C: Spoke mTLS client certificate (highest cryptographic binding).
	if p, presented, err := AuthenticateMTLS(r, a.clientCAPool); presented {
		if err != nil {
			return Principal{}, err
		}
		return p, nil
	}

	// Extract token from X-API-Key, Authorization: Bearer, or reconcilyx_api_key cookie.
	token, tokenScheme := extractTokenFromRequest(r)
	if token == "" {
		if a.allowDevMode {
			return Principal{Subject: "dev-local", Role: RoleAdmin, AuthMethod: "dev"}, nil
		}
		return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "missing authentication credentials"}
	}

	// Explicit Kubernetes TokenReview hint or non-JWT Bearer token when K8sTokenReviewer is active.
	authMode := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Reconcilyx-Auth-Mode")))
	if authMode == "k8s-token" {
		if a.k8sReviewer == nil {
			return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "kubernetes token authentication not enabled on hub"}
		}
		ns := extractRequestedNamespace(r)
		return a.k8sReviewer.VerifyToken(r.Context(), token, ns)
	}

	// Strategy D: Scoped or legacy hashed API keys (`rcx_*` or configured API key).
	if p, matched, err := a.keyStore.Verify(token); matched {
		if err != nil {
			return Principal{}, err
		}
		return p, nil
	}

	// Strategy B: Stateless OIDC / GitHub Actions JWT (3 dot-separated segments).
	if strings.Count(token, ".") == 2 && a.oidc.enabled() {
		return a.oidc.VerifyJWT(r.Context(), token)
	}

	// Strategy A fallback: Bearer token sent without explicit hint -> try K8s TokenReview.
	if tokenScheme == "bearer" && a.k8sReviewer != nil {
		ns := extractRequestedNamespace(r)
		return a.k8sReviewer.VerifyToken(r.Context(), token, ns)
	}

	return Principal{}, &AuthError{StatusCode: http.StatusUnauthorized, Message: "invalid authentication token or API key"}
}

func extractTokenFromRequest(r *http.Request) (string, string) {
	if k := strings.TrimSpace(r.Header.Get("X-API-Key")); k != "" {
		return k, "api-key"
	}
	if authz := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		return strings.TrimSpace(authz[7:]), "bearer"
	}
	if c, err := r.Cookie("reconcilyx_api_key"); err == nil && strings.TrimSpace(c.Value) != "" {
		return strings.TrimSpace(c.Value), "cookie"
	}
	return "", ""
}

func extractRequestedNamespace(r *http.Request) string {
	if ns := strings.TrimSpace(r.Header.Get("X-Reconcilyx-Namespace")); ns != "" {
		return ns
	}
	return strings.TrimSpace(r.URL.Query().Get("namespace"))
}

func matchesAudience(audClaim any, target string) bool {
	switch v := audClaim.(type) {
	case string:
		return v == target
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == target {
				return true
			}
		}
	}
	return false
}

func extractStringSlice(v any) []string {
	rawList, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(rawList))
	for _, item := range rawList {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func matchesAny(target string, allow []string) bool {
	for _, entry := range allow {
		if strings.EqualFold(strings.TrimSpace(entry), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func nonEmptySlice(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return []string{s}
}
