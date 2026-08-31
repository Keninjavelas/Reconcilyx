package federated

import (
	"fmt"
	"sync"
	"time"
)

type ModelRelease struct {
	VersionStr string        `json:"versionStr"` // "v42", "v43"
	Tag        string        `json:"tag"`        // "stable", "canary"
	Snapshot   *GlobalQTable `json:"snapshot"`
	CreatedAt  time.Time     `json:"createdAt"`
}

type ModelRegistry struct {
	mu         sync.RWMutex
	models     map[string]*ModelRelease
	stableVer  string
	canaryVer  string
}

func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models: make(map[string]*ModelRelease),
	}
}

func (r *ModelRegistry) RegisterModel(versionStr, tag string, snapshot *GlobalQTable) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rel := &ModelRelease{
		VersionStr: versionStr,
		Tag:        tag,
		Snapshot:   snapshot,
		CreatedAt:  time.Now().UTC(),
	}

	r.models[versionStr] = rel

	if tag == "stable" || r.stableVer == "" {
		r.stableVer = versionStr
	} else if tag == "canary" {
		r.canaryVer = versionStr
	}
}

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

func (r *ModelRegistry) RollbackCanary(reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.canaryVer == "" {
		return fmt.Errorf("registry: no canary version active")
	}

	fmt.Printf("↺ Model Registry Rollback: Canary %s demoted (%s). Falling back to Stable %s.\n", r.canaryVer, reason, r.stableVer)
	r.canaryVer = ""
	return nil
}
