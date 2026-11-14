package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestFederatedDualTriggerWatermarkCompaction(t *testing.T) {
	agg := federated.NewAggregator(0)
	s := NewServer(":0", "test-key", ServerDeps{
		FederatedAggregator: agg,
		HubCompactThreshold: 3,
		HubCompactInterval:  time.Hour,
	})

	postBatch := func(n int) map[string]any {
		deltas := make([]federated.QTableDelta, 0, n)
		for i := 0; i < n; i++ {
			deltas = append(deltas, federated.QTableDelta{
				ClusterName: "spoke-alpha",
				StateKey:    federated.QStateKey{CPUBucket: 4, MemBucket: 6, CPUReqMilli: 500, MemReqMilli: 1024},
				ActionKey:   federated.QActionKey{CPUAdjPct: -10, MemAdjPct: -10},
				DeltaQ:      0.42,
				VisitCount:  5,
			})
		}
		raw, _ := json.Marshal(federated.DeltaBatch{ClusterName: "spoke-alpha", Epoch: 1, Deltas: deltas})
		req := httptest.NewRequest(http.MethodPost, "/api/federated/deltas", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", "test-key")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return out
	}

	// Send 2 deltas (< threshold 3) -> buffered, not yet compacted
	r1 := postBatch(2)
	if r1["compacted"] != false {
		t.Fatalf("expected compacted=false below watermark, got %v", r1)
	}
	if agg.PendingDeltaCount() != 2 {
		t.Fatalf("expected 2 pending deltas, got %d", agg.PendingDeltaCount())
	}

	// Send 2 more deltas -> crosses threshold 3 (total 4, NOT a multiple of 500 or 3) -> compacts!
	r2 := postBatch(2)
	if r2["compacted"] != true {
		t.Fatalf("expected compacted=true when watermark crossed, got %v", r2)
	}
	if agg.PendingDeltaCount() != 0 {
		t.Fatalf("expected 0 pending deltas after compaction, got %d", agg.PendingDeltaCount())
	}
	if r2["versionStr"] != "v1" || r2["modelHash"] == "" {
		t.Fatalf("expected v1 and non-empty modelHash, got %v", r2)
	}
}

func TestFederatedCompactEndpointAuthAndExecution(t *testing.T) {
	agg := federated.NewAggregator(0)
	s := NewServer(":0", "secret-hub-key", ServerDeps{
		FederatedAggregator: agg,
		HubCompactThreshold: 500,
		HubCompactInterval:  time.Hour,
	})

	// Unauthenticated request must fail with 401
	unauthReq := httptest.NewRequest(http.MethodPost, "/api/v1/federation/compact", nil)
	unauthRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without API key, got %d", unauthRec.Code)
	}

	// Add 2 pending deltas (< 500 threshold) and trigger explicit compaction
	agg.AddDeltas([]federated.QTableDelta{
		{
			ClusterName: "spoke-1",
			StateKey:    federated.QStateKey{CPUBucket: 2, MemBucket: 2, CPUReqMilli: 250, MemReqMilli: 512},
			ActionKey:   federated.QActionKey{CPUAdjPct: -15, MemAdjPct: -10},
			DeltaQ:      0.33,
			VisitCount:  4,
		},
		{
			ClusterName: "spoke-2",
			StateKey:    federated.QStateKey{CPUBucket: 2, MemBucket: 2, CPUReqMilli: 250, MemReqMilli: 512},
			ActionKey:   federated.QActionKey{CPUAdjPct: -15, MemAdjPct: -10},
			DeltaQ:      0.35,
			VisitCount:  6,
		},
	})

	authReq := httptest.NewRequest(http.MethodPost, "/api/v1/federation/compact", nil)
	authReq.Header.Set("X-API-Key", "secret-hub-key")
	authRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid API key, got %d: %s", authRec.Code, authRec.Body.String())
	}
	var out struct {
		Status     string `json:"status"`
		VersionStr string `json:"versionStr"`
		ModelHash  string `json:"modelHash"`
		Entries    int    `json:"entries"`
		Pending    int    `json:"pending"`
	}
	if err := json.Unmarshal(authRec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode compact response: %v", err)
	}
	if out.Status != "compacted" || out.VersionStr != "v1" || out.ModelHash == "" || out.Entries != 1 || out.Pending != 0 {
		t.Fatalf("unexpected compact response: %+v", out)
	}
}

func TestFederatedDeltasEndpointDeduplicatesBatchID(t *testing.T) {
	s := newFederatedTestServer(t)
	batch := federated.DeltaBatch{
		BatchID:     "batch-abc-123",
		ClusterName: "spoke-dedup",
		Epoch:       1,
		Deltas: []federated.QTableDelta{{
			ClusterName: "spoke-dedup",
			StateKey:    federated.QStateKey{CPUBucket: 3, MemBucket: 4},
			ActionKey:   federated.QActionKey{CPUAdjPct: -10},
			DeltaQ:      0.4,
			VisitCount:  2,
		}},
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	post := func() map[string]any {
		req := httptest.NewRequest(http.MethodPost, "/api/federated/deltas", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return resp
	}

	first := post()
	if first["received"] != float64(1) || first["duplicate"] != false {
		t.Fatalf("expected first POST accepted with received=1, duplicate=false; got %+v", first)
	}

	second := post()
	if second["received"] != float64(0) || second["duplicate"] != true {
		t.Fatalf("expected second POST ignored as duplicate; got %+v", second)
	}
	if s.fedAggregator.PendingDeltaCount() != 1 {
		t.Fatalf("expected pending count 1 (no double apply), got %d", s.fedAggregator.PendingDeltaCount())
	}
}
