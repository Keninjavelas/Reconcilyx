package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reconcilyx/operator/pkg/tracing"
)

func TestTraceparentPropagationAndLogging(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	var seenTrace tracing.TraceContext
	h := RequestIDMiddleware(LoggingMiddleware(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc, ok := tracing.FromContext(r.Context())
		if !ok {
			t.Fatalf("expected TraceContext in request context")
		}
		seenTrace = tc
		w.WriteHeader(http.StatusNoContent)
	})))

	// Case 1: Fresh request without headers generates valid W3C traceparent + X-Request-ID.
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec1.Code)
	}
	tp1 := rec1.Header().Get(tracing.HeaderTraceparent)
	tid1, sid1, flags1, ok := tracing.ParseTraceparent(tp1)
	if !ok {
		t.Fatalf("expected valid W3C traceparent response header, got %q", tp1)
	}
	if tid1 != seenTrace.TraceID || sid1 != seenTrace.SpanID || flags1 != "01" {
		t.Fatalf("trace header mismatch: header=(%s,%s,%s) ctx=%+v", tid1, sid1, flags1, seenTrace)
	}
	if rec1.Header().Get(tracing.HeaderRequestID) == "" {
		t.Fatalf("expected X-Request-ID header")
	}

	// Case 2: Incoming W3C traceparent preserves trace_id while minting child span_id.
	logBuf.Reset()
	inTraceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	inParentSpan := "00f067aa0ba902b7"
	req2 := httptest.NewRequest(http.MethodGet, "/api/proposals", nil)
	req2.Header.Set(tracing.HeaderTraceparent, "00-"+inTraceID+"-"+inParentSpan+"-01")
	req2.Header.Set(tracing.HeaderRequestID, "corr-123")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	tp2 := rec2.Header().Get(tracing.HeaderTraceparent)
	tid2, sid2, _, ok := tracing.ParseTraceparent(tp2)
	if !ok {
		t.Fatalf("expected valid child traceparent, got %q", tp2)
	}
	if tid2 != inTraceID {
		t.Fatalf("expected trace_id %q to be preserved, got %q", inTraceID, tid2)
	}
	if sid2 == inParentSpan {
		t.Fatalf("expected fresh child span_id different from parent %q", inParentSpan)
	}
	if seenTrace.ParentSpanID != inParentSpan {
		t.Fatalf("expected parent_span_id=%q, got %q", inParentSpan, seenTrace.ParentSpanID)
	}
	if rec2.Header().Get(tracing.HeaderRequestID) != "corr-123" {
		t.Fatalf("expected preserved X-Request-ID corr-123, got %q", rec2.Header().Get(tracing.HeaderRequestID))
	}

	var logEntry map[string]any
	line := strings.TrimSpace(logBuf.String())
	if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
		t.Fatalf("expected JSON slog output, got %q: %v", line, err)
	}
	if logEntry["trace_id"] != inTraceID || logEntry["span_id"] != sid2 || logEntry["parent_span_id"] != inParentSpan {
		t.Fatalf("expected slog trace fields trace_id=%s span_id=%s parent_span_id=%s, got %+v", inTraceID, sid2, inParentSpan, logEntry)
	}
}
