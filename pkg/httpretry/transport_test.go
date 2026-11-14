package httpretry

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/reconcilyx/operator/pkg/pipelinemetrics"
)

func TestTransportRetries429RetryAfterAnd503RecoveringWithPOSTBody(t *testing.T) {
	var attempts atomic.Int32
	var receivedBodies []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))
		n := attempts.Add(1)
		switch n {
		case 1:
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer ts.Close()

	var recordedDelays []time.Duration
	tr := &Transport{
		Base:        http.DefaultTransport,
		Component:   "test_recover",
		BaseDelay:   200 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		MaxAttempts: 3,
		JitterN:     func(n int64) int64 { return n - 1 },
		Sleep: func(_ context.Context, d time.Duration) error {
			recordedDelays = append(recordedDelays, d)
			return nil
		},
	}

	before429 := testutil.ToFloat64(pipelinemetrics.RetriesTotal.WithLabelValues("test_recover", "429"))
	before503 := testutil.ToFloat64(pipelinemetrics.RetriesTotal.WithLabelValues("test_recover", "503"))

	client := &http.Client{Transport: tr}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL, bytes.NewBufferString(`{"batch":42}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Reconcilyx-Idempotency-Key", "batch-42")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("expected retry recovery, got err: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	if attempts.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts.Load())
	}
	for i, b := range receivedBodies {
		if b != `{"batch":42}` {
			t.Fatalf("attempt %d lost request body: %q", i+1, b)
		}
	}
	if len(recordedDelays) != 2 {
		t.Fatalf("expected 2 retry sleeps, got %d", len(recordedDelays))
	}
	if recordedDelays[0] != 2*time.Second {
		t.Fatalf("expected first Retry-After delay 2s, got %v", recordedDelays[0])
	}
	if recordedDelays[1] != 400*time.Millisecond {
		t.Fatalf("expected second full-jitter max delay 400ms, got %v", recordedDelays[1])
	}

	after429 := testutil.ToFloat64(pipelinemetrics.RetriesTotal.WithLabelValues("test_recover", "429"))
	after503 := testutil.ToFloat64(pipelinemetrics.RetriesTotal.WithLabelValues("test_recover", "503"))
	if after429-before429 != 1 || after503-before503 != 1 {
		t.Fatalf("expected retry metrics increment (+1 429, +1 503), got (+%.0f, +%.0f)", after429-before429, after503-before503)
	}
}

func TestTransportDoesNotRetryClientErrorsAndHonorsRetryAfterHTTPDate(t *testing.T) {
	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	client := WrapClient(&http.Client{}, "test_no_retry")
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if attempts.Load() != 1 {
		t.Fatalf("400 Bad Request must not retry, attempts=%d", attempts.Load())
	}

	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	tr := &Transport{
		Now:      func() time.Time { return now },
		BaseDelay: 200 * time.Millisecond,
		MaxDelay:  5 * time.Second,
	}
	respDate := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{now.Add(3 * time.Second).Format(http.TimeFormat)}},
	}
	if d := tr.computeDelay(0, 200*time.Millisecond, 5*time.Second, respDate); d != 3*time.Second {
		t.Fatalf("expected HTTP-date Retry-After 3s, got %v", d)
	}
	respClamped := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"60"}},
	}
	if d := tr.computeDelay(0, 200*time.Millisecond, 5*time.Second, respClamped); d != 5*time.Second {
		t.Fatalf("expected clamped Retry-After 5s, got %v", d)
	}
}

type failingRoundTripper struct {
	calls atomic.Int32
}

func (f *failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	n := f.calls.Add(1)
	if n < 3 {
		return nil, errors.New("dial tcp: i/o timeout")
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nil))}, nil
}

func TestTransportRetriesTransientNetworkError(t *testing.T) {
	rt := &failingRoundTripper{}
	tr := &Transport{
		Base:        rt,
		Component:   "test_net",
		MaxAttempts: 3,
		BaseDelay:   time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
		Sleep:       func(context.Context, time.Duration) error { return nil },
	}
	before := testutil.ToFloat64(pipelinemetrics.RetriesTotal.WithLabelValues("test_net", "transport_error"))
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.invalid", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("expected recovery after network errors, got %v", err)
	}
	resp.Body.Close()
	if rt.calls.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", rt.calls.Load())
	}
	after := testutil.ToFloat64(pipelinemetrics.RetriesTotal.WithLabelValues("test_net", "transport_error"))
	if after-before != 2 {
		t.Fatalf("expected 2 transport_error retry metrics, got %.0f", after-before)
	}
}

func TestTransportDoesNotRetryPOSTWithoutIdempotencyKey(t *testing.T) {
	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	tr := &Transport{
		Base:        http.DefaultTransport,
		Component:   "test_nonidempotent_post",
		BaseDelay:   time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
		MaxAttempts: 3,
		Sleep:       func(context.Context, time.Duration) error { return nil },
	}
	client := &http.Client{Transport: tr}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL, bytes.NewBufferString(`{"mutate":true}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 passthrough, got %d", resp.StatusCode)
	}
	if attempts.Load() != 1 {
		t.Fatalf("POST without X-Reconcilyx-Idempotency-Key must not retry, attempts=%d", attempts.Load())
	}
}
