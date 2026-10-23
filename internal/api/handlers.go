package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/reconcilyx/operator/internal/federated"
	"github.com/reconcilyx/operator/internal/notifiers"
)

// --- Common helpers ---------------------------------------------------------

type apiErr struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Details string `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string, details ...string) {
	d := ""
	if len(details) > 0 {
		d = strings.Join(details, "; ")
	}
	writeJSON(w, status, apiErr{Error: http.StatusText(status), Code: status, Details: msg + " " + d})
}

func readBody(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

func readOptionalBody(r *http.Request, out any) error {
	defer r.Body.Close()
	if r.ContentLength == 0 {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return nil
}

// actor returns a stable actor identifier string for audit / notifications, derived from request context.
func actor(r *http.Request) string {
	if a, ok := r.Context().Value(actorContextKey).(string); ok && a != "" {
		return a
	}
	if x := r.Header.Get("X-Actor"); x != "" {
		return x
	}
	return "api-user"
}

type ctxKey string

const actorContextKey ctxKey = "reconcilyx_actor"

// --- Proposals --------------------------------------------------------------

type proposalListItem struct {
	Name             string                                         `json:"name"`
	Namespace        string                                         `json:"namespace"`
	Cluster          string                                         `json:"cluster,omitempty"`
	WorkloadKind     string                                         `json:"workloadKind"`
	WorkloadName     string                                         `json:"workloadName"`
	Recommendation   optimizationv1alpha1.RecommendationType        `json:"recommendation"`
	ApprovalState    optimizationv1alpha1.ApprovalState             `json:"approvalState"`
	Phase            optimizationv1alpha1.OptimizationProposalPhase `json:"phase"`
	ExecutionState   optimizationv1alpha1.ExecutionState            `json:"executionState,omitempty"`
	RiskScore        int32                                          `json:"riskScore"`
	ConfidenceScore  int32                                          `json:"confidenceScore"`
	Rationale        string                                         `json:"rationale,omitempty"`
	EstimatedSavings string                                         `json:"estimatedSavings,omitempty"`
	CarbonSavedGrams int64                                          `json:"carbonSavedGrams,omitempty"`
	CarbonSavedHuman string                                         `json:"carbonSavedHuman,omitempty"`
	CPUBefore        string                                         `json:"cpuBefore,omitempty"`
	CPUAfter         string                                         `json:"cpuAfter,omitempty"`
	MemoryBefore     string                                         `json:"memoryBefore,omitempty"`
	MemoryAfter      string                                         `json:"memoryAfter,omitempty"`
	CreatedAt        time.Time                                      `json:"createdAt"`
	ExternalPRID     string                                         `json:"externalPRID,omitempty"`
}

func (s *Server) handleListProposals(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	phaseFilter := q.Get("phase")
	approvalFilter := q.Get("approvalState")
	clusterFilter := q.Get("cluster")
	nsFilter := q.Get("namespace")
	workloadFilter := q.Get("workload")

	var list optimizationv1alpha1.OptimizationProposalList
	if err := s.deps.Client.List(r.Context(), &list); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list proposals", err.Error())
		return
	}

	items := make([]proposalListItem, 0, len(list.Items))
	for _, p := range list.Items {
		if phaseFilter != "" && string(p.Status.Phase) != phaseFilter {
			continue
		}
		if approvalFilter != "" && string(p.Status.ApprovalState) != approvalFilter {
			continue
		}
		if clusterFilter != "" && p.Spec.TargetClusterName != clusterFilter {
			continue
		}
		if nsFilter != "" && p.Namespace != nsFilter {
			continue
		}
		if workloadFilter != "" && p.Spec.TargetWorkload.Name != workloadFilter {
			continue
		}
		item := proposalListItem{
			Name:            p.Name,
			Namespace:       p.Namespace,
			Cluster:         p.Spec.TargetClusterName,
			WorkloadKind:    p.Spec.TargetWorkload.Kind,
			WorkloadName:    p.Spec.TargetWorkload.Name,
			Recommendation:  p.Spec.Recommendation,
			ApprovalState:   p.Status.ApprovalState,
			Phase:           p.Status.Phase,
			ExecutionState:  p.Status.ExecutionState,
			RiskScore:       p.Spec.DecisionMetadata.RiskScore,
			ConfidenceScore: p.Spec.DecisionMetadata.ConfidenceScore,
			Rationale:       p.Spec.DecisionMetadata.Rationale,
			CreatedAt:       p.CreationTimestamp.Time,
			ExternalPRID:    p.Status.ExternalPRID,
		}
		if q := p.Spec.ObservedResources.CurrentResources.Requests.Cpu(); q != nil {
			item.CPUBefore = q.String()
		}
		if q := p.Spec.ProposedResources.Requests.Cpu(); q != nil {
			item.CPUAfter = q.String()
		}
		if q := p.Spec.ObservedResources.CurrentResources.Requests.Memory(); q != nil {
			item.MemoryBefore = q.String()
		}
		if q := p.Spec.ProposedResources.Requests.Memory(); q != nil {
			item.MemoryAfter = q.String()
		}
		if p.Spec.ExpectedImpact.EstimatedSavingsUSD != nil {
			f := p.Spec.ExpectedImpact.EstimatedSavingsUSD.AsApproximateFloat64()
			item.EstimatedSavings = fmt.Sprintf("%.2f", f)
		}
		if q := p.Spec.ExpectedImpact.CarbonSavedGramsCO2; q != nil {
			grams := q.MilliValue() / 1000
			item.CarbonSavedGrams = grams
			item.CarbonSavedHuman = formatCarbonGramsHuman(grams)
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) handleGetProposal(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "default"
	}
	var p optimizationv1alpha1.OptimizationProposal
	if err := s.deps.Client.Get(r.Context(), types.NamespacedName{Namespace: ns, Name: name}, &p); err != nil {
		if apierrors.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "Proposal not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to get proposal", err.Error())
		return
	}

	var evts corev1.EventList
	_ = s.deps.Client.List(r.Context(), &evts, client.InNamespace(ns))
	related := []corev1.Event{}
	for _, e := range evts.Items {
		if e.InvolvedObject.Kind == "OptimizationProposal" && e.InvolvedObject.Name == p.Name {
			related = append(related, e)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"proposal": p, "events": related})
}

// --- Approval / Rejection / Rollback state-mutation endpoints --------------

type actionRequest struct {
	Reason string `json:"reason"`
	Actor  string `json:"actor,omitempty"`
}

type actionResponse struct {
	Status        string `json:"status"`
	ApprovalState string `json:"approvalState,omitempty"`
	Phase         string `json:"phase,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Message       string `json:"message,omitempty"`
}

