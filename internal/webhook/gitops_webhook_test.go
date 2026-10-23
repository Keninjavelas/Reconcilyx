package webhook

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

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
