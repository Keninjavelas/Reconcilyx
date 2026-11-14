package webhook

import (
	"container/list"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/reconcilyx/operator/internal/gitops"
)

const (
	maxWebhookBodyBytes  = 1 << 20 // 1 MiB
	webhookReplayWindow  = 10 * time.Minute
	webhookTimestampSkew = 5 * time.Minute
	maxReplayEntries     = 10000
)

type replayEntry struct {
	id     string
	seenAt time.Time
}

type AutoMergeWebhookServer struct {
	logger         logr.Logger
	providers      *gitops.ProviderFactory
	secretToken    string
	addr           string
	replayMu       sync.Mutex
	seenDeliveries map[string]*list.Element
	replayOrder    *list.List
	maxReplayCap   int
}

type GitHubWebhookPush struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	} `json:"repository"`
}

type GitHubStatusEvent struct {
	State  string `json:"state"`
	Sha    string `json:"sha"`
	Commit struct {
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	} `json:"commit"`
	Repository struct {
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	} `json:"repository"`
}

type GitLabMRNoteEvent struct {
	ObjectKind string `json:"object_kind"`
	Project    struct {
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
	MergeRequest struct {
		ID    int    `json:"id"`
		IID   int    `json:"iid"`
		State string `json:"state"`
		Head  struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"merge_request"`
	ObjectAttributes struct {
		ID          int    `json:"id"`
		State       string `json:"state"`
		MergeStatus string `json:"merge_status"`
	} `json:"object_attributes"`
	Pipeline struct {
		Status string `json:"status"`
	} `json:"pipeline"`
}

func NewAutoMergeWebhookServer(logger logr.Logger, providers *gitops.ProviderFactory, addr, secretToken string) *AutoMergeWebhookServer {
	return &AutoMergeWebhookServer{
		logger:         logger,
		providers:      providers,
		secretToken:    secretToken,
		addr:           addr,
		seenDeliveries: make(map[string]*list.Element),
		replayOrder:    list.New(),
		maxReplayCap:   maxReplayEntries,
	}
}

func NewAutoMergeWebhookServerFromEnv(logger logr.Logger, providers *gitops.ProviderFactory) *AutoMergeWebhookServer {
	addr := os.Getenv("AUTOMERGE_WEBHOOK_ADDR")
	if addr == "" {
		addr = ":8090"
	}
	token := os.Getenv("GITOPS_WEBHOOK_SECRET")
	return NewAutoMergeWebhookServer(logger, providers, addr, token)
}

func computeGitHubSignature(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func (s *AutoMergeWebhookServer) verifyGitHub(r *http.Request, body []byte) bool {
	if s.secretToken == "" {
		return true
	}
	got := r.Header.Get("X-Hub-Signature-256")
	if got == "" {
		return false
	}
	want := computeGitHubSignature([]byte(s.secretToken), body)
	return hmac.Equal([]byte(got), []byte(want))
}

func (s *AutoMergeWebhookServer) verifyGitLab(r *http.Request, body []byte) bool {
	if s.secretToken == "" {
		return true
	}
	token := r.Header.Get("X-Gitlab-Token")
	sumA := sha256.Sum256([]byte(token))
	sumB := sha256.Sum256([]byte(s.secretToken))
	return subtle.ConstantTimeCompare(sumA[:], sumB[:]) == 1 &&
		subtle.ConstantTimeCompare([]byte(token), []byte(s.secretToken)) == 1
}

// verifyReplayWindow rejects duplicate delivery IDs (when supplied) and stale timestamps.
func (s *AutoMergeWebhookServer) verifyReplayWindow(r *http.Request, deliveryID string) bool {
	if tsRaw := strings.TrimSpace(r.Header.Get("X-Reconcilyx-Timestamp")); tsRaw != "" {
		sec, err := strconv.ParseInt(tsRaw, 10, 64)
		if err != nil {
			return false
		}
		ts := time.Unix(sec, 0)
		skew := time.Since(ts)
		if skew < 0 {
			skew = -skew
		}
		if skew > webhookTimestampSkew {
			return false
		}
	}

	deliveryID = strings.TrimSpace(deliveryID)
	if deliveryID == "" {
		return true
	}

	now := time.Now()
	s.replayMu.Lock()
	defer s.replayMu.Unlock()
	if s.seenDeliveries == nil {
		s.seenDeliveries = make(map[string]*list.Element)
	}
	if s.replayOrder == nil {
		s.replayOrder = list.New()
	}
	capLimit := s.maxReplayCap
	if capLimit <= 0 {
		capLimit = maxReplayEntries
	}

	// Evict TTL-expired entries from oldest-first list tail.
	for elem := s.replayOrder.Back(); elem != nil; {
		prev := elem.Prev()
		entry := elem.Value.(*replayEntry)
		if now.Sub(entry.seenAt) > webhookReplayWindow {
			s.replayOrder.Remove(elem)
			delete(s.seenDeliveries, entry.id)
			elem = prev
			continue
		}
		break
	}

	if _, exists := s.seenDeliveries[deliveryID]; exists {
		return false
	}

	// Enforce hard capacity bound so bursts of unique delivery IDs never OOM the server.
	for len(s.seenDeliveries) >= capLimit {
		oldest := s.replayOrder.Back()
		if oldest == nil {
			break
		}
		entry := oldest.Value.(*replayEntry)
		s.replayOrder.Remove(oldest)
		delete(s.seenDeliveries, entry.id)
	}

	elem := s.replayOrder.PushFront(&replayEntry{id: deliveryID, seenAt: now})
	s.seenDeliveries[deliveryID] = elem
	return true
}

func (s *AutoMergeWebhookServer) handleGitHub(w http.ResponseWriter, r *http.Request) {
	event := r.Header.Get("X-GitHub-Event")
	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	if !s.verifyGitHub(r, body) {
		http.Error(w, "invalid signature", http.StatusForbidden)
		return
	}
	if !s.verifyReplayWindow(r, r.Header.Get("X-GitHub-Delivery")) {
		http.Error(w, "replay or stale timestamp rejected", http.StatusForbidden)
		return
	}
	switch event {
	case "status":
		var evt GitHubStatusEvent
		if err := json.Unmarshal(body, &evt); err != nil {
			http.Error(w, "parse status", http.StatusBadRequest)
			return
		}
		if strings.EqualFold(evt.State, "success") {
			s.logger.Info("GitHub commit status success", "sha", evt.Sha, "repo", evt.Repository.FullName)
			go s.tryAutoMergeGitHub(evt.Repository.Owner.Login, evt.Repository.Name, evt.Sha)
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *AutoMergeWebhookServer) handleGitLab(w http.ResponseWriter, r *http.Request) {
	event := r.Header.Get("X-Gitlab-Event")
	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	if !s.verifyGitLab(r, body) {
		http.Error(w, "invalid token", http.StatusForbidden)
		return
	}
	if !s.verifyReplayWindow(r, r.Header.Get("X-Gitlab-Event-UUID")) {
		http.Error(w, "replay or stale timestamp rejected", http.StatusForbidden)
		return
	}
	if strings.HasPrefix(event, "Merge Request Hook") {
		var mr GitLabMRNoteEvent
		if err := json.Unmarshal(body, &mr); err != nil {
			http.Error(w, "parse mr", http.StatusBadRequest)
			return
		}
		pipelineOk := strings.EqualFold(mr.Pipeline.Status, "success") || mr.Pipeline.Status == ""
		stateOk := strings.EqualFold(mr.ObjectAttributes.State, "opened") ||
			strings.EqualFold(mr.MergeRequest.State, "opened")
		if pipelineOk && stateOk && strings.EqualFold(mr.ObjectAttributes.MergeStatus, "can_be_merged") {
			owner, repo, ok := strings.Cut(mr.Project.PathWithNamespace, "/")
			if !ok || owner == "" || repo == "" {
				s.logger.Info("GitLab event missing project path; auto-merge skipped", "id", mr.MergeRequest.IID)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
				return
			}
			s.logger.Info("GitLab MR mergeable with pipeline success", "id", mr.MergeRequest.IID, "project", mr.Project.PathWithNamespace)
			go s.tryAutoMergeGitLab(owner, repo, strconv.Itoa(mr.MergeRequest.IID))
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *AutoMergeWebhookServer) tryAutoMergeGitHub(owner, repo, sha string) {
	provider, err := s.providers.NewGitHub(owner, repo)
	if err != nil {
		s.logger.Error(err, "Cannot create GitHub provider for auto-merge", "owner", owner, "repo", repo)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	finder, ok := provider.(interface {
		FindPRForCommit(ctx context.Context, sha string) (string, error)
	})
	if !ok {
		s.logger.Info("Provider does not support commit lookup, auto-merge skipped", "sha", sha)
		return
	}
	prID, err := finder.FindPRForCommit(ctx, sha)
	if err != nil || prID == "" {
		s.logger.Info("No Reconcilyx PR found for commit", "sha", sha, "err", err)
		return
	}
	status, err := provider.GetPullRequestStatus(ctx, prID)
	if err != nil {
		s.logger.Error(err, "Cannot read PR status", "pr", prID)
		return
	}
	if !autoMergeAuthorized(status) {
		s.logger.Info("PR is not policy-marked for auto-merge, skipping", "pr", prID, "branch", status.HeadBranch)
		return
	}
	conditions := gitops.MergeConditions{MinApprovalCount: 0}
	if err := provider.AutoMerge(ctx, prID, &conditions); err != nil {
		s.logger.Error(err, "Auto-merge failed", "pr", prID)
		return
	}
	s.logger.Info("Auto-merged PR on success status", "pr", prID)
}

func (s *AutoMergeWebhookServer) tryAutoMergeGitLab(owner, repo, mrID string) {
	provider, err := s.providers.NewGitLab(owner, repo)
	if err != nil {
		s.logger.Error(err, "Cannot create GitLab provider for auto-merge", "owner", owner, "repo", repo)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, err := provider.GetPullRequestStatus(ctx, mrID)
	if err != nil {
		s.logger.Error(err, "Cannot read MR status", "mr", mrID)
		return
	}
	if !autoMergeAuthorized(status) {
		s.logger.Info("MR is not policy-marked for auto-merge, skipping", "mr", mrID, "branch", status.HeadBranch)
		return
	}

	conditions := gitops.MergeConditions{MinApprovalCount: 0}
	if err := provider.AutoMerge(ctx, mrID, &conditions); err != nil {
		s.logger.Error(err, "Auto-merge failed", "mr", mrID)
		return
	}
	s.logger.Info("Auto-merged MR on success pipeline", "mr", mrID)
}

// autoMergeAuthorized ensures the webhook only merges Reconcilyx PRs whose
// policy opted into auto-merge. The marker is written by PRManager only for
// proposals that also meet the confidence/risk thresholds.
func autoMergeAuthorized(status *gitops.PullRequestStatus) bool {
	if status == nil || status.Merged || status.State == "closed" {
		return false
	}
	return strings.HasPrefix(status.HeadBranch, "reconcilyx/") &&
		strings.Contains(status.Body, gitops.AutoMergeMarker)
}

func (s *AutoMergeWebhookServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("X-GitHub-Event") != "" {
			s.handleGitHub(w, r)
			return
		}
		if r.Header.Get("X-Gitlab-Event") != "" {
			s.handleGitLab(w, r)
			return
		}
		http.Error(w, "unknown provider", http.StatusBadRequest)
	})
	return mux
}

func (s *AutoMergeWebhookServer) Start() error {
	return s.StartContext(context.Background())
}

// StartContext runs the webhook listener until ctx is cancelled.
func (s *AutoMergeWebhookServer) StartContext(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	s.logger.Info("Starting auto-merge webhook server", "addr", s.addr, "signature_verification", s.secretToken != "")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("webhook server: %w", err)
	}
	return nil
}