func (s *Server) handleApproveProposal(w http.ResponseWriter, r *http.Request) {
	var body actionRequest
	if err := readOptionalBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid body", err.Error())
		return
	}
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "default"
	}
	a := actor(r)
	if body.Actor != "" {
		a = body.Actor
	}
	if strings.TrimSpace(body.Reason) == "" {
		body.Reason = "Approved via API"
	}

	var p optimizationv1alpha1.OptimizationProposal
	key := types.NamespacedName{Namespace: ns, Name: r.PathValue("name")}
	if err := s.deps.Client.Get(r.Context(), key, &p); err != nil {
		writeError(w, http.StatusNotFound, "Proposal not found", err.Error())
		return
	}
	if p.Status.ApprovalState == optimizationv1alpha1.ApprovalApproved {
		writeJSON(w, http.StatusOK, actionResponse{Status: "no-op", ApprovalState: string(p.Status.ApprovalState), Message: "Proposal already approved"})
		return
	}
	if slices.Contains([]optimizationv1alpha1.OptimizationProposalPhase{
		optimizationv1alpha1.PhaseExecuting,
		optimizationv1alpha1.PhaseCompleted,
		optimizationv1alpha1.PhaseRollingBack,
		optimizationv1alpha1.PhaseRolledBack,
	}, p.Status.Phase) {
		writeError(w, http.StatusConflict, "Proposal is already executing/completed/rolled back; approval is immutable")
		return
	}

	patch := client.MergeFrom(p.DeepCopy())
	p.Status.ApprovalState = optimizationv1alpha1.ApprovalApproved
	p.Status.Reason = body.Reason
	if p.Status.Phase == "" || p.Status.Phase == optimizationv1alpha1.PhaseDetected || p.Status.Phase == optimizationv1alpha1.PhaseEvaluating {
		p.Status.Phase = optimizationv1alpha1.PhaseApproved
	}
	if err := s.deps.Client.Status().Patch(r.Context(), &p, patch); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to approve proposal", err.Error())
		return
	}

	_ = s.emitProposalEvent(r.Context(), &p, "Approved", fmt.Sprintf("Approved by %s: %s", a, body.Reason))
	s.deps.AuditLog.Append(notifiers.AuditEntry{
		Time:      time.Now().UTC(),
		Actor:     a,
		Action:    "approve",
		Proposal:  p.Name,
		Namespace: p.Namespace,
		Reason:    body.Reason,
	})
	_ = s.deps.NotifierDispatcher.Dispatch(r.Context(), notifiers.Event{
		Type: notifiers.EventProposalApproved, Proposal: &p, Actor: a, Reason: body.Reason,
	})

	writeJSON(w, http.StatusOK, actionResponse{
		Status: "ok", ApprovalState: string(p.Status.ApprovalState), Phase: string(p.Status.Phase), Reason: body.Reason,
	})
}

