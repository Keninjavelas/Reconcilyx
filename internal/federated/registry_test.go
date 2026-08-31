package federated

import (
	"testing"
)

func TestModelRegistryCanaryAndRollback(t *testing.T) {
	registry := NewModelRegistry()

	v42 := &GlobalQTable{Version: 42, VersionStr: "v42", IsStable: true}
	v43 := &GlobalQTable{Version: 43, VersionStr: "v43", IsStable: false}

	registry.RegisterModel("v42", "stable", v42)
	registry.RegisterModel("v43", "canary", v43)

	// 1. Regular cluster gets stable model v42
	snapStable, verStable := registry.GetModelForCluster("spoke-regular", false)
	if verStable != "v42" || snapStable.Version != 42 {
		t.Errorf("Expected regular cluster to receive v42 stable model, got %s", verStable)
	}

	// 2. Canary opt-in cluster gets canary model v43
	snapCanary, verCanary := registry.GetModelForCluster("spoke-canary", true)
	if verCanary != "v43" || snapCanary.Version != 43 {
		t.Errorf("Expected canary opt-in cluster to receive v43 canary model, got %s", verCanary)
	}

	// 3. Rollback canary on detected performance regression
	err := registry.RollbackCanary("Degraded RL reward signal")
	if err != nil {
		t.Fatalf("RollbackCanary failed: %v", err)
	}

	// 4. After rollback, canary opt-in cluster falls back to stable v42
	_, verPostRollback := registry.GetModelForCluster("spoke-canary", true)
	if verPostRollback != "v42" {
		t.Errorf("Expected post-rollback version to be v42, got %s", verPostRollback)
	}
}
