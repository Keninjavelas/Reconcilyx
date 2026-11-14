package credentials

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func replaceForTest(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.Rename(src, dst); err == nil {
		return
	} else if runtime.GOOS != "windows" {
		t.Fatalf("rename %s -> %s: %v", src, dst, err)
	}
	// Windows cannot atomically rename over an existing directory symlink (unlike POSIX rename(2))
	// and may briefly hold directory watcher handles during creation.
	_ = os.Remove(dst)
	var lastErr error
	for i := 0; i < 10; i++ {
		if err := os.Rename(src, dst); err == nil {
			return
		} else {
			lastErr = err
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("rename %s -> %s: %v", src, dst, lastErr)
}

func TestSecretStringRedactsEverywhere(t *testing.T) {
	raw := "super-secret-token-12345"
	sec := NewSecretString("  " + raw + "\n")

	if got := sec.Expose(); got != raw {
		t.Fatalf("Expose() = %q, want %q", got, raw)
	}
	if sec.IsEmpty() {
		t.Fatal("expected non-empty SecretString")
	}

	if got := fmt.Sprint(sec); got != redactedMask || strings.Contains(got, raw) {
		t.Fatalf("fmt.Sprint leaked or wrong mask: %q", got)
	}
	if got := fmt.Sprintf("%#v", sec); got != redactedMask || strings.Contains(got, raw) {
		t.Fatalf("fmt.Sprintf(%%#v) leaked or wrong mask: %q", got)
	}

	jsonBytes, err := json.Marshal(struct {
		Token SecretString `json:"token"`
	}{Token: sec})
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	if strings.Contains(string(jsonBytes), raw) || !strings.Contains(string(jsonBytes), redactedMask) {
		t.Fatalf("json.Marshal leaked or omitted mask: %s", jsonBytes)
	}

	textBytes, err := sec.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error: %v", err)
	}
	if string(textBytes) != redactedMask {
		t.Fatalf("MarshalText = %q, want %q", string(textBytes), redactedMask)
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	logger.Info("outbound auth", "credential", sec)
	if strings.Contains(logBuf.String(), raw) || !strings.Contains(logBuf.String(), redactedMask) {
		t.Fatalf("slog leaked secret: %s", logBuf.String())
	}

	var empty SecretString
	if !empty.IsEmpty() || empty.String() != "" {
		t.Fatalf("empty SecretString mismatch: isEmpty=%v str=%q", empty.IsEmpty(), empty.String())
	}
}

func TestCredentialManagerPrefersFileMountOverEnvAndHotReloads(t *testing.T) {
	tmpDir := t.TempDir()
	providerDir := filepath.Join(tmpDir, "electricitymaps")
	if err := os.MkdirAll(providerDir, 0o755); err != nil {
		t.Fatalf("mkdir provider: %v", err)
	}
	secretFile := filepath.Join(providerDir, "api_key")
	if err := os.WriteFile(secretFile, []byte("file-key-v1\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}

	t.Setenv("TEST_EMAPS_KEY", "env-key-fallback")

	mgr, err := NewCredentialManager(tmpDir, ProviderSpec{
		Name:        "electricitymaps",
		FileName:    "api_key",
		EnvFallback: []string{"TEST_EMAPS_KEY"},
	}, ProviderSpec{
		Name:        "watttime",
		FileName:    "api_key",
		EnvFallback: []string{"TEST_WATTTIME_KEY"},
	})
	if err != nil {
		t.Fatalf("NewCredentialManager failed: %v", err)
	}
	defer func() { _ = mgr.Close() }()

	// File mount wins over env fallback.
	entry := mgr.GetEntry("electricitymaps")
	if entry.Source != SourceFile || entry.Secret.Expose() != "file-key-v1" {
		t.Fatalf("expected file-mount secret file-key-v1, got source=%s val=%q", entry.Source, entry.Secret.Expose())
	}

	// Missing mount -> none until env is set.
	if wt := mgr.GetEntry("watttime"); wt.Source != SourceNone || !wt.Secret.IsEmpty() {
		t.Fatalf("expected watttime empty, got %+v", wt)
	}

	var reloads atomic.Int32
	mgr.OnReload(func(provider string, updated Entry) {
		if provider == "electricitymaps" && updated.Secret.Expose() == "file-key-v2" {
			reloads.Add(1)
		}
	})

	// Simulate Kubernetes atomic ..data symlink / atomic replace rotation.
	if err := os.WriteFile(secretFile+".tmp", []byte("file-key-v2\n"), 0o600); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}
	replaceForTest(t, secretFile+".tmp", secretFile)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if mgr.Get("electricitymaps").Expose() == "file-key-v2" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	if got := mgr.Get("electricitymaps").Expose(); got != "file-key-v2" {
		t.Fatalf("hot reload did not update secret in time: got %q, want file-key-v2", got)
	}
	if reloads.Load() == 0 {
		t.Fatal("expected OnReload callback to fire at least once")
	}
}

func TestLoadFromFileOrEnvFallback(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TEST_FALLBACK_TOKEN", "env-secret-value")

	entry := LoadFromFileOrEnv(tmpDir, "github", "token", "TEST_FALLBACK_TOKEN")
	if entry.Source != SourceEnv || entry.Secret.Expose() != "env-secret-value" {
		t.Fatalf("expected env fallback, got source=%s val=%q", entry.Source, entry.Secret.Expose())
	}
}

func TestCredentialManagerKubeletDotDataSymlinkRotation(t *testing.T) {
	tmpDir := t.TempDir()
	providerDir := filepath.Join(tmpDir, "gcp")
	dirV1 := filepath.Join(providerDir, "..2026_11_13_001")
	dirV2 := filepath.Join(providerDir, "..2026_11_13_002")
	if err := os.MkdirAll(dirV1, 0o755); err != nil {
		t.Fatalf("mkdir v1: %v", err)
	}
	if err := os.MkdirAll(dirV2, 0o755); err != nil {
		t.Fatalf("mkdir v2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirV1, "api_key"), []byte("gcp-k8s-secret-v1\n"), 0o600); err != nil {
		t.Fatalf("write v1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirV2, "api_key"), []byte("gcp-k8s-secret-v2\n"), 0o600); err != nil {
		t.Fatalf("write v2: %v", err)
	}

	dotData := filepath.Join(providerDir, "..data")
	if err := os.Symlink("..2026_11_13_001", dotData); err != nil {
		t.Skipf("symlinks require Developer Mode / SeCreateSymbolicLinkPrivilege on Windows: %v", err)
	}
	if err := os.Symlink(filepath.Join("..data", "api_key"), filepath.Join(providerDir, "api_key")); err != nil {
		t.Fatalf("symlink leaf: %v", err)
	}

	mgr, err := NewCredentialManager(tmpDir, ProviderSpec{
		Name:     "gcp",
		FileName: "api_key",
	})
	if err != nil {
		t.Fatalf("NewCredentialManager: %v", err)
	}
	defer func() { _ = mgr.Close() }()

	if got := mgr.Get("gcp").Expose(); got != "gcp-k8s-secret-v1" {
		t.Fatalf("initial secret = %q, want gcp-k8s-secret-v1", got)
	}

	// Simulate Kubelet atomic ..data symlink swap via ..data_tmp -> ..data
	dotDataTmp := filepath.Join(providerDir, "..data_tmp")
	if err := os.Symlink("..2026_11_13_002", dotDataTmp); err != nil {
		t.Fatalf("symlink tmp: %v", err)
	}
	replaceForTest(t, dotDataTmp, dotData)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if mgr.Get("gcp").Expose() == "gcp-k8s-secret-v2" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if got := mgr.Get("gcp").Expose(); got != "gcp-k8s-secret-v2" {
		t.Fatalf("after ..data symlink swap got %q, want gcp-k8s-secret-v2", got)
	}
}
