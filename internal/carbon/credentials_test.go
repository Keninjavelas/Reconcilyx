package carbon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/reconcilyx/operator/pkg/security/credentials"
)

func TestCarbonFileMountPreferenceAndHotReload(t *testing.T) {
	secDir := t.TempDir()
	emDir := filepath.Join(secDir, "electricitymaps")
	wtDir := filepath.Join(secDir, "watttime")
	_ = os.MkdirAll(emDir, 0o755)
	_ = os.MkdirAll(wtDir, 0o755)

	emFile := filepath.Join(emDir, "api_key")
	_ = os.WriteFile(emFile, []byte("em-file-key-v1\n"), 0o600)
	_ = os.WriteFile(filepath.Join(wtDir, "api_key"), []byte("wt-file-key-v1\n"), 0o600)

	t.Setenv("RECONCILYX_SECRETS_DIR", secDir)
	t.Setenv("RECONCILYX_EM_API_KEY", "em-env-ignored")
	t.Setenv("RECONCILYX_WT_API_KEY", "wt-env-ignored")

	cfg := CarbonConfigFromEnv()
	if cfg.ElectricityMaps.APIKey != "em-file-key-v1" {
		t.Fatalf("expected file-mounted EM key, got %q", cfg.ElectricityMaps.APIKey)
	}
	if cfg.WattTime.APIKey != "wt-file-key-v1" {
		t.Fatalf("expected file-mounted WT key, got %q", cfg.WattTime.APIKey)
	}

	mgr, err := credentials.NewCredentialManager(secDir, credentials.ProviderSpec{
		Name:     "electricitymaps",
		FileName: "api_key",
	})
	if err != nil {
		t.Fatalf("NewCredentialManager: %v", err)
	}
	defer func() { _ = mgr.Close() }()

	var seenTokens []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenTokens = append(seenTokens, r.Header.Get("auth-token"))
		_ = json.NewEncoder(w).Encode(map[string]any{"carbonIntensity": 120.5})
	}))
	defer srv.Close()

	p := NewElectricityMapsProvider("")
	p.BaseURL = srv.URL
	p.SecretSource = func() credentials.SecretString { return mgr.Get("electricitymaps") }

	if _, err := p.IntensityForRegion(context.Background(), "US-CAL-CISO"); err != nil {
		t.Fatalf("first IntensityForRegion: %v", err)
	}

	// Rotate key on disk and wait for fsnotify reload.
	_ = os.WriteFile(emFile+".tmp", []byte("em-file-key-v2\n"), 0o600)
	_ = os.Rename(emFile+".tmp", emFile)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if mgr.Get("electricitymaps").Expose() == "em-file-key-v2" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	if _, err := p.IntensityForRegion(context.Background(), "US-CAL-CISO"); err != nil {
		t.Fatalf("second IntensityForRegion: %v", err)
	}
	if len(seenTokens) != 2 || seenTokens[0] != "em-file-key-v1" || seenTokens[1] != "em-file-key-v2" {
		t.Fatalf("expected zero-downtime hot-reloaded tokens, got %v", seenTokens)
	}
}
