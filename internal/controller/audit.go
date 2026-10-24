package controller

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/reconcilyx/operator/api/v1alpha1"
)

const (
	auditConfigMapName = "reconcilyx-audit"
	auditConfigMapKey  = "audit.jsonl"
	maxAuditEntries    = 500
	auditActor         = "reconcilyx-controller"
)

// Auditor records critical execution decisions for tracing.
type Auditor interface {
	RecordExecutionAttempt(ctx context.Context, proposal *v1alpha1.OptimizationProposal, decision string) error
	RecordEvaluation(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy, decision string) error
}

// auditEntry is one newline-delimited JSON record in the audit ConfigMap.
type auditEntry struct {
	Time      time.Time `json:"time"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Proposal  string    `json:"proposal"`
	Namespace string    `json:"namespace"`
	Policy    string    `json:"policy,omitempty"`
	Workload  string    `json:"workload,omitempty"`
	Decision  string    `json:"decision"`
}

type defaultAuditor struct {
	client client.Client
}

func NewAuditor(c client.Client) Auditor {
	return &defaultAuditor{client: c}
}

func (a *defaultAuditor) RecordExecutionAttempt(ctx context.Context, proposal *v1alpha1.OptimizationProposal, decision string) error {
	return a.append(ctx, proposal, "execution-attempt", "", decision)
}

func (a *defaultAuditor) RecordEvaluation(ctx context.Context, proposal *v1alpha1.OptimizationProposal, policy *v1alpha1.OptimizationPolicy, decision string) error {
	policyName := ""
	if policy != nil {
		policyName = policy.Name
	}
	return a.append(ctx, proposal, "policy-evaluation", policyName, decision)
}

func (a *defaultAuditor) append(ctx context.Context, proposal *v1alpha1.OptimizationProposal, action, policyName, decision string) error {
	if a == nil || a.client == nil || proposal == nil {
		return nil
	}
	entry := auditEntry{
		Time:      time.Now().UTC(),
		Actor:     auditActor,
		Action:    action,
		Proposal:  proposal.Name,
		Namespace: proposal.Namespace,
		Policy:    policyName,
		Workload:  proposal.Spec.TargetWorkload.Kind + "/" + probeWorkloadName(proposal),
		Decision:  decision,
	}
	line, err := encodeAuditEntry(entry)
	if err != nil {
		return err
	}

	key := types.NamespacedName{Namespace: proposal.Namespace, Name: auditConfigMapName}
	retryable := func(err error) bool {
		return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
	}
	return retry.OnError(retry.DefaultRetry, retryable, func() error {
		configMap := &corev1.ConfigMap{}
		err := a.client.Get(ctx, key, configMap)
		if apierrors.IsNotFound(err) {
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      auditConfigMapName,
					Namespace: proposal.Namespace,
					Labels: map[string]string{
						"app.kubernetes.io/name":      "reconcilyx",
						"app.kubernetes.io/component": "audit",
					},
				},
				Data: map[string]string{auditConfigMapKey: line},
			}
			return a.client.Create(ctx, configMap)
		}
		if err != nil {
			return err
		}

		if configMap.Data == nil {
			configMap.Data = map[string]string{}
		}
		lines := make([]string, 0, maxAuditEntries)
		if current := strings.TrimSpace(configMap.Data[auditConfigMapKey]); current != "" {
			lines = append(lines, strings.Split(current, "\n")...)
		}
		lines = append(lines, line)
		if len(lines) > maxAuditEntries {
			lines = lines[len(lines)-maxAuditEntries:]
		}
		configMap.Data[auditConfigMapKey] = strings.Join(lines, "\n")
		return a.client.Update(ctx, configMap)
	})
}

func encodeAuditEntry(entry auditEntry) (string, error) {
	data, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
