package gitops

import (
	"context"
	"fmt"

	"github.com/reconcilyx/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

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

// CreateProposalPR creates a pull request for an OptimizationProposal
func (m *PRManager) CreateProposalPR(
	ctx context.Context,
	proposal *v1alpha1.OptimizationProposal,
	currentResources map[string]corev1.ResourceRequirements,
) (string, error) {
	prTitle := fmt.Sprintf("Optimize resources for %s/%s", proposal.Spec.TargetWorkload.Namespace, proposal.Spec.TargetWorkload.Name)
	prDescription := m.buildPRDescription(proposal, currentResources)
	prDiff := m.buildDiff(proposal, currentResources)

	pr := &PullRequest{
		Title:       prTitle,
		Description: prDescription,
		HeadBranch:  fmt.Sprintf("reconcilyx-optimize-%s", proposal.Name),
		BaseBranch:  "main",
		Diff:        prDiff,
	}

	prID, err := m.provider.CreatePullRequest(ctx, pr)
	if err != nil {
		return "", err
	}

	costComment := &Comment{
		Body: m.buildCostComment(proposal),
	}
	if err := m.provider.AddComment(ctx, prID, costComment); err != nil {
	}

	if m.shouldAutoMerge(proposal) {
		if err := m.provider.AutoMerge(ctx, prID, &MergeConditions{
			MinApprovalCount: 0,
			MaxRiskScore:     20,
			MinConfidence:    80,
		}); err != nil {
		}
	}

	return prID, nil
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
	currentResources map[string]corev1.ResourceRequirements,
) string {
	carbonLine := ""
	if line := formatCarbonImpact(proposal.Spec.ExpectedImpact.CarbonSavedGramsCO2); line != "" {
		carbonLine = fmt.Sprintf("- **Estimated carbon reduction:** %s\n", line)
	}
	return fmt.Sprintf(`# Reconcilyx Optimization Proposal

This pull request optimizes the resources for %s/%s.

## Decision Details
- **Recommendation Type**: %s
- **Confidence Score**: %d
- **Risk Score**: %d
- **Rationale**: %s

## Expected Impact
- Estimated monthly savings: %s
%s`,
		proposal.Spec.TargetWorkload.Namespace,
		proposal.Spec.TargetWorkload.Name,
		proposal.Spec.Recommendation,
		proposal.Spec.DecisionMetadata.ConfidenceScore,
		proposal.Spec.DecisionMetadata.RiskScore,
		proposal.Spec.DecisionMetadata.Rationale,
		proposal.Spec.ExpectedImpact.EstimatedSavingsUSD.String(),
		carbonLine,
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

func (m *PRManager) buildDiff(
	proposal *v1alpha1.OptimizationProposal,
	currentResources map[string]corev1.ResourceRequirements,
) string {
	return fmt.Sprintf("diff: update resources for %s", proposal.Spec.TargetWorkload.Name)
}

func (m *PRManager) shouldAutoMerge(proposal *v1alpha1.OptimizationProposal) bool {
	return proposal.Spec.DecisionMetadata.ConfidenceScore >= 80 &&
		proposal.Spec.DecisionMetadata.RiskScore <= 20
}
