package gitops

import (
	"context"
	"errors"
)

// AutoMergeMarker is embedded in PR bodies when the policy allows the PR to be
// merged after CI succeeds. The webhook refuses to merge PRs without it.
const AutoMergeMarker = "<!-- reconcilyx-auto-merge: true -->"

// ErrFileNotFound is returned when a manifest path does not exist in the
// repository at the requested branch.
var ErrFileNotFound = errors.New("gitops: file not found")

// PullRequest represents a pull request in a Git repository.
type PullRequest struct {
	Title       string
	Description string
	HeadBranch  string
	BaseBranch  string
	// FilePath is the repository-relative path that receives the manifest
	// content in Diff. Providers commit the file to HeadBranch.
	FilePath string
	// Diff carries the complete patched manifest file contents.
	Diff string
}

// Comment represents a comment on a pull request
type Comment struct {
	Body string
}

// PullRequestStatus is the provider-neutral view of a PR/MR lifecycle.
type PullRequestStatus struct {
	// State is the provider state: open, merged, closed.
	State string
	// Merged reports whether the change has landed on the base branch.
	Merged bool
	// Mergeable reports whether the provider currently considers it mergeable.
	Mergeable bool
	// HeadBranch is the source branch of the PR.
	HeadBranch string
	// Body is the PR description, used to detect the auto-merge marker.
	Body string
}

// IsOpen reports whether the PR is still awaiting review or CI.
func (s *PullRequestStatus) IsOpen() bool {
	return s != nil && !s.Merged && s.State != "closed"
}

// GitProvider defines the interface for Git operations (GitHub, GitLab, etc.)
type GitProvider interface {
	// CreatePullRequest creates a new pull request
	CreatePullRequest(ctx context.Context, pr *PullRequest) (string, error)
	// AddComment adds a comment to an existing pull request
	AddComment(ctx context.Context, prID string, comment *Comment) error
	// AutoMerge merges a pull request if certain conditions are met
	AutoMerge(ctx context.Context, prID string, conditions *MergeConditions) error
	// GetFile reads a repository file at the given branch. It returns
	// ErrFileNotFound when the path does not exist.
	GetFile(ctx context.Context, path, branch string) ([]byte, error)
	// GetPullRequestStatus returns the current lifecycle state of a PR/MR.
	GetPullRequestStatus(ctx context.Context, prID string) (*PullRequestStatus, error)
}

// MergeConditions defines conditions for auto-merging a PR
type MergeConditions struct {
	MinApprovalCount int
	MaxRiskScore     int32
	MinConfidence    int32
}
