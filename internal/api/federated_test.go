package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reconcilyx/operator/internal/federated"
)

func newFederatedTestServer(t *testing.T) *Server {
	t.Helper()
	agg := federated.NewAggregator(0)
	return NewServer(":0", "", ServerDeps{FederatedAggregator: agg})
}

func addFederatedDelta(t *testing.T, s *Server, q float64) {
	t.Helper()
	s.fedAggregator.AddDeltas([]federated.QTableDelta{{
		ClusterName: "spoke-test",
		StateKey:    federated.QStateKey{CPUBucket: 5, MemBucket: 10, CPUReqMilli: 1000, MemReqMilli: 2048},
		ActionKey:   federated.QActionKey{CPUAdjPct: -20, MemAdjPct: -10},
		DeltaQ:      q,
		VisitCount:  10,
	}})
	if err := s.compactAndPersist(context.Background()); err != nil {
		t.Fatalf("compactAndPersist failed: %v", err)
	}
}

func fetchModel(t *testing.T, s *Server, query string) (*federated.GlobalQTable, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/federated/global-model"+query, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var snap federated.GlobalQTable
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode global model: %v", err)
	}
	return &snap, rec
}

func TestFederatedModelChannelSelection(t *testing.T) {
	s := newFederatedTestServer(t)
	addFederatedDelta(t, s, 0.5)    // v1: new entry -> canary
	addFederatedDelta(t, s, 0.5001) // v2: negligible shift -> stable
	addFederatedDelta(t, s, 0.9)    // v3: large shift -> canary

	stable, stableRec := fetchModel(t, s, "?channel=stable&cluster=spoke-regular")
	if stable.VersionStr != "v2" {
		t.Fatalf("expected stable channel to serve v2, got %s", stable.VersionStr)
	}
	if got := stableRec.Header().Get("X-Reconcilyx-Model-Channel"); got != "stable" {
		t.Fatalf("expected stable channel header, got %q", got)
	}

	canary, canaryRec := fetchModel(t, s, "?channel=canary&cluster=spoke-canary")
	if canary.VersionStr != "v3" {
		t.Fatalf("expected canary channel to serve v3, got %s", canary.VersionStr)
	}
	if got := canaryRec.Header().Get("X-Reconcilyx-Model-Version"); got != "v3" {
		t.Fatalf("expected version header v3, got %q", got)
	}

	latest, _ := fetchModel(t, s, "")
	if latest.VersionStr != "v3" {
		t.Fatalf("expected unqualified request to serve live snapshot v3, got %s", latest.VersionStr)
	}
}

func TestFederatedCanaryRollback(t *testing.T) {
	s := newFederatedTestServer(t)
	addFederatedDelta(t, s, 0.5)    // v1 canary
	addFederatedDelta(t, s, 0.5001) // v2 stable
	addFederatedDelta(t, s, 0.9)    // v3 canary

	req := httptest.NewRequest(http.MethodDelete, "/api/federated/releases/canary?reason=regression", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	after, afterRec := fetchModel(t, s, "?channel=canary")
	if after.VersionStr != "v2" {
		t.Fatalf("expected canary opt-in to fall back to v2, got %s", after.VersionStr)
	}
	if got := afterRec.Header().Get("X-Reconcilyx-Model-Channel"); got != "stable" {
		t.Fatalf("expected fallback channel header stable, got %q", got)
	}
	if got := s.modelRegistry.CanaryVersion(); got != "" {
		t.Fatalf("expected canary channel empty after rollback, got %s", got)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/federated/releases/canary", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 when no canary is active, got %d", rec.Code)
	}
}
