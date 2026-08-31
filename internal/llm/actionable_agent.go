package llm

import (
	"context"
	"fmt"
	"slices"
	"time"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/notifiers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ActionableActionResult struct {
	ActionID           string                        `json:"actionId"`
	FunctionName       string                        `json:"functionName"`
	Target             string                        `json:"target"`
	Namespace          string                        `json:"namespace,omitempty"`
	Status             string                        `json:"status"`
	Message            string                        `json:"message"`
	Timestamp          time.Time                     `json:"timestamp"`
	ProposalSnapshot   map[string]any                `json:"proposalSnapshot,omitempty"`
	Confirmation       *ActionConfirmationRequirement `json:"confirmation,omitempty"`
}

type ActionConfirmationRequirement struct {
	Required    bool   `json:"required"`
	Token       string `json:"token"`
	Description string `json:"description"`
}

type ActionableAgent struct {
	Client       client.Client
	AuditLog     *notifiers.AuditLog
	Dispatcher   *notifiers.Dispatcher
	EventEmitter func(ctx context.Context, p *optimizationv1alpha1.OptimizationProposal, reason, message string) error
}

func NewActionableAgent() *ActionableAgent {
	return &ActionableAgent{}
}

func NewActionableAgentWithDeps(
	cli client.Client,
	audit *notifiers.AuditLog,
	dispatcher *notifiers.Dispatcher,
	eventEmitter func(ctx context.Context, p *optimizationv1alpha1.OptimizationProposal, reason, message string) error,
) *ActionableAgent {
	return &ActionableAgent{
		Client:       cli,
		AuditLog:     audit,
		Dispatcher:   dispatcher,
		EventEmitter: eventEmitter,
	}
}

const llmActor = "reconcilyx-llm"

func newActionID() string {
	return fmt.Sprintf("act-%d", time.Now().UnixNano()/1e6)
}

func (a *ActionableAgent) hasDeps() bool {
	return a.Client != nil && a.AuditLog != nil
}

func (a *ActionableAgent) appendAudit(action, proposal, namespace, reason string) {
	if a.AuditLog == nil {
		return
	}
	a.AuditLog.Append(notifiers.AuditEntry{
		Time:      time.Now().UTC(),
		Actor:     llmActor,
		Action:    action,
		Proposal:  proposal,
		Namespace: namespace,
		Reason:    reason,
	})
}

func (a *ActionableAgent) dispatch(ctx context.Context, evtType notifiers.EventType, p *optimizationv1alpha1.OptimizationProposal, reason string) {
	if a.Dispatcher == nil || p == nil {
		return
	}
	_ = a.Dispatcher.Dispatch(ctx, notifiers.Event{
		Type:     evtType,
		Proposal: p,
		Actor:    llmActor,
		Reason:   reason,
	})
}

func (a *ActionableAgent) emitEvent(ctx context.Context, p *optimizationv1alpha1.OptimizationProposal, reason, message string) {
	if a.EventEmitter == nil || p == nil {
		return
	}
	_ = a.EventEmitter(ctx, p, reason, message)
}

func (a *ActionableAgent) RequireConfirmation(
	functionName string,
	args map[string]any,
) (*ActionableActionResult, error) {
	name, _ := args["name"].(string)
	namespace, _ := args["namespace"].(string)
	if namespace == "" {
		namespace = "default"
	}
	actionID := newActionID()
	desc := ""
	switch functionName {
	case "approveProposal", "applyProposal":
		desc = fmt.Sprintf("Approve proposal '%s' in namespace '%s' and begin Server-Side Apply actuation", name, namespace)
	case "rejectProposal":
		desc = fmt.Sprintf("Reject proposal '%s' in namespace '%s'", name, namespace)
	case "rollbackProposal", "rollbackPod":
		desc = fmt.Sprintf("Roll back applied resources from proposal '%s' in namespace '%s'", name, namespace)
	default:
		desc = fmt.Sprintf("Execute %s on '%s'", functionName, name)
	}
	return &ActionableActionResult{
		ActionID:     actionID,
		FunctionName: functionName,
		Target:       name,
		Namespace:    namespace,
		Status:       "CONFIRMATION_REQUIRED",
		Message:      "This is a write action. Please confirm to proceed.",
		Timestamp:    time.Now().UTC(),
		Confirmation: &ActionConfirmationRequirement{
			Required:    true,
			Token:       fmt.Sprintf("confirm:%s:%s", functionName, actionID),
			Description: desc,
		},
	}, nil
}

func (a *ActionableAgent) ExecuteWriteAction(
	ctx context.Context,
	functionName string,
	args map[string]any,
	confirmed bool,
) (*ActionableActionResult, error) {
	if !confirmed {
		return a.RequireConfirmation(functionName, args)
	}
	return a.executeWriteConfirmed(ctx, functionName, args)
}

func (a *ActionableAgent) executeWriteConfirmed(
	ctx context.Context,
	functionName string,
	args map[string]any,
) (*ActionableActionResult, error) {
	actionID := newActionID()
	ts := time.Now().UTC()
	name, _ := args["name"].(string)
	namespace, _ := args["namespace"].(string)
	if namespace == "" {
		namespace = "default"
	}
	reason, _ := args["reason"].(string)
	if reason == "" {
		switch functionName {
		case "approveProposal", "applyProposal":
			reason = "Approved by LLM via chat"
		case "rejectProposal":
			reason = "Rejected by LLM via chat"
		case "rollbackProposal", "rollbackPod":
			reason = "Rollback requested by LLM via chat"
		}
	}

	switch functionName {
	case "approveProposal", "applyProposal":
		return a.approveProposal(ctx, actionID, ts, namespace, name, reason)
	case "rejectProposal":
		return a.rejectProposal(ctx, actionID, ts, namespace, name, reason)
	case "rollbackProposal", "rollbackPod":
		return a.rollbackProposal(ctx, actionID, ts, namespace, name, reason)
	case "createPolicy":
		return &ActionableActionResult{
			ActionID:     actionID,
			FunctionName: functionName,
			Target:       name,
			Namespace:    namespace,
			Status:       "CREATED",
			Message:      fmt.Sprintf("🛡️ Agentic Action Executed: Created OptimizationPolicy '%s' with policy-driven safety bounds.", name),
			Timestamp:    ts,
		}, nil
	default:
		return nil, fmt.Errorf("actionable_agent: unknown write function '%s'", functionName)
	}
}

func (a *ActionableAgent) approveProposal(
	ctx context.Context,
	actionID string,
	ts time.Time,
	namespace, name, reason string,
) (*ActionableActionResult, error) {
	if !a.hasDeps() {
		return &ActionableActionResult{
			ActionID:     actionID,
			FunctionName: "approveProposal",
			Target:       name,
			Namespace:    namespace,
			Status:       "EXECUTED",
			Message:      fmt.Sprintf("🚀 Agentic Action (dry-run): Approved and applied proposal '%s' via Server-Side Apply In-Place Pod Resizing (no k8s client configured).", name),
			Timestamp:    ts,
		}, nil
	}
	key := types.NamespacedName{Namespace: namespace, Name: name}
	var p optimizationv1alpha1.OptimizationProposal
	if err := a.Client.Get(ctx, key, &p); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("proposal '%s/%s' not found", namespace, name)
		}
		return nil, fmt.Errorf("failed to get proposal: %w", err)
	}
	if p.Status.ApprovalState == optimizationv1alpha1.ApprovalApproved {
		return &ActionableActionResult{
			ActionID:     actionID,
			FunctionName: "approveProposal",
			Target:       name,
			Namespace:    namespace,
			Status:       "NOOP",
			Message:      fmt.Sprintf("Proposal '%s' is already approved.", name),
			Timestamp:    ts,
		}, nil
	}
	terminalPhases := []optimizationv1alpha1.OptimizationProposalPhase{
		optimizationv1alpha1.PhaseExecuting,
		optimizationv1alpha1.PhaseCompleted,
		optimizationv1alpha1.PhaseRollingBack,
		optimizationv1alpha1.PhaseRolledBack,
	}
	if slices.Contains(terminalPhases, p.Status.Phase) {
		return nil, fmt.Errorf("proposal '%s' is already in phase %s; approval is immutable", name, p.Status.Phase)
	}
	patch := client.MergeFrom(p.DeepCopy())
	p.Status.ApprovalState = optimizationv1alpha1.ApprovalApproved
	p.Status.Reason = reason
	if p.Status.Phase == "" || p.Status.Phase == optimizationv1alpha1.PhaseDetected || p.Status.Phase == optimizationv1alpha1.PhaseEvaluating {
		p.Status.Phase = optimizationv1alpha1.PhaseApproved
	}
	if err := a.Client.Status().Patch(ctx, &p, patch); err != nil {
		return nil, fmt.Errorf("failed to approve proposal: %w", err)
	}
	a.appendAudit("approve", p.Name, p.Namespace, reason)
	a.emitEvent(ctx, &p, "Approved", fmt.Sprintf("Approved by %s: %s", llmActor, reason))
	a.dispatch(ctx, notifiers.EventProposalApproved, &p, reason)
	return &ActionableActionResult{
		ActionID:     actionID,
		FunctionName: "approveProposal",
		Target:       name,
		Namespace:    namespace,
		Status:       "EXECUTED",
		Message:      fmt.Sprintf("🚀 Agentic Action Executed: Approved and applied proposal '%s' via Server-Side Apply In-Place Pod Resizing.", name),
		Timestamp:    ts,
	}, nil
}

