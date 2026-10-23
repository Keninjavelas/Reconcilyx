package federated

import (
	"testing"
)

func TestModelRegistryCanaryAndRollback(t *testing.T) {
	registry := NewModelRegistry()

	v42 := &GlobalQTable{Version: 42, VersionStr: "v42", IsStable: true}
	v43 := &GlobalQTable{Version: 43, VersionStr: "v43", IsStable: false}

	if err := registry.RegisterModel("v42", "stable", v42); err != nil {
		t.Fatalf("RegisterModel stable failed: %v", err)
	}
	if err := registry.RegisterModel("v43", "canary", v43); err != nil {
		t.Fatalf("RegisterModel canary failed: %v", err)
	}

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

func TestModelRegistryCanaryDoesNotBecomeStable(t *testing.T) {
	registry := NewModelRegistry()
	v1 := &GlobalQTable{Version: 1, VersionStr: "v1"}

	if err := registry.RegisterModel("v1", "canary", v1); err != nil {
		t.Fatalf("RegisterModel failed: %v", err)
	}
	if got := registry.StableVersion(); got != "" {
		t.Fatalf("canary registered as stable: %q", got)
	}
	if snap, ver := registry.GetModelForCluster("spoke-a", false); snap != nil || ver != "" {
		t.Fatalf("expected no stable model, got %s", ver)
	}
	if _, ver := registry.GetModelForCluster("spoke-a", true); ver != "v1" {
		t.Fatalf("expected canary v1, got %s", ver)
	}
}

func TestModelRegistryPromoteCanary(t *testing.T) {
	registry := NewModelRegistry()
	v2 := &GlobalQTable{Version: 2, VersionStr: "v2"}
	v3 := &GlobalQTable{Version: 3, VersionStr: "v3"}
	_ = registry.RegisterModel("v2", "stable", v2)
	_ = registry.RegisterModel("v3", "canary", v3)

	if err := registry.Promote(""); err != nil {
		t.Fatalf("Promote failed: %v", err)
	}
	if got := registry.StableVersion(); got != "v3" {
		t.Fatalf("expected stable v3, got %s", got)
	}
	if got := registry.CanaryVersion(); got != "" {
		t.Fatalf("expected canary cleared after promotion, got %s", got)
	}
	if snap, ver := registry.GetModelForCluster("spoke-a", true); ver != "v3" || snap.Version != 3 {
		t.Fatalf("expected promoted model for canary opt-in, got %s", ver)
	}
}

func TestModelRegistryRejectsInvalidInput(t *testing.T) {
	registry := NewModelRegistry()
	if err := registry.RegisterModel("", "stable", &GlobalQTable{}); err == nil {
		t.Fatal("expected error for empty version")
	}
	if err := registry.RegisterModel("v1", "beta", &GlobalQTable{}); err == nil {
		t.Fatal("expected error for unsupported tag")
	}
	if err := registry.RegisterModel("v1", "stable", nil); err == nil {
		t.Fatal("expected error for nil snapshot")
	}
	if err := registry.Promote("v9"); err == nil {
		t.Fatal("expected error promoting unreleased version")
	}
}
