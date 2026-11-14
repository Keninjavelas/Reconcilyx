package webhook

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/reconcilyx/operator/internal/gitops"
)

func TestAutoMergeAuthorized(t *testing.T) {
	tests := []struct {
		name   string
		status *gitops.PullRequestStatus
		want   bool
	}{
		{
			name:   "policy-marked reconcilyx branch",
			status: &gitops.PullRequestStatus{State: "open", HeadBranch: "reconcilyx/default-demo", Body: "body\n" + gitops.AutoMergeMarker},
			want:   true,
		},
		{
			name:   "missing marker",
			status: &gitops.PullRequestStatus{State: "open", HeadBranch: "reconcilyx/default-demo", Body: "body"},
			want:   false,
		},
		{
			name:   "non-reconcilyx branch",
			status: &gitops.PullRequestStatus{State: "open", HeadBranch: "feature/x", Body: gitops.AutoMergeMarker},
			want:   false,
		},
		{
			name:   "already merged",
			status: &gitops.PullRequestStatus{State: "merged", Merged: true, HeadBranch: "reconcilyx/default-demo", Body: gitops.AutoMergeMarker},
			want:   false,
		},
		{
			name:   "closed without merge",
			status: &gitops.PullRequestStatus{State: "closed", HeadBranch: "reconcilyx/default-demo", Body: gitops.AutoMergeMarker},
			want:   false,
		},
		{name: "nil", status: nil, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := autoMergeAuthorized(tc.status); got != tc.want {
				t.Fatalf("autoMergeAuthorized() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWebhookHandler_VerifiesGitHubSignature(t *testing.T) {
	server := NewAutoMergeWebhookServer(logr.Discard(), gitops.NewProviderFactory(), ":0", "topsecret")
	handler := server.Handler()
	body := []byte(`{"ref":"refs/heads/main"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "ping")
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("invalid signature: got %d, want %d", rec.Code, http.StatusForbidden)
	}

	req = httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "ping")
	req.Header.Set("X-Hub-Signature-256", computeGitHubSignature([]byte("topsecret"), body))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid signature: got %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWebhookHandler_VerifiesGitLabToken(t *testing.T) {
	server := NewAutoMergeWebhookServer(logr.Discard(), gitops.NewProviderFactory(), ":0", "topsecret")
	handler := server.Handler()
	body := []byte(`{"object_kind":"merge_request"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	req.Header.Set("X-Gitlab-Token", "wrong")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("invalid token: got %d, want %d", rec.Code, http.StatusForbidden)
	}

	req = httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	req.Header.Set("X-Gitlab-Token", "topsecret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token: got %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWebhookHandler_Healthz(t *testing.T) {
	server := NewAutoMergeWebhookServer(logr.Discard(), gitops.NewProviderFactory(), ":0", "")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz: got %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWebhookSignatureAndReplayProtection(t *testing.T) {
	secret := "top-secret-webhook-key"
	srv := NewAutoMergeWebhookServer(logr.Discard(), nil, ":0", secret)
	handler := srv.Handler()

	body := []byte(`{"state":"pending","sha":"abc123"}`)
	sig := computeGitHubSignature([]byte(secret), body)

	// 1. Valid fresh GitHub delivery succeeds
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "status")
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Delivery", "delivery-001")
	req.Header.Set("X-Reconcilyx-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on first signed delivery, got %d (%s)", rec.Code, rec.Body.String())
	}

	// 2. Duplicate delivery ID (replay) is rejected with 403
	reqDup := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	reqDup.Header.Set("X-GitHub-Event", "status")
	reqDup.Header.Set("X-Hub-Signature-256", sig)
	reqDup.Header.Set("X-GitHub-Delivery", "delivery-001")
	recDup := httptest.NewRecorder()
	handler.ServeHTTP(recDup, reqDup)
	if recDup.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on duplicate delivery replay, got %d", recDup.Code)
	}

	// 3. Stale timestamp (> 5m old) is rejected with 403
	reqStale := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	reqStale.Header.Set("X-GitHub-Event", "status")
	reqStale.Header.Set("X-Hub-Signature-256", sig)
	reqStale.Header.Set("X-GitHub-Delivery", "delivery-002")
	reqStale.Header.Set("X-Reconcilyx-Timestamp", fmt.Sprintf("%d", time.Now().Add(-15*time.Minute).Unix()))
	recStale := httptest.NewRecorder()
	handler.ServeHTTP(recStale, reqStale)
	if recStale.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on stale timestamp, got %d", recStale.Code)
	}

	// 4. GitLab valid event UUID vs duplicate event UUID
	glReq := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte(`{"object_kind":"note"}`)))
	glReq.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	glReq.Header.Set("X-Gitlab-Token", secret)
	glReq.Header.Set("X-Gitlab-Event-UUID", "gl-evt-1")
	glRec := httptest.NewRecorder()
	handler.ServeHTTP(glRec, glReq)
	if glRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on GitLab webhook, got %d", glRec.Code)
	}

	glReplay := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte(`{"object_kind":"note"}`)))
	glReplay.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	glReplay.Header.Set("X-Gitlab-Token", secret)
	glReplay.Header.Set("X-Gitlab-Event-UUID", "gl-evt-1")
	glReplayRec := httptest.NewRecorder()
	handler.ServeHTTP(glReplayRec, glReplay)
	if glReplayRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on replayed GitLab event UUID, got %d", glReplayRec.Code)
	}
}
