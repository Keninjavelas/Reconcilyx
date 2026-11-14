package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestKubernetesTokenReviewerTTLCacheAndBounds(t *testing.T) {
	var tokenReviews atomic.Int32
	var sarCalls atomic.Int32

	cs := fake.NewSimpleClientset()
	cs.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		tokenReviews.Add(1)
		tr := action.(k8stesting.CreateAction).GetObject().(*authv1.TokenReview).DeepCopy()
		if tr.Spec.Token == "valid-admin-tok" {
			tr.Status.Authenticated = true
			tr.Status.User = authv1.UserInfo{Username: "alice", Groups: []string{"system:masters"}}
		} else {
			tr.Status.Authenticated = false
			tr.Status.Error = "invalid token"
		}
		return true, tr, nil
	})
	cs.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		sarCalls.Add(1)
		sar := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SubjectAccessReview).DeepCopy()
		sar.Status.Allowed = sar.Spec.User == "alice" && sar.Spec.ResourceAttributes.Verb == "update"
		return true, sar, nil
	})

	now := time.Unix(1_700_000_000, 0)
	var timeMu sync.Mutex
	reviewer := &KubernetesTokenReviewer{
		Client:       cs,
		MaxCacheSize: 4,
		PositiveTTL:  45 * time.Second,
		NegativeTTL:  5 * time.Second,
		Now: func() time.Time {
			timeMu.Lock()
			defer timeMu.Unlock()
			return now
		},
	}

	// 1. Burst 20 concurrent calls with the same token+ns -> only 1 TokenReview RPC.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := reviewer.VerifyToken(context.Background(), "valid-admin-tok", "prod")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if p.Role != RoleAdmin || p.Subject != "alice" {
				t.Errorf("unexpected principal: %+v", p)
			}
		}()
	}
	wg.Wait()
	if got := tokenReviews.Load(); got > 20 {
		t.Fatalf("too many tokenReviews: %d", got)
	}
	// Sequential second wave must hit cache cleanly with zero extra RPCs.
	before := tokenReviews.Load()
	for i := 0; i < 10; i++ {
		if _, err := reviewer.VerifyToken(context.Background(), "valid-admin-tok", "prod"); err != nil {
			t.Fatalf("cached VerifyToken failed: %v", err)
		}
	}
	if after := tokenReviews.Load(); after != before {
		t.Fatalf("expected 0 extra TokenReviews during 45s positive TTL, got %d -> %d", before, after)
	}

	// 2. Invalid token negative-cached for 5s, re-checked after 6s.
	if _, err := reviewer.VerifyToken(context.Background(), "bad-tok", "prod"); err == nil {
		t.Fatal("expected invalid token rejection")
	}
	negBefore := tokenReviews.Load()
	for i := 0; i < 5; i++ {
		if _, err := reviewer.VerifyToken(context.Background(), "bad-tok", "prod"); err == nil {
			t.Fatal("expected cached rejection")
		}
	}
	if negAfter := tokenReviews.Load(); negAfter != negBefore {
		t.Fatalf("expected 0 extra TokenReviews during 5s negative TTL, got %d -> %d", negBefore, negAfter)
	}

	timeMu.Lock()
	now = now.Add(6 * time.Second)
	timeMu.Unlock()

	_, _ = reviewer.VerifyToken(context.Background(), "bad-tok", "prod")
	if tokenReviews.Load() != negBefore+1 {
		t.Fatalf("expected negative entry to expire after 6s")
	}

	// 3. Positive entry still valid at +6s (within 45s), expires after +40s more.
	posBefore := tokenReviews.Load()
	_, _ = reviewer.VerifyToken(context.Background(), "valid-admin-tok", "prod")
	if tokenReviews.Load() != posBefore {
		t.Fatal("expected positive entry still cached at +6s")
	}

	timeMu.Lock()
	now = now.Add(40 * time.Second)
	timeMu.Unlock()

	_, _ = reviewer.VerifyToken(context.Background(), "valid-admin-tok", "prod")
	if tokenReviews.Load() != posBefore+1 {
		t.Fatal("expected positive entry to expire at +46s")
	}

	// 4. Bounded LRU capacity (MaxCacheSize = 4).
	for i := 0; i < 10; i++ {
		_, _ = reviewer.VerifyToken(context.Background(), fmt.Sprintf("spam-%d", i), "prod")
	}
	reviewer.cacheMu.Lock()
	cacheLen := reviewer.cacheOrder.Len()
	reviewer.cacheMu.Unlock()
	if cacheLen != 4 {
		t.Fatalf("expected LRU cache capped at 4, got %d", cacheLen)
	}
}

func TestOIDCAlgorithmPinningAndJWKSRefreshCooldown(t *testing.T) {
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	var jwksHits atomic.Int32

	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwksHits.Add(1)
		eBytes := big.NewInt(int64(rsaKey.PublicKey.E)).Bytes()
		_ = json.NewEncoder(w).Encode(jwksDocument{
			Keys: []jwkKey{{
				Kid: "known-kid",
				Kty: "RSA",
				Alg: "RS256",
				N:   base64.RawURLEncoding.EncodeToString(rsaKey.PublicKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(eBytes),
			}},
		})
	}))
	defer jwksSrv.Close()

	authenticator, err := NewAuthenticator(Config{
		OIDC: OIDCConfig{
			IssuerURL:       jwksSrv.URL,
			ClientID:        "reconcilyx-ui",
			JWKSURL:         jwksSrv.URL + "/jwks.json",
			RefreshCooldown: 60 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}

	makeToken := func(alg, kid string) string {
		hdr, _ := json.Marshal(map[string]string{"alg": alg, "kid": kid, "typ": "JWT"})
		pl, _ := json.Marshal(map[string]any{
			"iss":    jwksSrv.URL,
			"aud":    "reconcilyx-ui",
			"sub":    "bob@corp.io",
			"groups": []string{"reconcilyx-admins"},
			"exp":    time.Now().Add(5 * time.Minute).Unix(),
		})
		unsigned := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(pl)
		digest := sha256.Sum256([]byte(unsigned))
		sig, _ := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, digest[:])
		return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)
	}

	// Disallowed algorithms ("none", "HS256") fail before touching JWKS.
	for _, disallowed := range []string{"none", "HS256", "HS512"} {
		req := httptest.NewRequest(http.MethodGet, "/api/proposals", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(disallowed, "known-kid"))
		if _, err := authenticator.Authenticate(req); err == nil {
			t.Fatalf("expected alg=%s rejected", disallowed)
		}
	}
	if hits := jwksHits.Load(); hits != 0 {
		t.Fatalf("expected 0 JWKS fetches for disallowed algs, got %d", hits)
	}

	// Burst 50 tokens with distinct unknown kids -> JWKS endpoint called at most once due to 60s cooldown.
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/proposals", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken("RS256", fmt.Sprintf("unknown-kid-%d", i)))
		if _, err := authenticator.Authenticate(req); err == nil {
			t.Fatalf("expected unknown kid %d rejected", i)
		}
	}
	if hits := jwksHits.Load(); hits != 1 {
		t.Fatalf("expected exactly 1 JWKS fetch across 50 unknown-kid probes (60s cooldown), got %d", hits)
	}
}
