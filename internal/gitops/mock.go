package gitops

import (
	"context"
	"fmt"
)

// MockProvider is a mock implementation of GitProvider for testing
type MockProvider struct {
	CreatedPRs  []*PullRequest
	AddedComments map[string][]*Comment
	MergedPRs   []string
}

// NewMockProvider creates a new mock Git provider
func NewMockProvider() *MockProvider {
	return &MockProvider{
		CreatedPRs:    make([]*PullRequest, 0),
		AddedComments: make(map[string][]*Comment),
		MergedPRs:     make([]string, 0),
	}
}

// CreatePullRequest implements GitProvider.CreatePullRequest
func (m *MockProvider) CreatePullRequest(ctx context.Context, pr *PullRequest) (string, error) {
	m.CreatedPRs = append(m.CreatedPRs, pr)
	prID := fmt.Sprintf("pr-%d", len(m.CreatedPRs))
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
	return nil
}
