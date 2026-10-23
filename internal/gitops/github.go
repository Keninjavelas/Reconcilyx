package gitops

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v63/github"
	"github.com/reconcilyx/operator/internal/telemetry"
	"golang.org/x/oauth2"
)

type GitHubProvider struct {
	client *github.Client
	owner  string
	repo   string
}

func NewGitHubProvider(token, owner, repo string) *GitHubProvider {
	var httpClient *http.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		httpClient = oauth2.NewClient(context.Background(), ts)
	}
	client := github.NewClient(httpClient)
	return &GitHubProvider{client: client, owner: owner, repo: repo}
}

func NewGitHubProviderFromEnv(owner, repo string) (*GitHubProvider, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable is not set")
	}
	return NewGitHubProvider(token, owner, repo), nil
}

func (g *GitHubProvider) CreatePullRequest(ctx context.Context, pr *PullRequest) (string, error) {
	baseBranch := pr.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}
	headBranch := pr.HeadBranch
	if headBranch == "" {
		headBranch = fmt.Sprintf("reconcilyx/opt-%d", time.Now().Unix())
	}

	ref, _, err := g.client.Git.GetRef(ctx, g.owner, g.repo, "refs/heads/"+baseBranch)
	if err != nil {
		return "", fmt.Errorf("failed to get base branch ref: %w", err)
	}

	newRef := &github.Reference{
		Ref:    github.String("refs/heads/" + headBranch),
		Object: &github.GitObject{SHA: ref.Object.SHA},
	}
	if _, _, err = g.client.Git.CreateRef(ctx, g.owner, g.repo, newRef); err != nil &&
		!strings.Contains(err.Error(), "Reference already exists") {
		return "", fmt.Errorf("failed to create head branch: %w", err)
	}

	manifestPath := pr.FilePath
	if manifestPath == "" {
		manifestPath = fmt.Sprintf("manifests/reconcilyx-proposal-%d.yaml", time.Now().Unix())
	}
	commitMsg := pr.Title
	if commitMsg == "" {
		commitMsg = "reconcilyx: optimization proposal"
	}
	opts := &github.RepositoryContentFileOptions{
		Message: github.String(commitMsg),
		Content: []byte(pr.Diff),
		Branch:  github.String(headBranch),
	}
	if sha, shaErr := g.getCurrentFileSHA(ctx, manifestPath, headBranch); shaErr == nil {
		opts.SHA = sha
		_, _, err = g.client.Repositories.UpdateFile(ctx, g.owner, g.repo, manifestPath, opts)
	} else {
		_, _, err = g.client.Repositories.CreateFile(ctx, g.owner, g.repo, manifestPath, opts)
	}
	if err != nil {
		return "", fmt.Errorf("failed to write manifest %s: %w", manifestPath, err)
	}

	pullReq := &github.NewPullRequest{
		Title:               github.String(pr.Title),
		Head:                github.String(headBranch),
		Base:                github.String(baseBranch),
		Body:                github.String(pr.Description),
		MaintainerCanModify: github.Bool(true),
	}
	created, _, err := g.client.PullRequests.Create(ctx, g.owner, g.repo, pullReq)
	if err != nil {
		if existing := g.findOpenPR(ctx, headBranch); existing != "" {
			return existing, nil
		}
		return "", fmt.Errorf("failed to create pull request: %w", err)
	}

	telemetry.PRCreatedTotal.WithLabelValues("github", g.owner, g.repo).Inc()

	return fmt.Sprintf("%d", created.GetNumber()), nil
}

func (g *GitHubProvider) findOpenPR(ctx context.Context, headBranch string) string {
	prs, _, err := g.client.PullRequests.List(ctx, g.owner, g.repo, &github.PullRequestListOptions{
		Head:  g.owner + ":" + headBranch,
		State: "open",
	})
	if err != nil || len(prs) == 0 {
		return ""
	}
	return strconv.Itoa(prs[0].GetNumber())
}

func (g *GitHubProvider) getCurrentFileSHA(ctx context.Context, path, branch string) (*string, error) {
	opts := &github.RepositoryContentGetOptions{Ref: branch}
	file, _, _, err := g.client.Repositories.GetContents(ctx, g.owner, g.repo, path, opts)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, fmt.Errorf("file not found at path: %s", path)
	}
	return file.SHA, nil
}

