package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/reconcilyx/operator/internal/telemetry"
	secauth "github.com/reconcilyx/operator/pkg/security/auth"
)

type apiMockWindowedClient struct {
	cpuMatrix model.Matrix
	memMatrix model.Matrix
}

func (m *apiMockWindowedClient) QueryRangeWindow(_ context.Context, query string, _ time.Time, _ time.Time, _ time.Duration) (model.Value, error) {
	if strings.Contains(query, "container_cpu_usage_seconds_total") {
		return m.cpuMatrix, nil
	}
	return m.memMatrix, nil
}

func TestBackfillEndpointRBACAndIdempotency(t *testing.T) {
	t0 := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	t1 := t0.Add(5 * time.Minute)
	metric := model.Metric{"namespace": "prod", "pod": "web-0", "uid": "uid-web-0"}
	engine := telemetry.NewBackfillEngine(&apiMockWindowedClient{
		cpuMatrix: model.Matrix{
			&model.SampleStream{Metric: metric, Values: []model.SamplePair{{Timestamp: model.TimeFromUnix(t0.Unix()), Value: 0.5}}},
		},
		memMatrix: model.Matrix{
			&model.SampleStream{Metric: metric, Values: []model.SamplePair{{Timestamp: model.TimeFromUnix(t0.Unix()), Value: 128 * 1024 * 1024}}},
		},
	})

	s := NewServer(":0", "", ServerDeps{
		BackfillEngine: engine,
		AuthConfig: &secauth.Config{
			ScopedKeys: []string{
				"rcx_viewer_viewertok",
				"rcx_admin_admintok",
			},
		},
	})

	body, _ := json.Marshal(map[string]string{
		"namespace": "prod",
		"from":      t0.Format(time.RFC3339),
		"to":        t1.Format(time.RFC3339),
	})

	// Viewer role must fail closed (403).
	viewerReq := httptest.NewRequest(http.MethodPost, "/api/v1/backfill", bytes.NewReader(body))
	viewerReq.Header.Set("Content-Type", "application/json")
	viewerReq.Header.Set("X-API-Key", "rcx_viewer_viewertok")
	viewerRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for RoleViewer on /api/v1/backfill, got %d", viewerRec.Code)
	}

	callAdmin := func() telemetry.BackfillReport {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/backfill", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", "rcx_admin_admintok")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for RoleAdmin on /api/v1/backfill, got %d: %s", rec.Code, rec.Body.String())
		}
		var rep telemetry.BackfillReport
		if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
			t.Fatalf("decode report: %v", err)
		}
		return rep
	}

	rep1 := callAdmin()
	if rep1.BucketsIngested != 1 || rep1.BucketsSkippedDuplicate != 0 {
		t.Fatalf("expected first backfill ingest=1 skip=0, got %+v", rep1)
	}

	rep2 := callAdmin()
	if rep2.BucketsIngested != 0 || rep2.BucketsSkippedDuplicate != 1 {
		t.Fatalf("expected duplicate replay ingest=0 skip=1, got %+v", rep2)
	}
}
