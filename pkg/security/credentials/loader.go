package credentials

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// DefaultMountDir is the Kubernetes projected/tmpfs secret mount root.
const DefaultMountDir = "/var/run/secrets/reconcilyx"

// ProviderSpec defines where a provider credential is read from on disk and which
// fallback environment variables are checked when the mounted file is absent.
type ProviderSpec struct {
	Name        string
	FileName    string // Defaults to "api_key" when empty.
	EnvFallback []string
}

// CredentialSource records whether a credential came from a file mount, an
// environment fallback, or was not configured.
type CredentialSource string

const (
	SourceNone CredentialSource = "none"
	SourceFile CredentialSource = "file-mount"
	SourceEnv  CredentialSource = "env-fallback"
)

// Entry holds the current secret and provenance for a provider.
type Entry struct {
	Secret   SecretString
	Source   CredentialSource
	FilePath string
}

// CredentialManager loads credentials from file mounts under BaseDir
// (<BaseDir>/<provider>/<fileName>) before falling back to environment variables,
// and watches mount directories with fsnotify for zero-downtime hot reload
// (including Kubernetes atomic ConfigMap/Secret ..data symlink swaps).
type CredentialManager struct {
	baseDir string

	mu      sync.RWMutex
	specs   map[string]ProviderSpec
	entries map[string]Entry
	onLoad  []func(provider string, entry Entry)

	watcher   *fsnotify.Watcher
	done      chan struct{}
	closeOnce sync.Once
}

// NewCredentialManager initializes a CredentialManager rooted at baseDir
// (DefaultMountDir when empty) for the given provider specifications.
// File-mounted secrets are preferred over environment fallbacks.
func NewCredentialManager(baseDir string, specs ...ProviderSpec) (*CredentialManager, error) {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = DefaultMountDir
	}
	m := &CredentialManager{
		baseDir: baseDir,
		specs:   make(map[string]ProviderSpec, len(specs)),
		entries: make(map[string]Entry, len(specs)),
		done:    make(chan struct{}),
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}
	m.watcher = watcher

	for _, spec := range specs {
		if spec.Name == "" {
			continue
		}
		if spec.FileName == "" {
			spec.FileName = "api_key"
		}
		m.specs[spec.Name] = spec
		m.reloadProviderLocked(spec)
		m.watchProviderDirs(spec)
	}

	// Watch baseDir itself if present so newly mounted provider subdirectories are detected.
	if info, statErr := os.Stat(baseDir); statErr == nil && info.IsDir() {
		_ = m.watcher.Add(baseDir)
	}

	go m.watchLoop()
	return m, nil
}

// Get returns the current SecretString for a provider (empty if unconfigured).
func (m *CredentialManager) Get(provider string) SecretString {
	if m == nil {
		return SecretString{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.entries[provider].Secret
}

// GetEntry returns both the SecretString and its provenance source.
func (m *CredentialManager) GetEntry(provider string) Entry {
	if m == nil {
		return Entry{Source: SourceNone}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if e, ok := m.entries[provider]; ok {
		return e
	}
	return Entry{Source: SourceNone}
}

// OnReload registers a callback invoked whenever a provider credential is reloaded.
func (m *CredentialManager) OnReload(fn func(provider string, entry Entry)) {
	if m == nil || fn == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onLoad = append(m.onLoad, fn)
}

// Reload forces an immediate re-read of all registered providers from disk/env.
func (m *CredentialManager) Reload() {
	if m == nil {
		return
	}
	m.mu.Lock()
	callbacks := make([]func(), 0, len(m.specs))
	for _, spec := range m.specs {
		changed, entry := m.reloadProviderLocked(spec)
		if changed {
			p := spec.Name
			e := entry
			for _, fn := range m.onLoad {
				cb := fn
				callbacks = append(callbacks, func() { cb(p, e) })
			}
		}
	}
	m.mu.Unlock()

	for _, cb := range callbacks {
		cb()
	}
}

// Close stops the fsnotify watch goroutine and releases OS handles.
func (m *CredentialManager) Close() error {
	if m == nil {
		return nil
	}
	var closeErr error
	m.closeOnce.Do(func() {
		close(m.done)
		if m.watcher != nil {
			closeErr = m.watcher.Close()
		}
	})
	return closeErr
}

// LoadFromFileOrEnv performs a standalone single-shot lookup checking
// <baseDir>/<provider>/<fileName> before envFallback without starting a watcher.
func LoadFromFileOrEnv(baseDir, provider, fileName string, envFallback ...string) Entry {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = DefaultMountDir
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = "api_key"
	}
	candidate := filepath.Join(baseDir, provider, fileName)
	if raw, err := os.ReadFile(candidate); err == nil {
		sec := NewSecretString(string(raw))
		if !sec.IsEmpty() {
			return Entry{
				Secret:   sec,
				Source:   SourceFile,
				FilePath: candidate,
			}
		}
	}
	for _, envName := range envFallback {
		if val := strings.TrimSpace(os.Getenv(envName)); val != "" {
			return Entry{
				Secret: NewSecretString(val),
				Source: SourceEnv,
			}
		}
	}
	return Entry{Source: SourceNone}
}

func (m *CredentialManager) reloadProviderLocked(spec ProviderSpec) (bool, Entry) {
	next := LoadFromFileOrEnv(m.baseDir, spec.Name, spec.FileName, spec.EnvFallback...)
	prev, existed := m.entries[spec.Name]
	m.entries[spec.Name] = next
	changed := !existed || prev.Secret.Expose() != next.Secret.Expose() || prev.Source != next.Source
	return changed, next
}

func (m *CredentialManager) watchProviderDirs(spec ProviderSpec) {
	if m.watcher == nil {
		return
	}
	// Watch the parent directory (never the leaf file inode) so Kubelet atomic
	// ..data symlink swaps keep firing reload events after directory replacement.
	providerDir := filepath.Join(m.baseDir, spec.Name)
	if info, err := os.Stat(providerDir); err == nil && info.IsDir() {
		_ = m.watcher.Add(providerDir)
	}
}

func (m *CredentialManager) shouldReloadForEvent(event fsnotify.Event) bool {
	if !event.Has(fsnotify.Create | fsnotify.Write | fsnotify.Rename | fsnotify.Chmod | fsnotify.Remove) {
		return false
	}
	base := filepath.Base(event.Name)
	if base == "..data" {
		return true
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, spec := range m.specs {
		if base == spec.FileName || base == spec.Name {
			return true
		}
	}
	return false
}

func (m *CredentialManager) watchLoop() {
	for {
		select {
		case <-m.done:
			return
		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			if !m.shouldReloadForEvent(event) {
				continue
			}
			m.mu.RLock()
			specs := make([]ProviderSpec, 0, len(m.specs))
			for _, spec := range m.specs {
				specs = append(specs, spec)
			}
			m.mu.RUnlock()
			for _, spec := range specs {
				m.watchProviderDirs(spec)
			}
			m.Reload()
		case _, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}
