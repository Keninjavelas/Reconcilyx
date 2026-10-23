package gitops

import (
	"context"
	"fmt"
)

// MockProvider is a mock implementation of GitProvider for testing
type MockProvider struct {
	CreatedPRs    []*PullRequest
	AddedComments map[string][]*Comment
	MergedPRs     []string
	// Files seeds repository contents for GetFile. Keys are repository paths.
	Files map[string][]byte
	// PRStatuses overrides the default open status returned for a PR.
	PRStatuses map[string]*PullRequestStatus
}

// NewMockProvider creates a new mock Git provider
func NewMockProvider() *MockProvider {
	return &MockProvider{
		CreatedPRs:    make([]*PullRequest, 0),
		AddedComments: make(map[string][]*Comment),
		MergedPRs:     make([]string, 0),
		Files:         make(map[string][]byte),
		PRStatuses:    make(map[string]*PullRequestStatus),
	}
}

// CreatePullRequest implements GitProvider.CreatePullRequest
func (m *MockProvider) CreatePullRequest(ctx context.Context, pr *PullRequest) (string, error) {
	m.CreatedPRs = append(m.CreatedPRs, pr)
	prID := fmt.Sprintf("pr-%d", len(m.CreatedPRs))
	if m.PRStatuses == nil {
		m.PRStatuses = make(map[string]*PullRequestStatus)
	}
	m.PRStatuses[prID] = &PullRequestStatus{
		State:      "open",
		Mergeable:  true,
		HeadBranch: pr.HeadBranch,
		Body:       pr.Description,
	}
	return prID, nil
}

// AddComment implements GitProvider.AddComment
func (m *MockProvider) AddComment(ctx context.Context, prID string, comment *Comment) error {
	m.AddedComments[prID] = append(m.AddedComments[prID], comment)
	return nil
}

// AutoMerge implements GitProvider.AutoMerge
func (m *MockProvider) AutoMerge(ctx context.Context, prID string, conditions *MergeConditions) error {
	m.MergedPRs = append(m.MergedPRs, prID)
	if status, ok := m.PRStatuses[prID]; ok {
		status.State = "merged"
		status.Merged = true
	} else if m.PRStatuses != nil {
		m.PRStatuses[prID] = &PullRequestStatus{State: "merged", Merged: true}
	}
	return nil
}

// GetFile implements GitProvider.GetFile using the seeded Files map.
func (m *MockProvider) GetFile(ctx context.Context, path, branch string) ([]byte, error) {
	if data, ok := m.Files[path]; ok {
		out := make([]byte, len(data))
		copy(out, data)
		return out, nil
	}
	return nil, fmt.Errorf("%w: %s@%s", ErrFileNotFound, path, branch)
}

// GetPullRequestStatus implements GitProvider.GetPullRequestStatus.
func (m *MockProvider) GetPullRequestStatus(ctx context.Context, prID string) (*PullRequestStatus, error) {
	if status, ok := m.PRStatuses[prID]; ok {
		copyStatus := *status
		return &copyStatus, nil
	}
	return &PullRequestStatus{State: "open", Mergeable: true}, nil
}
