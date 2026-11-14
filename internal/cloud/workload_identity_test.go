package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAWSWebIdentitySTSExchangeAndPricing(t *testing.T) {
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "token")
	if err := os.WriteFile(tokenFile, []byte("projected-k8s-oidc-jwt\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	var gotAction, gotRole, gotToken, gotAuthz string
	stsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.FormValue("Action")
		gotRole = r.FormValue("RoleArn")
		gotToken = r.FormValue("WebIdentityToken")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprint(w, `<AssumeRoleWithWebIdentityResponse>
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>ASIAEXAMPLE123</AccessKeyId>
      <SecretAccessKey>temp-secret-xyz</SecretAccessKey>
      <SessionToken>temp-session-tok</SessionToken>
      <Expiration>2099-01-01T00:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleWithWebIdentityResult>
</AssumeRoleWithWebIdentityResponse>`)
	}))
	defer stsSrv.Close()

	pricingSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthz = r.Header.Get("Authorization")
		if secTok := r.Header.Get("X-Amz-Security-Token"); secTok != "temp-session-tok" {
			t.Errorf("expected X-Amz-Security-Token=temp-session-tok, got %q", secTok)
		}
		itemBytes, _ := json.Marshal(map[string]any{
			"product": map[string]any{
				"attributes": map[string]any{"instanceType": "c6i.large"},
			},
			"terms": map[string]any{
				"OnDemand": map[string]any{
					"SKU": map[string]any{
						"TERM": map[string]any{
							"priceDimensions": map[string]any{
								"DIM": map[string]any{
									"pricePerUnit": map[string]string{"USD": "0.088"},
								},
							},
						},
					},
				},
			},
		})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"PriceList": []string{string(itemBytes)},
		})
	}))
	defer pricingSrv.Close()

	p := NewWebIdentityAWSPricingProvider("arn:aws:iam::123456789012:role/reconcilyx", tokenFile, stsSrv.URL)
	p.BaseURL = pricingSrv.URL
	if !p.Live() || p.Source() != SourceAWSLive {
		t.Fatalf("expected live AWS source, got live=%v source=%s", p.Live(), p.Source())
	}

	rate, err := p.OnDemandPrice(context.Background(), "c6i.large", "us-east-1")
	if err != nil {
		t.Fatalf("OnDemandPrice: %v", err)
	}
	if rate != 0.088 {
		t.Fatalf("rate = %v, want 0.088", rate)
	}
	if gotAction != "AssumeRoleWithWebIdentity" || gotRole != "arn:aws:iam::123456789012:role/reconcilyx" || gotToken != "projected-k8s-oidc-jwt" {
		t.Fatalf("unexpected STS form params: action=%s role=%s tok=%s", gotAction, gotRole, gotToken)
	}
	if gotAuthz == "" {
		t.Fatal("expected SigV4 Authorization header built from STS credentials")
	}
}

func TestGCPWorkloadIdentityMetadataAndFileMount(t *testing.T) {
	var gotMetadataHeader, gotBearer string
	metaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMetadataHeader = r.Header.Get("Metadata-Flavor")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ya29.ephemeral-wi-token",
			"expires_in":   3600,
		})
	}))
	defer metaSrv.Close()

	billingSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBearer = r.Header.Get("Authorization")
		if r.URL.Query().Get("key") != "" {
			t.Errorf("expected no static ?key query param in workload identity mode, got %q", r.URL.Query().Get("key"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"skus": []map[string]any{
				{
					"description":    "E2 Instance Core running in Americas",
					"category":       map[string]string{"resourceFamily": "Compute", "usageType": "OnDemand"},
					"serviceRegions": []string{"us-central1"},
					"pricingInfo": []map[string]any{{
						"pricingExpression": map[string]any{
							"usageUnit": "h",
							"tieredRates": []map[string]any{{
								"unitPrice": map[string]any{"currencyCode": "USD", "units": "0", "nanos": 22000000},
							}},
						},
					}},
				},
				{
					"description":    "E2 Instance Ram running in Americas",
					"category":       map[string]string{"resourceFamily": "Compute", "usageType": "OnDemand"},
					"serviceRegions": []string{"us-central1"},
					"pricingInfo": []map[string]any{{
						"pricingExpression": map[string]any{
							"usageUnit": "h",
							"tieredRates": []map[string]any{{
								"unitPrice": map[string]any{"currencyCode": "USD", "units": "0", "nanos": 3000000},
							}},
						},
					}},
				},
			},
		})
	}))
	defer billingSrv.Close()

	g := NewWorkloadIdentityGCPPricingProvider(metaSrv.URL)
	g.BaseURL = billingSrv.URL
	rate, err := g.OnDemandPrice(context.Background(), "e2-standard-2", "us-central1")
	if err != nil {
		t.Fatalf("GCP WI OnDemandPrice: %v", err)
	}
	if rate <= 0 {
		t.Fatalf("expected positive live GCP WI rate, got %v", rate)
	}
	if gotMetadataHeader != "Google" || gotBearer != "Bearer ya29.ephemeral-wi-token" {
		t.Fatalf("unexpected headers metadata=%q authz=%q", gotMetadataHeader, gotBearer)
	}

	// Verify file-mounted GCP key overrides env var in PricingConfigFromEnv
	secDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(secDir, "gcp"), 0o755)
	_ = os.WriteFile(filepath.Join(secDir, "gcp", "api_key"), []byte("gcp-mounted-key\n"), 0o600)
	t.Setenv("RECONCILYX_SECRETS_DIR", secDir)
	t.Setenv("RECONCILYX_PRICING_GCP_API_KEY", "gcp-env-key")

	cfg := PricingConfigFromEnv()
	if cfg.GCPAPIKey != "gcp-mounted-key" {
		t.Fatalf("expected file-mounted gcp key, got %q", cfg.GCPAPIKey)
	}
}
