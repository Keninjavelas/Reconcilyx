package federated

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ModelTagStable = "stable"
	ModelTagCanary = "canary"
)

type ModelRelease struct {
	VersionStr string        `json:"versionStr"` // "v42", "v43"
	Tag        string        `json:"tag"`        // "stable", "canary"
	Snapshot   *GlobalQTable `json:"snapshot"`
	CreatedAt  time.Time     `json:"createdAt"`
}

type ModelRegistry struct {
	mu        sync.RWMutex
	models    map[string]*ModelRelease
	stableVer string
	canaryVer string
}

func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models: make(map[string]*ModelRelease),
	}
}

// RegisterModel records a released snapshot under the stable or canary channel.
// Registering a canary never promotes it: a canary that arrives before any
// stable model stays opt-in only.
func (r *ModelRegistry) RegisterModel(versionStr, tag string, snapshot *GlobalQTable) error {
	versionStr = strings.TrimSpace(versionStr)
	tag = strings.ToLower(strings.TrimSpace(tag))
	if versionStr == "" {
		return fmt.Errorf("registry: version is required")
	}
	if snapshot == nil {
		return fmt.Errorf("registry: snapshot is required")
	}
	if tag != ModelTagStable && tag != ModelTagCanary {
		return fmt.Errorf("registry: unsupported tag %q (want stable or canary)", tag)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.models[versionStr] = &ModelRelease{
		VersionStr: versionStr,
		Tag:        tag,
		Snapshot:   snapshot,
		CreatedAt:  time.Now().UTC(),
	}
	switch tag {
	case ModelTagStable:
		r.stableVer = versionStr
		// A stable release supersedes any canary at the same or older version;
		// a newer canary stays opt-in.
		if r.canaryVer == versionStr {
			r.canaryVer = ""
		} else if canaryOrd, ok := versionOrdinal(r.canaryVer); ok {
			if stableOrd, ok := versionOrdinal(versionStr); ok && canaryOrd <= stableOrd {
				delete(r.models, r.canaryVer)
				r.canaryVer = ""
			}
		}
	case ModelTagCanary:
		r.canaryVer = versionStr
	}
	return nil
}

// Promote moves an existing release onto the stable channel. An empty version
// promotes the current canary.
func (r *ModelRegistry) Promote(versionStr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	versionStr = strings.TrimSpace(versionStr)
	if versionStr == "" {
		versionStr = r.canaryVer
	}
	if versionStr == "" {
		return fmt.Errorf("registry: no canary version to promote")
	}
	rel, ok := r.models[versionStr]
	if !ok {
		return fmt.Errorf("registry: version %q has not been released", versionStr)
	}
	rel.Tag = ModelTagStable
	r.stableVer = versionStr
	if r.canaryVer == versionStr {
		r.canaryVer = ""
	}
	return nil
}

// GetModelForCluster resolves the snapshot a spoke should load. Canary opt-in
// clusters receive the canary release when one exists; everything else gets
// the stable release.
func (r *ModelRegistry) GetModelForCluster(clusterName string, isCanaryOptIn bool) (*GlobalQTable, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if isCanaryOptIn && r.canaryVer != "" {
		if rel, ok := r.models[r.canaryVer]; ok {
			return rel.Snapshot, rel.VersionStr
		}
	}

	if rel, ok := r.models[r.stableVer]; ok {
		return rel.Snapshot, rel.VersionStr
	}

	return nil, ""
}

func (r *ModelRegistry) StableVersion() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stableVer
}

func (r *ModelRegistry) CanaryVersion() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.canaryVer
}

func (r *ModelRegistry) RollbackCanary(reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.canaryVer == "" {
		return fmt.Errorf("registry: no canary version active")
	}

	fmt.Printf("↺ Model Registry Rollback: Canary %s demoted (%s). Falling back to Stable %s.\n", r.canaryVer, reason, r.stableVer)
	delete(r.models, r.canaryVer)
	r.canaryVer = ""
	return nil
}

func versionOrdinal(version string) (int64, bool) {
	if !strings.HasPrefix(version, "v") {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(version, "v"), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