func (s *Server) handleRejectProposal(w http.ResponseWriter, r *http.Request) {
	var body actionRequest
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid body", err.Error())
		return
	}
	if strings.TrimSpace(body.Reason) == "" {
		writeError(w, http.StatusBadRequest, "Reason is required when rejecting a proposal")
		return
	}
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "default"
	}
	a := actor(r)
	if body.Actor != "" {
		a = body.Actor
	}
	var p optimizationv1alpha1.OptimizationProposal
	key := types.NamespacedName{Namespace: ns, Name: r.PathValue("name")}
	if err := s.deps.Client.Get(r.Context(), key, &p); err != nil {
		writeError(w, http.StatusNotFound, "Proposal not found", err.Error())
		return
	}
	if slices.Contains([]optimizationv1alpha1.OptimizationProposalPhase{
		optimizationv1alpha1.PhaseExecuting,
		optimizationv1alpha1.PhaseCompleted,
		optimizationv1alpha1.PhaseRollingBack,
		optimizationv1alpha1.PhaseRolledBack,
	}, p.Status.Phase) {
		writeError(w, http.StatusConflict, "Proposal is already executing/completed/rolled back; rejection is not allowed")
		return
	}
	patch := client.MergeFrom(p.DeepCopy())
	p.Status.ApprovalState = optimizationv1alpha1.ApprovalRejected
	p.Status.Phase = optimizationv1alpha1.PhaseRejected
	p.Status.Reason = body.Reason
	if err := s.deps.Client.Status().Patch(r.Context(), &p, patch); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to reject proposal", err.Error())
		return
	}

	_ = s.emitProposalEvent(r.Context(), &p, "Rejected", fmt.Sprintf("Rejected by %s: %s", a, body.Reason))
	s.deps.AuditLog.Append(notifiers.AuditEntry{
		Time:      time.Now().UTC(),
		Actor:     a,
		Action:    "reject",
		Proposal:  p.Name,
		Namespace: p.Namespace,
		Reason:    body.Reason,
	})
	_ = s.deps.NotifierDispatcher.Dispatch(r.Context(), notifiers.Event{
		Type: notifiers.EventProposalRejected, Proposal: &p, Actor: a, Reason: body.Reason,
	})

	writeJSON(w, http.StatusOK, actionResponse{
		Status: "ok", ApprovalState: string(p.Status.ApprovalState), Phase: string(p.Status.Phase), Reason: body.Reason,
	})
}

