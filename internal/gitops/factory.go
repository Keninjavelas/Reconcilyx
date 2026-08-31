package gitops

import (
	"fmt"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
)

type ProviderFactory struct{}

func NewProviderFactory() *ProviderFactory {
	return &ProviderFactory{}
}

func (f *ProviderFactory) ForPolicy(policy *optimizationv1alpha1.OptimizationPolicy) (GitProvider, error) {
	cfg := policy.Spec.GitOps
	switch cfg.Provider {
	case "", optimizationv1alpha1.GitProviderMock:
		return NewMockProvider(), nil
	case optimizationv1alpha1.GitProviderGitHub:
		owner := cfg.Owner
		repo := cfg.Repo
		if owner == "" || repo == "" {
			return NewMockProvider(), fmt.Errorf("github provider requires gitOps.owner and gitOps.repo")
		}
		prov, err := NewGitHubProviderFromEnv(owner, repo)
		if err != nil {
			return NewMockProvider(), err
		}
		return prov, nil
	case optimizationv1alpha1.GitProviderGitLab:
		owner := cfg.Owner
		repo := cfg.Repo
		if owner == "" || repo == "" {
			return NewMockProvider(), fmt.Errorf("gitlab provider requires gitOps.owner and gitOps.repo")
		}
		prov, err := NewGitLabProviderFromEnv(owner, repo)
		if err != nil {
			return NewMockProvider(), err
		}
		return prov, nil
	default:
		return NewMockProvider(), fmt.Errorf("unknown git provider: %s", cfg.Provider)
	}
}

func (f *ProviderFactory) NewGitHub(owner, repo string) (GitProvider, error) {
	return NewGitHubProviderFromEnv(owner, repo)
}

func (f *ProviderFactory) NewGitLab(owner, repo string) (GitProvider, error) {
	return NewGitLabProviderFromEnv(owner, repo)
}