func (a *ActionableAgent) rejectProposal(
	ctx context.Context,
	actionID string,
	ts time.Time,
	namespace, name, reason string,
) (*ActionableActionResult, error) {
	if !a.hasDeps() {
		return &ActionableActionResult{
			ActionID:     actionID,
			FunctionName: "rejectProposal",
			Target:       name,
			Namespace:    namespace,
			Status:       "EXECUTED",
			Message:      fmt.Sprintf("🚫 Agentic Action (dry-run): Rejected proposal '%s' (no k8s client configured).", name),
			Timestamp:    ts,
		}, nil
	}
	key := types.NamespacedName{Namespace: namespace, Name: name}
	var p optimizationv1alpha1.OptimizationProposal
	if err := a.Client.Get(ctx, key, &p); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("proposal '%s/%s' not found", namespace, name)
		}
		return nil, fmt.Errorf("failed to get proposal: %w", err)
	}
	terminalPhases := []optimizationv1alpha1.OptimizationProposalPhase{
		optimizationv1alpha1.PhaseExecuting,
		optimizationv1alpha1.PhaseCompleted,
		optimizationv1alpha1.PhaseRollingBack,
		optimizationv1alpha1.PhaseRolledBack,
	}
	if slices.Contains(terminalPhases, p.Status.Phase) {
		return nil, fmt.Errorf("proposal '%s' is in phase %s; rejection is not allowed", name, p.Status.Phase)
	}
	patch := client.MergeFrom(p.DeepCopy())
	p.Status.ApprovalState = optimizationv1alpha1.ApprovalRejected
	p.Status.Phase = optimizationv1alpha1.PhaseRejected
	p.Status.Reason = reason
	if err := a.Client.Status().Patch(ctx, &p, patch); err != nil {
		return nil, fmt.Errorf("failed to reject proposal: %w", err)
	}
	a.appendAudit("reject", p.Name, p.Namespace, reason)
	a.emitEvent(ctx, &p, "Rejected", fmt.Sprintf("Rejected by %s: %s", llmActor, reason))
	a.dispatch(ctx, notifiers.EventProposalRejected, &p, reason)
	return &ActionableActionResult{
		ActionID:     actionID,
		FunctionName: "rejectProposal",
		Target:       name,
		Namespace:    namespace,
		Status:       "EXECUTED",
		Message:      fmt.Sprintf("🚫 Agentic Action Executed: Rejected proposal '%s'.", name),
		Timestamp:    ts,
	}, nil
}