func (g *GitHubProvider) AddComment(ctx context.Context, prID string, comment *Comment) error {
	prNum := 0
	_, err := fmt.Sscanf(prID, "%d", &prNum)
	if err != nil {
		return fmt.Errorf("invalid PR id %q: %w", prID, err)
	}
	issueComment := &github.IssueComment{Body: github.String(comment.Body)}
	_, _, err = g.client.Issues.CreateComment(ctx, g.owner, g.repo, prNum, issueComment)
	return err
}

func (g *GitHubProvider) AutoMerge(ctx context.Context, prID string, conditions *MergeConditions) error {
	prNum := 0
	_, err := fmt.Sscanf(prID, "%d", &prNum)
	if err != nil {
		return fmt.Errorf("invalid PR id %q: %w", prID, err)
	}

	pr, _, err := g.client.PullRequests.Get(ctx, g.owner, g.repo, prNum)
	if err != nil {
		return fmt.Errorf("failed to get PR: %w", err)
	}

	if conditions != nil {
		if conditions.MaxRiskScore > 0 {
			// risk score would be embedded in labels/body; skip for now
		}
		if conditions.MinConfidence > 0 {
			// skip for now
		}
		if conditions.MinApprovalCount > 0 {
			reviews, _, err := g.client.PullRequests.ListReviews(ctx, g.owner, g.repo, prNum, nil)
			if err != nil {
				return fmt.Errorf("failed to list reviews: %w", err)
			}
			approved := 0
			for _, r := range reviews {
				if r.GetState() == "APPROVED" {
					approved++
				}
			}
			if approved < conditions.MinApprovalCount {
				return fmt.Errorf("insufficient approvals: %d < %d", approved, conditions.MinApprovalCount)
			}
		}
	}

	if !pr.GetMergeable() {
		return fmt.Errorf("PR %d is not mergeable (state=%s)", prNum, pr.GetMergeableState())
	}

	mergeOpts := &github.PullRequestOptions{
		MergeMethod: "squash",
		CommitTitle: pr.GetTitle(),
	}
	_, _, err = g.client.PullRequests.Merge(ctx, g.owner, g.repo, prNum, "Auto-merged by Reconcilyx", mergeOpts)
	if err != nil {
		return fmt.Errorf("failed to merge PR: %w", err)
	}

	telemetry.PRAutoMergedTotal.WithLabelValues("github", g.owner, g.repo).Inc()
	return nil
}

// GetFile reads a repository file at the given branch.
func (g *GitHubProvider) GetFile(ctx context.Context, path, branch string) ([]byte, error) {
	opts := &github.RepositoryContentGetOptions{Ref: branch}
	file, _, _, err := g.client.Repositories.GetContents(ctx, g.owner, g.repo, path, opts)
	if err != nil {
		var respErr *github.ErrorResponse
		if errors.As(err, &respErr) && respErr.Response != nil && respErr.Response.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %s@%s", ErrFileNotFound, path, branch)
		}
		return nil, err
	}
	if file == nil {
		return nil, fmt.Errorf("%w: %s@%s", ErrFileNotFound, path, branch)
	}
	content, err := file.GetContent()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return []byte(content), nil
}

// GetPullRequestStatus returns the lifecycle state of a pull request.
func (g *GitHubProvider) GetPullRequestStatus(ctx context.Context, prID string) (*PullRequestStatus, error) {
	prNum := 0
	if _, err := fmt.Sscanf(prID, "%d", &prNum); err != nil {
		return nil, fmt.Errorf("invalid PR id %q: %w", prID, err)
	}
	pr, _, err := g.client.PullRequests.Get(ctx, g.owner, g.repo, prNum)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR: %w", err)
	}
	return &PullRequestStatus{
		State:      pr.GetState(),
		Merged:     pr.GetMerged(),
		Mergeable:  pr.GetMergeable(),
		HeadBranch: pr.GetHead().GetRef(),
		Body:       pr.GetBody(),
	}, nil
}

// FindPRForCommit returns the first Reconcilyx PR associated with a commit.
func (g *GitHubProvider) FindPRForCommit(ctx context.Context, sha string) (string, error) {
	prs, _, err := g.client.PullRequests.ListPullRequestsWithCommit(ctx, g.owner, g.repo, sha, &github.ListOptions{PerPage: 10})
	if err != nil {
		return "", err
	}
	for _, pr := range prs {
		if strings.HasPrefix(pr.GetHead().GetRef(), "reconcilyx/") {
			return strconv.Itoa(pr.GetNumber()), nil
		}
	}
	return "", nil
}
