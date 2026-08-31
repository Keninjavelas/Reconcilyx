package gitops

import (
	"context"
)

// PullRequest represents a pull request in a Git repository
type PullRequest struct {
	Title       string
	Description string
	HeadBranch  string
	BaseBranch  string
	Diff        string
}

// Comment represents a comment on a pull request
type Comment struct {
	Body string
}

// GitProvider defines the interface for Git operations (GitHub, GitLab, etc.)
type GitProvider interface {
	// CreatePullRequest creates a new pull request
	CreatePullRequest(ctx context.Context, pr *PullRequest) (string, error)
	// AddComment adds a comment to an existing pull request
	AddComment(ctx context.Context, prID string, comment *Comment) error
	// AutoMerge merges a pull request if certain conditions are met
	AutoMerge(ctx context.Context, prID string, conditions *MergeConditions) error
}

// MergeConditions defines conditions for auto-merging a PR
type MergeConditions struct {
	MinApprovalCount int
	MaxRiskScore     int32
	MinConfidence    int32
}