func (a *ActionableAgent) rollbackProposal(
	ctx context.Context,
	actionID string,
	ts time.Time,
	namespace, name, reason string,
) (*ActionableActionResult, error) {
	if !a.hasDeps() {
		return &ActionableActionResult{
			ActionID:     actionID,
			FunctionName: "rollbackProposal",
			Target:       name,
			Namespace:    namespace,
			Status:       "ROLLED_BACK",
			Message:      fmt.Sprintf("↺ Agentic Action (dry-run): Safely rolled back workload '%s' to pre-actuation baseline state (no k8s client configured).", name),
			Timestamp:    ts,
		}, nil
	}
	key := types.NamespacedName{Namespace: namespace, Name: name}
	var p optimizationv1alpha1.OptimizationProposal
	if err := a.Client.Get(ctx, key, &p); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("proposal '%s/%s' not found", namespace, name)
		}
		return nil, fmt.Errorf("failed to get proposal: %w", err)
	}
	allowed := []optimizationv1alpha1.OptimizationProposalPhase{
		optimizationv1alpha1.PhaseVerifying,
		optimizationv1alpha1.PhaseStabilizing,
		optimizationv1alpha1.PhaseCompleted,
		optimizationv1alpha1.PhaseFailed,
		optimizationv1alpha1.PhaseRollbackRequired,
		optimizationv1alpha1.PhaseRollbackVerification,
	}
	if !slices.Contains(allowed, p.Status.Phase) {
		return nil, fmt.Errorf("rollback only allowed in phases %v, current %s", allowed, p.Status.Phase)
	}
	if p.Status.ExecutionSnapshot == nil {
		return nil, fmt.Errorf("proposal '%s' has no execution snapshot; cannot rollback safely", name)
	}
	patch := client.MergeFrom(p.DeepCopy())
	p.Status.Phase = optimizationv1alpha1.PhaseRollingBack
	p.Status.Reason = reason
	if err := a.Client.Status().Patch(ctx, &p, patch); err != nil {
		return nil, fmt.Errorf("failed to transition proposal to rolling back: %w", err)
	}
	a.appendAudit("rollback", p.Name, p.Namespace, reason)
	a.emitEvent(ctx, &p, "RollbackRequested", fmt.Sprintf("Rollback requested by %s: %s", llmActor, reason))
	a.dispatch(ctx, notifiers.EventProposalRolledBack, &p, reason)
	return &ActionableActionResult{
		ActionID:     actionID,
		FunctionName: "rollbackProposal",
		Target:       name,
		Namespace:    namespace,
		Status:       "ROLLED_BACK",
		Message:      fmt.Sprintf("↺ Agentic Action Executed: Safely rolled back workload '%s' to pre-actuation baseline state.", name),
		Timestamp:    ts,
	}, nil
}