func (s *Server) handleRollbackProposal(w http.ResponseWriter, r *http.Request) {
	var body actionRequest
	if err := readOptionalBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid body", err.Error())
		return
	}
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "default"
	}
	a := actor(r)
	if body.Actor != "" {
		a = body.Actor
	}
	if strings.TrimSpace(body.Reason) == "" {
		body.Reason = "Rollback requested via API"
	}
	var p optimizationv1alpha1.OptimizationProposal
	key := types.NamespacedName{Namespace: ns, Name: r.PathValue("name")}
	if err := s.deps.Client.Get(r.Context(), key, &p); err != nil {
		writeError(w, http.StatusNotFound, "Proposal not found", err.Error())
		return
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
		writeError(w, http.StatusConflict, fmt.Sprintf("Rollback only allowed in phases %v, current %s", allowed, p.Status.Phase))
		return
	}
	if p.Status.ExecutionSnapshot == nil {
		writeError(w, http.StatusUnprocessableEntity, "Proposal has no execution snapshot; cannot rollback safely")
		return
	}

	patch := client.MergeFrom(p.DeepCopy())
	p.Status.Phase = optimizationv1alpha1.PhaseRollingBack
	p.Status.Reason = body.Reason
	if err := s.deps.Client.Status().Patch(r.Context(), &p, patch); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to transition proposal to rolling back", err.Error())
		return
	}

	_ = s.emitProposalEvent(r.Context(), &p, "RollbackRequested", fmt.Sprintf("Rollback requested by %s: %s", a, body.Reason))
	s.deps.AuditLog.Append(notifiers.AuditEntry{
		Time:      time.Now().UTC(),
		Actor:     a,
		Action:    "rollback",
		Proposal:  p.Name,
		Namespace: p.Namespace,
		Reason:    body.Reason,
	})
	_ = s.deps.NotifierDispatcher.Dispatch(r.Context(), notifiers.Event{
		Type: notifiers.EventProposalRolledBack, Proposal: &p, Actor: a, Reason: body.Reason,
	})

	writeJSON(w, http.StatusOK, actionResponse{Status: "ok", Phase: string(p.Status.Phase), Reason: body.Reason})
}

func (s *Server) emitProposalEvent(ctx context.Context, p *optimizationv1alpha1.OptimizationProposal, reason, message string) error {
	ref := p.Spec.TargetWorkload.DeepCopy()
	if ref.Name == "" {
		ref = &corev1.ObjectReference{
			Kind:       "OptimizationProposal",
			Namespace:  p.Namespace,
			Name:       p.Name,
			UID:        p.UID,
			APIVersion: p.APIVersion,
		}
	}
	token := make([]byte, 5)
	_, _ = rand.Read(token)
	evt := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s.%s.%s", p.Name, strings.ToLower(reason), hex.EncodeToString(token)),
			Namespace: p.Namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:            "OptimizationProposal",
			Namespace:       p.Namespace,
			Name:            p.Name,
			UID:             p.UID,
			APIVersion:      optimizationv1alpha1.GroupVersion.String(),
			ResourceVersion: p.ResourceVersion,
		},
		Reason:  reason,
		Message: message,
		Source:  corev1.EventSource{Component: "reconcilyx-api"},
		Type:    corev1.EventTypeNormal,
	}
	err := s.deps.Client.Create(ctx, evt)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// --- Policies ---------------------------------------------------------------

func (s *Server) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	var list optimizationv1alpha1.OptimizationPolicyList
	if err := s.deps.Client.List(r.Context(), &list); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list policies", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list.Items, "total": len(list.Items)})
}

