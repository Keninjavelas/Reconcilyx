package gitops

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/reconcilyx/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const autoMergeMarker = AutoMergeMarker

// PROptions carries the policy-controlled GitOps settings for a proposal PR.
type PROptions struct {
	ManifestsPath string
	BaseBranch    string
	AutoMerge     bool
}

// PRResult describes the created pull/merge request.
type PRResult struct {
	ID         string
	FilePath   string
	AutoMerged bool
}

// PRManager manages pull requests for optimization proposals
type PRManager struct {
	provider GitProvider
}

// NewPRManager creates a new PR manager with the given Git provider
func NewPRManager(provider GitProvider) *PRManager {
	return &PRManager{
		provider: provider,
	}
}

// CreateProposalPR patches the workload's existing manifest and opens a pull
// request against the configured repository. The manifest patch is required:
// a partial document committed into an auto-synced directory would prune live
// Deployment fields, so a missing manifest is an error rather than a fallback.
func (m *PRManager) CreateProposalPR(
	ctx context.Context,
	proposal *v1alpha1.OptimizationProposal,
	currentResources map[string]corev1.ResourceRequirements,
	opts PROptions,
) (*PRResult, error) {
	if m.provider == nil {
		return nil, fmt.Errorf("gitops: provider is nil")
	}
	if proposal == nil {
		return nil, fmt.Errorf("gitops: proposal is nil")
	}

	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	manifest, err := ResolveManifest(
		ctx,
		m.provider,
		proposal.Spec.TargetWorkload,
		proposal.Spec.ProposedResources,
		proposal.Spec.ProposedResourcesByContainer,
		currentResources,
		opts.ManifestsPath,
		baseBranch,
		proposal.Name,
	)
	if err != nil {
		return nil, err
	}

	autoMergeEligible := opts.AutoMerge && m.shouldAutoMerge(proposal)

	pr := &PullRequest{
		Title:       fmt.Sprintf("Optimize resources for %s/%s", proposal.Spec.TargetWorkload.Namespace, proposal.Spec.TargetWorkload.Name),
		Description: m.buildPRDescription(proposal, manifest.Path, autoMergeEligible),
		HeadBranch:  fmt.Sprintf("reconcilyx/%s-%s", proposal.Spec.TargetWorkload.Namespace, proposal.Spec.TargetWorkload.Name),
		BaseBranch:  baseBranch,
		FilePath:    manifest.Path,
		Diff:        string(manifest.Content),
	}

	prID, err := m.provider.CreatePullRequest(ctx, pr)
	if err != nil {
		return nil, err
	}

	costComment := &Comment{Body: m.buildCostComment(proposal)}
	if err := m.provider.AddComment(ctx, prID, costComment); err != nil {
		slog.Warn("Could not add cost comment to GitOps PR", "pr", prID, "error", err)
	}

	result := &PRResult{ID: prID, FilePath: manifest.Path}
	if autoMergeEligible {
		conditions := &MergeConditions{
			MinApprovalCount: 0,
			MaxRiskScore:     20,
			MinConfidence:    80,
		}
		if err := m.provider.AutoMerge(ctx, prID, conditions); err != nil {
			slog.Warn("Auto-merge request failed; PR remains open", "pr", prID, "error", err)
		} else {
			result.AutoMerged = true
		}
	}

	return result, nil
}

func formatCarbonImpact(q *resource.Quantity) string {
	if q == nil || q.IsZero() {
		return ""
	}
	grams := float64(q.Value())
	if grams >= 1_000_000 {
		return fmt.Sprintf("%.2f tonnes CO₂ / month", grams/1_000_000.0)
	}
	if grams >= 1_000 {
		return fmt.Sprintf("%.1f kg CO₂ / month", grams/1_000.0)
	}
	return fmt.Sprintf("%d g CO₂ / month", int64(grams))
}

func (m *PRManager) buildPRDescription(
	proposal *v1alpha1.OptimizationProposal,
	manifestPath string,
	autoMerge bool,
) string {
	carbonLine := ""
	if line := formatCarbonImpact(proposal.Spec.ExpectedImpact.CarbonSavedGramsCO2); line != "" {
		carbonLine = fmt.Sprintf("- **Estimated carbon reduction:** %s\n", line)
	}
	marker := ""
	if autoMerge {
		marker = "\n" + autoMergeMarker + "\n"
	}
	return fmt.Sprintf(`# Reconcilyx Optimization Proposal

This pull request optimizes the resources for %s/%s by patching `+"`%s`"+`.

## Decision Details
- **Recommendation Type**: %s
- **Confidence Score**: %d
- **Risk Score**: %d
- **Rationale**: %s

## Expected Impact
- Estimated monthly savings: %s
%s%s`,
		proposal.Spec.TargetWorkload.Namespace,
		proposal.Spec.TargetWorkload.Name,
		manifestPath,
		proposal.Spec.Recommendation,
		proposal.Spec.DecisionMetadata.ConfidenceScore,
		proposal.Spec.DecisionMetadata.RiskScore,
		proposal.Spec.DecisionMetadata.Rationale,
		proposal.Spec.ExpectedImpact.EstimatedSavingsUSD.String(),
		carbonLine,
		marker,
	)
}

func (m *PRManager) buildCostComment(proposal *v1alpha1.OptimizationProposal) string {
	carbonRow := ""
	if line := formatCarbonImpact(proposal.Spec.ExpectedImpact.CarbonSavedGramsCO2); line != "" {
		carbonRow = fmt.Sprintf("| Estimated Monthly Carbon Reduction | %s |\n", line)
	}
	return fmt.Sprintf(`## Cost + Carbon Impact Dashboard

| Metric | Value |
|--------|-------|
| Estimated Monthly Savings | %s |
%s| Confidence Score | %d |
| Risk Score | %d |
`,
		proposal.Spec.ExpectedImpact.EstimatedSavingsUSD.String(),
		carbonRow,
		proposal.Spec.DecisionMetadata.ConfidenceScore,
		proposal.Spec.DecisionMetadata.RiskScore,
	)
}

func (m *PRManager) shouldAutoMerge(proposal *v1alpha1.OptimizationProposal) bool {
	return proposal.Spec.DecisionMetadata.ConfidenceScore >= 80 &&
		proposal.Spec.DecisionMetadata.RiskScore <= 20
}
