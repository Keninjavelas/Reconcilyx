package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
)

func auditScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := policyGuardScheme(t)
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	return scheme
}

func auditProposal() *optimizationv1alpha1.OptimizationProposal {
	return &optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{Name: "proposal-one", Namespace: "default"},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			TargetWorkload: corev1.ObjectReference{Kind: "Deployment", Namespace: "default", Name: "demo"},
		},
	}
}

func TestAuditorAppendsEntries(t *testing.T) {
	proposal := auditProposal()
	client := fake.NewClientBuilder().WithScheme(auditScheme(t)).Build()
	auditor := NewAuditor(client)

	policy := &optimizationv1alpha1.OptimizationPolicy{ObjectMeta: metav1.ObjectMeta{Name: "guard-policy"}}
	if err := auditor.RecordEvaluation(context.Background(), proposal, policy, "Approved: constraints passed"); err != nil {
		t.Fatalf("record evaluation: %v", err)
	}
	if err := auditor.RecordExecutionAttempt(context.Background(), proposal, "allowed: JIT validation passed"); err != nil {
		t.Fatalf("record execution: %v", err)
	}

	configMap := &corev1.ConfigMap{}
	key := types.NamespacedName{Namespace: "default", Name: auditConfigMapName}
	if err := client.Get(context.Background(), key, configMap); err != nil {
		t.Fatalf("get audit configmap: %v", err)
	}
	lines := strings.Split(configMap.Data[auditConfigMapKey], "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 audit entries, got %d: %v", len(lines), lines)
	}

	var evaluation auditEntry
	if err := json.Unmarshal([]byte(lines[0]), &evaluation); err != nil {
		t.Fatalf("parse evaluation entry: %v", err)
	}
	if evaluation.Action != "policy-evaluation" || evaluation.Policy != "guard-policy" ||
		evaluation.Proposal != "proposal-one" || evaluation.Namespace != "default" ||
		evaluation.Workload != "Deployment/default/demo" || evaluation.Actor != auditActor {
		t.Fatalf("unexpected evaluation entry: %+v", evaluation)
	}

	var execution auditEntry
	if err := json.Unmarshal([]byte(lines[1]), &execution); err != nil {
		t.Fatalf("parse execution entry: %v", err)
	}
	if execution.Action != "execution-attempt" || !strings.Contains(execution.Decision, "allowed") {
		t.Fatalf("unexpected execution entry: %+v", execution)
	}
}

func TestAuditorCapsEntries(t *testing.T) {
	existing := make([]string, maxAuditEntries)
	for i := range existing {
		existing[i] = fmt.Sprintf(`{"action":"policy-evaluation","decision":"entry-%d"}`, i)
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: auditConfigMapName, Namespace: "default"},
		Data:       map[string]string{auditConfigMapKey: strings.Join(existing, "\n")},
	}
	client := fake.NewClientBuilder().WithScheme(auditScheme(t)).WithObjects(configMap).Build()

	if err := NewAuditor(client).RecordExecutionAttempt(context.Background(), auditProposal(), "new entry"); err != nil {
		t.Fatalf("record execution: %v", err)
	}

	updated := &corev1.ConfigMap{}
	key := types.NamespacedName{Namespace: "default", Name: auditConfigMapName}
	if err := client.Get(context.Background(), key, updated); err != nil {
		t.Fatalf("get audit configmap: %v", err)
	}
	lines := strings.Split(updated.Data[auditConfigMapKey], "\n")
	if len(lines) != maxAuditEntries {
		t.Fatalf("expected %d entries, got %d", maxAuditEntries, len(lines))
	}
	if !strings.Contains(lines[0], "entry-1") {
		t.Fatalf("expected the oldest entry to be trimmed, first line: %s", lines[0])
	}
	if !strings.Contains(lines[len(lines)-1], "new entry") {
		t.Fatalf("expected the newest entry last, last line: %s", lines[len(lines)-1])
	}
}

func TestAuditorWithoutClientIsNoop(t *testing.T) {
	auditor := NewAuditor(nil)
	if err := auditor.RecordEvaluation(context.Background(), auditProposal(), nil, "decision"); err != nil {
		t.Fatalf("nil client evaluation: %v", err)
	}
	if err := auditor.RecordExecutionAttempt(context.Background(), auditProposal(), "decision"); err != nil {
		t.Fatalf("nil client execution: %v", err)
	}
}