func (s *Server) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "default"
	}
	var p optimizationv1alpha1.OptimizationPolicy
	if err := s.deps.Client.Get(r.Context(), types.NamespacedName{Namespace: ns, Name: r.PathValue("name")}, &p); err != nil {
		if apierrors.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "Policy not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to get policy", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	var p optimizationv1alpha1.OptimizationPolicy
	if err := readBody(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid policy body", err.Error())
		return
	}
	if p.Namespace == "" {
		p.Namespace = "default"
	}
	if p.Name == "" {
		writeError(w, http.StatusBadRequest, "Policy name is required")
		return
	}
	if err := s.deps.Client.Create(r.Context(), &p); err != nil {
		if apierrors.IsAlreadyExists(err) {
			writeError(w, http.StatusConflict, "Policy already exists", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to create policy", err.Error())
		return
	}
	a := actor(r)
	s.deps.AuditLog.Append(notifiers.AuditEntry{
		Time:      time.Now().UTC(),
		Actor:     a,
		Action:    "create_policy",
		Proposal:  p.Name,
		Namespace: p.Namespace,
	})
	writeJSON(w, http.StatusCreated, p)
}

// --- Savings, Forecast, Anomalies, Clusters --------------------------------

type savingsPoint struct {
	Date     string  `json:"date"`
	Cluster  string  `json:"cluster,omitempty"`
	USD      float64 `json:"usd"`
	GramsCO2 int64   `json:"gramsCO2,omitempty"`
}

func formatCarbonGramsHuman(grams int64) string {
	if grams <= 0 {
		return ""
	}
	const (
		kg    = 1000
		tonne = 1_000_000
	)
	switch {
	case grams >= tonne:
		return fmt.Sprintf("%.2f tonnes CO₂", float64(grams)/float64(tonne))
	case grams >= kg:
		return fmt.Sprintf("%.1f kg CO₂", float64(grams)/float64(kg))
	default:
		return fmt.Sprintf("%d g CO₂", grams)
	}
}

func (s *Server) handleSavingsMetrics(w http.ResponseWriter, r *http.Request) {
	var list optimizationv1alpha1.OptimizationProposalList
	if err := s.deps.Client.List(r.Context(), &list); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to compute savings", err.Error())
		return
	}
	total := 0.0
	totalGrams := int64(0)
	byCluster := map[string]float64{}
	byNs := map[string]float64{}
	byClusterGrams := map[string]int64{}
	byNsGrams := map[string]int64{}
	series := []savingsPoint{}
	for _, p := range list.Items {
		if !(p.Status.Phase == optimizationv1alpha1.PhaseCompleted ||
			p.Status.Phase == optimizationv1alpha1.PhaseStabilizing ||
			p.Status.ApprovalState == optimizationv1alpha1.ApprovalApproved) {
			continue
		}
		pt := savingsPoint{
			Date:    p.CreationTimestamp.Time.UTC().Format("2006-01-02"),
			Cluster: p.Spec.TargetClusterName,
		}
		if q := p.Spec.ExpectedImpact.EstimatedSavingsUSD; q != nil {
			f := q.AsApproximateFloat64()
			total += f
			byCluster[p.Spec.TargetClusterName] += f
			byNs[p.Namespace] += f
			pt.USD = f
		}
		if q := p.Spec.ExpectedImpact.CarbonSavedGramsCO2; q != nil {
			g := q.MilliValue() / 1000
			totalGrams += g
			byClusterGrams[p.Spec.TargetClusterName] += g
			byNsGrams[p.Namespace] += g
			pt.GramsCO2 = g
		}
		series = append(series, pt)
	}
	sort.Slice(series, func(i, j int) bool { return series[i].Date < series[j].Date })
	writeJSON(w, http.StatusOK, map[string]any{
		"totalUSD":          total,
		"totalGramsCO2":     totalGrams,
		"totalCarbonHuman":  formatCarbonGramsHuman(totalGrams),
		"byCluster":         byCluster,
		"byNs":              byNs,
		"byClusterGramsCO2": byClusterGrams,
		"byNsGramsCO2":      byNsGrams,
		"series":            series,
		"refreshed":         time.Now().UTC(),
	})
}

func (s *Server) handleForecast(w http.ResponseWriter, r *http.Request) {
	// This endpoint relies on the retriable Prometheus client + the Predictor.
	// For a light, always-available response we return sample buckets in the shape
	// the UI expects, populated from the proposal database when Prometheus is unreachable.
	workload := r.PathValue("workload")
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "default"
	}
	var list optimizationv1alpha1.OptimizationProposalList
	_ = s.deps.Client.List(r.Context(), &list, client.InNamespace(ns))

	type point struct {
		T         int64   `json:"t"`
		ActualCPU float64 `json:"actualCpu,omitempty"`
		PredCPU   float64 `json:"predCpu,omitempty"`
		ActualMem float64 `json:"actualMem,omitempty"`
		PredMem   float64 `json:"predMem,omitempty"`
	}
	now := time.Now().UTC()
	series := []point{}
	// Build a 24-point back-filled series
	for i := 24; i >= 0; i-- {
		t := now.Add(-time.Duration(i) * time.Hour)
		series = append(series, point{T: t.Unix()})
	}
	// Fill best-effort from known proposals
	for _, p := range list.Items {
		if p.Spec.TargetWorkload.Name != workload {
			continue
		}
		if q := p.Spec.ObservedResources.P95Usage.Cpu(); q != nil {
			for i := range series {
				if series[i].ActualCPU == 0 {
					series[i].ActualCPU = q.AsApproximateFloat64()
				}
			}
		}
		if q := p.Spec.ProposedResources.Requests.Cpu(); q != nil {
			for i := range series {
				if series[i].PredCPU == 0 {
					series[i].PredCPU = q.AsApproximateFloat64()
				}
			}
		}
		if q := p.Spec.ObservedResources.P95Usage.Memory(); q != nil {
			for i := range series {
				if series[i].ActualMem == 0 {
					series[i].ActualMem = q.AsApproximateFloat64()
				}
			}
		}
		if q := p.Spec.ProposedResources.Requests.Memory(); q != nil {
			for i := range series {
				if series[i].PredMem == 0 {
					series[i].PredMem = q.AsApproximateFloat64()
				}
			}
		}
		break
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workload":  workload,
		"namespace": ns,
		"series":    series,
	})
}

