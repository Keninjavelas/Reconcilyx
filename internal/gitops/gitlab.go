package gitops

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/reconcilyx/operator/internal/telemetry"
	"github.com/xanzy/go-gitlab"
)

type GitLabProvider struct {
	client    *gitlab.Client
	projectID interface{}
	owner     string
	repo      string
}

func NewGitLabProvider(token, baseURL, owner, repo string) (*GitLabProvider, error) {
	var opts []gitlab.ClientOptionFunc
	if baseURL != "" {
		opts = append(opts, gitlab.WithBaseURL(baseURL))
	}
	client, err := gitlab.NewClient(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gitlab client: %w", err)
	}
	return &GitLabProvider{
		client:    client,
		projectID: fmt.Sprintf("%s/%s", owner, repo),
		owner:     owner,
		repo:      repo,
	}, nil
}

func NewGitLabProviderFromEnv(owner, repo string) (*GitLabProvider, error) {
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITLAB_TOKEN environment variable is not set")
	}
	baseURL := os.Getenv("GITLAB_BASE_URL")
	return NewGitLabProvider(token, baseURL, owner, repo)
}

func (g *GitLabProvider) CreatePullRequest(ctx context.Context, pr *PullRequest) (string, error) {
	baseBranch := pr.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}
	headBranch := pr.HeadBranch
	if headBranch == "" {
		headBranch = fmt.Sprintf("reconcilyx/opt-%d", time.Now().Unix())
	}

	branchOpts := &gitlab.CreateBranchOptions{
		Branch: gitlab.String(headBranch),
		Ref:    gitlab.String(baseBranch),
	}
	_, _, err := g.client.Branches.CreateBranch(g.projectID, branchOpts, gitlab.WithContext(ctx))
	if err != nil && !isGitLab404BranchExists(err) {
		return "", fmt.Errorf("failed to create branch: %w", err)
	}

	filePath := pr.HeadBranch
	if filePath == "" || filePath == headBranch {
		filePath = fmt.Sprintf("manifests/reconcilyx-proposal-%d.yaml", time.Now().Unix())
	}
	content := pr.Diff
	if content == "" {
		content = "# Reconcilyx optimization patch\n"
	}
	commitMsg := pr.Title
	if commitMsg == "" {
		commitMsg = "reconcilyx: optimization proposal"
	}
	_, _, err = g.client.RepositoryFiles.GetFileMetaData(g.projectID, filePath, &gitlab.GetFileMetaDataOptions{Ref: gitlab.String(headBranch)}, gitlab.WithContext(ctx))
	if err == nil {
		updateOpts := &gitlab.UpdateFileOptions{
			Branch:        gitlab.String(headBranch),
			CommitMessage: gitlab.String(commitMsg),
			Content:       gitlab.String(content),
		}
		_, _, err = g.client.RepositoryFiles.UpdateFile(g.projectID, filePath, updateOpts, gitlab.WithContext(ctx))
	} else {
		createOpts := &gitlab.CreateFileOptions{
			Branch:        gitlab.String(headBranch),
			CommitMessage: gitlab.String(commitMsg),
			Content:       gitlab.String(content),
		}
		_, _, err = g.client.RepositoryFiles.CreateFile(g.projectID, filePath, createOpts, gitlab.WithContext(ctx))
	}
	if err != nil {
		return "", fmt.Errorf("failed to write manifest file: %w", err)
	}

	mrOpts := &gitlab.CreateMergeRequestOptions{
		Title:        gitlab.String(pr.Title),
		SourceBranch: gitlab.String(headBranch),
		TargetBranch: gitlab.String(baseBranch),
		Description:  gitlab.String(pr.Description),
		RemoveSourceBranch: gitlab.Bool(true),
	}
	mr, _, err := g.client.MergeRequests.CreateMergeRequest(g.projectID, mrOpts, gitlab.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("failed to create merge request: %w", err)
	}

	telemetry.PRCreatedTotal.WithLabelValues("gitlab", g.owner, g.repo).Inc()

	return strconv.Itoa(mr.IID), nil
}

func isGitLab404BranchExists(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return (len(msg) > 0) && (msg[0:1] == "4" || containsAny(msg, "already exists", "Branch Already Exists"))
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0) {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func (g *GitLabProvider) AddComment(ctx context.Context, prID string, comment *Comment) error {
	mrIID, err := strconv.Atoi(prID)
	if err != nil {
		return fmt.Errorf("invalid MR IID %q: %w", prID, err)
	}
	opts := &gitlab.CreateMergeRequestNoteOptions{Body: gitlab.String(comment.Body)}
	_, _, err = g.client.Notes.CreateMergeRequestNote(g.projectID, mrIID, opts, gitlab.WithContext(ctx))
	return err
}

func (g *GitLabProvider) AutoMerge(ctx context.Context, prID string, conditions *MergeConditions) error {
	mrIID, err := strconv.Atoi(prID)
	if err != nil {
		return fmt.Errorf("invalid MR IID %q: %w", prID, err)
	}

	mr, _, err := g.client.MergeRequests.GetMergeRequest(g.projectID, mrIID, nil, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("failed to get MR: %w", err)
	}

	if conditions != nil && conditions.MinApprovalCount > 0 {
		if mr.ApprovalsBeforeMerge < conditions.MinApprovalCount {
			return fmt.Errorf("insufficient approvals: %d < %d", mr.ApprovalsBeforeMerge, conditions.MinApprovalCount)
		}
	}

	switch mr.State {
	case "merged":
		return nil
	case "closed", "locked":
		return fmt.Errorf("MR %d is in state %s, cannot auto-merge", mrIID, mr.State)
	}

	mergeOpts := &gitlab.AcceptMergeRequestOptions{
		SquashCommitMessage:       gitlab.String(mr.Title),
		Squash:                    gitlab.Bool(true),
		ShouldRemoveSourceBranch:  gitlab.Bool(true),
		MergeCommitMessage:        gitlab.String("Auto-merged by Reconcilyx"),
	}
	_, _, err = g.client.MergeRequests.AcceptMergeRequest(g.projectID, mrIID, mergeOpts, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("failed to accept merge request: %w", err)
	}

	telemetry.PRAutoMergedTotal.WithLabelValues("gitlab", g.owner, g.repo).Inc()
	return nil
}