func (s *Server) handleAnomalies(w http.ResponseWriter, r *http.Request) {
	// Surface anomalies per workload: read back proposals and return DecisionMetadata risk spikes.
	var list optimizationv1alpha1.OptimizationProposalList
	if err := s.deps.Client.List(r.Context(), &list); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to collect anomalies", err.Error())
		return
	}
	type anomalyRow struct {
		Time      time.Time `json:"time"`
		Workload  string    `json:"workload"`
		Namespace string    `json:"namespace"`
		Metric    string    `json:"metric"`
		RiskScore int32     `json:"riskScore"`
		Severity  string    `json:"severity"`
		Proposal  string    `json:"proposal,omitempty"`
	}
	rows := []anomalyRow{}
	for _, p := range list.Items {
		score := p.Spec.DecisionMetadata.RiskScore
		if score == 0 {
			continue
		}
		sev := "low"
		switch {
		case score >= 70:
			sev = "high"
		case score >= 40:
			sev = "medium"
		}
		rows = append(rows, anomalyRow{
			Time:      p.CreationTimestamp.Time,
			Workload:  p.Spec.TargetWorkload.Name,
			Namespace: p.Namespace,
			Metric:    "cpu-memory-composite",
			RiskScore: score,
			Severity:  sev,
			Proposal:  p.Name,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Time.After(rows[j].Time) })
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil && n > 0 && n < len(rows) {
			rows = rows[:n]
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handleListClusters(w http.ResponseWriter, r *http.Request) {
	var list optimizationv1alpha1.ClusterRegistrationList
	_ = s.deps.Client.List(r.Context(), &list)
	items := []map[string]any{
		{"name": "hub", "type": "hub", "status": "ready"},
	}
	for _, c := range list.Items {
		items = append(items, map[string]any{
			"name":          c.Name,
			"type":          "spoke",
			"clusterName":   c.Spec.ClusterName,
			"apiEndpoint":   c.Spec.APIEndpoint,
			"status":        c.Status.Status,
			"lastHeartbeat": c.Status.LastHeartbeat,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// --- What-If simulator ------------------------------------------------------

type whatIfRequest struct {
	Namespace      string `json:"namespace"`
	WorkloadName   string `json:"workloadName"`
	WorkloadKind   string `json:"workloadKind"`
	CurrentCPU     string `json:"currentCPU"`
	CurrentMemory  string `json:"currentMemory"`
	ProposedCPU    string `json:"proposedCPU"`
	ProposedMemory string `json:"proposedMemory"`
	P95CPU         string `json:"p95CPU,omitempty"`
	P95Memory      string `json:"p95Memory,omitempty"`
}

type whatIfResponse struct {
	Allowed            bool   `json:"allowed"`
	ConfidenceScore    int32  `json:"confidenceScore"`
	RiskScore          int32  `json:"riskScore"`
	CPUDelta           string `json:"cpuDelta"`
	MemoryDelta        string `json:"memoryDelta"`
	SavingsUSDPerMonth string `json:"savingsUSDPerMonth"`
	Rationale          string `json:"rationale"`
}

func (s *Server) handleWhatIf(w http.ResponseWriter, r *http.Request) {
	var body whatIfRequest
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid body", err.Error())
		return
	}
	cpu1, err := resource.ParseQuantity(body.CurrentCPU)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid currentCPU", err.Error())
		return
	}
	mem1, err := resource.ParseQuantity(body.CurrentMemory)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid currentMemory", err.Error())
		return
	}
	cpu2, err := resource.ParseQuantity(body.ProposedCPU)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid proposedCPU", err.Error())
		return
	}
	mem2, err := resource.ParseQuantity(body.ProposedMemory)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid proposedMemory", err.Error())
		return
	}
	deltaCPU := cpu1.DeepCopy()
	deltaCPU.Sub(cpu2)
	deltaMem := mem1.DeepCopy()
	deltaMem.Sub(mem2)

	risk := int32(15)
	confidence := int32(78)
	rationale := "Proposal within default safety constraints. What-If recommends approve for cost."

	// If CPU was cut by more than 60% or memory more than 50%, bump risk and flag
	cpuCutPct := 0.0
	if cpu1.MilliValue() > 0 {
		cpuCutPct = float64(cpu1.MilliValue()-cpu2.MilliValue()) / float64(cpu1.MilliValue()) * 100
	}
	memCutPct := 0.0
	if mem1.Value() > 0 {
		memCutPct = float64(mem1.Value()-mem2.Value()) / float64(mem1.Value()) * 100
	}
	if cpuCutPct > 60 || memCutPct > 50 {
		risk = 70
		confidence = 52
		rationale = fmt.Sprintf("Aggressive cut (CPU %.1f%% / Mem %.1f%%). Risk elevated — consider phasing this change.", cpuCutPct, memCutPct)
	}

	// Rough pricing: assume $25 / vCPU-month + $3 / 8Gi-month for quick cost estimate
	savingsCPU := (float64(cpu1.MilliValue()-cpu2.MilliValue()) / 1000.0) * 25.0
	savingsMem := (float64(mem1.Value()-mem2.Value()) / (8 * 1024 * 1024 * 1024)) * 3.0
	savingsTotal := savingsCPU + savingsMem
	if savingsTotal < 0 {
		savingsTotal = 0
		rationale += " Note: proposed configuration increases spend."
	}

	writeJSON(w, http.StatusOK, whatIfResponse{
		Allowed:            risk < 70,
		ConfidenceScore:    confidence,
		RiskScore:          risk,
		CPUDelta:           deltaCPU.String(),
		MemoryDelta:        deltaMem.String(),
		SavingsUSDPerMonth: fmt.Sprintf("%.2f", savingsTotal),
		Rationale:          rationale,
	})
}

// --- Audit tail -------------------------------------------------------------

func (s *Server) handleAuditTail(w http.ResponseWriter, r *http.Request) {
	proposalFilter := r.URL.Query().Get("proposal")
	limit := 500
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	entries := s.deps.AuditLog.Tail(limit, proposalFilter)
	writeJSON(w, http.StatusOK, map[string]any{"items": entries, "total": len(entries)})
}

// --- Static / SPA -----------------------------------------------------------

func (s *Server) handleStaticOrSPA(w http.ResponseWriter, r *http.Request) {
	if !s.deps.UIEnabled {
		http.Error(w, "UI not embedded. Run `make build-ui` and rebuild the controller binary.", http.StatusNotFound)
		return
	}
	path := r.URL.Path
	if path == "/" {
		path = "index.html"
	}
	assetPath := strings.TrimPrefix(path, "/")
	f, err := s.deps.UIDist.Open(assetPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			idx, idxErr := s.deps.UIDist.Open("index.html")
			if idxErr != nil {
				writeError(w, http.StatusNotFound, "File not found and no index.html fallback")
				return
			}
			defer idx.Close()
			buf, readErr := io.ReadAll(idx)
			if readErr != nil {
				writeError(w, http.StatusInternalServerError, "Cannot read index.html", readErr.Error())
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(buf)
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to open static asset", err.Error())
		return
	}
	defer f.Close()
	buf, readErr := io.ReadAll(f)
	if readErr != nil {
		writeError(w, http.StatusInternalServerError, "Cannot read asset", readErr.Error())
		return
	}
	stat, _ := f.Stat()
	ct := mimeTypeFromName(assetPath)
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if strings.HasPrefix(assetPath, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeContent(w, r, assetPath, stat.ModTime(), bytes.NewReader(buf))
}

func mimeTypeFromName(p string) string {
	switch {
	case strings.HasSuffix(p, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(p, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(p, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(p, ".json"):
		return "application/json"
	case strings.HasSuffix(p, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(p, ".png"):
		return "image/png"
	case strings.HasSuffix(p, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(p, ".woff2"):
		return "font/woff2"
	}
	return ""
}

// --- Federated intelligence endpoints ---------------------------------------

func telemetryDeltasReceivedAdd(cluster string, n int) {
	if n <= 0 {
		return
	}
	for i := 0; i < n; i++ {
		telemetryDeltasReceivedOne(cluster)
	}
}

var telemetryDeltasReceivedOne = func(cluster string) {}
var telemetryDeltasSentOne = func(cluster string) {}
var telemetryDeltasBatchSentOne = func(cluster, outcome string) {}
var telemetryBootstrapOutcomeOne = func(cluster, outcome string) {}

func (s *Server) handleFederatedDeltas(w http.ResponseWriter, r *http.Request) {
	if s.fedAggregator == nil {
		writeError(w, http.StatusServiceUnavailable, "Federated aggregator is not initialized")
		return
	}
	var body federated.DeltaBatch
	if err := readBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid delta batch body", err.Error())
		return
	}
	if len(body.Deltas) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "received": 0})
		return
	}
	cluster := body.ClusterName
	if cluster == "" {
		cluster = "unknown"
	}
	normalized := make([]federated.QTableDelta, 0, len(body.Deltas))
	for _, d := range body.Deltas {
		if d.ClusterName == "" {
			d.ClusterName = cluster
		}
		normalized = append(normalized, d)
	}
	s.fedAggregator.AddDeltas(normalized)
	telemetryDeltasReceivedAdd(cluster, len(normalized))
	if n := s.fedAggregator.PendingDeltaCount(); n > 0 && n%500 == 0 {
		snap := s.fedAggregator.CompactAndSnapshot()
		if s.deps.Client != nil && snap != nil {
			_ = federated.SaveGlobalModel(r.Context(), s.deps.Client, s.fedNamespace, s.fedModelName, snap)
		}
		syncGauges(s.fedAggregator)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "accepted",
		"cluster":  cluster,
		"received": len(normalized),
		"pending":  s.fedAggregator.PendingDeltaCount(),
	})
}

func (s *Server) handleFederatedGlobalModel(w http.ResponseWriter, r *http.Request) {
	if s.fedAggregator == nil {
		writeError(w, http.StatusServiceUnavailable, "Federated aggregator is not initialized")
		return
	}
	channel := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("channel")))
	cluster := strings.TrimSpace(r.URL.Query().Get("cluster"))

	var snap *federated.GlobalQTable
	if channel == federated.ModelTagStable || channel == federated.ModelTagCanary {
		var version string
		snap, version = s.modelRegistry.GetModelForCluster(cluster, channel == federated.ModelTagCanary)
		if version != "" {
			actualChannel := federated.ModelTagStable
			if channel == federated.ModelTagCanary && s.modelRegistry.CanaryVersion() != "" {
				actualChannel = federated.ModelTagCanary
			}
			w.Header().Set("X-Reconcilyx-Model-Version", version)
			w.Header().Set("X-Reconcilyx-Model-Channel", actualChannel)
		}
	}
	if snap == nil {
		// No release has been published for this channel yet; serve the live
		// aggregated snapshot so pre-registry spokes keep bootstrapping.
		snap = s.fedAggregator.CurrentSnapshot()
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleFederatedReleases(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"stable": s.modelRegistry.StableVersion(),
		"canary": s.modelRegistry.CanaryVersion(),
	})
}

func (s *Server) handleFederatedCanaryRollback(w http.ResponseWriter, r *http.Request) {
	reason := strings.TrimSpace(r.URL.Query().Get("reason"))
	if reason == "" {
		reason = "operator rollback"
	}
	if err := s.modelRegistry.RollbackCanary(reason); err != nil {
		writeError(w, http.StatusConflict, "Cannot roll back canary model", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "canary-demoted",
		"stable": s.modelRegistry.StableVersion(),
		"canary": s.modelRegistry.CanaryVersion(),
	})
}

var _ = context.Background
