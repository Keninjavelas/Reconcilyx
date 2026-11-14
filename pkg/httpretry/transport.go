package httpretry

import (
	"bytes"
	"context"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/reconcilyx/operator/pkg/pipelinemetrics"
	"github.com/reconcilyx/operator/pkg/tracing"
)

const (
	DefaultBaseDelay   = 200 * time.Millisecond
	DefaultMaxDelay    = 5 * time.Second
	DefaultMaxAttempts = 3

	maxBufferedBodyBytes = 4 << 20 // 4 MiB replay buffer cap when GetBody is unset
)

// Transport wraps an http.RoundTripper with full-jitter exponential backoff
// on transient transport errors and retryable HTTP status codes (429, 502, 503, 504).
type Transport struct {
	Base        http.RoundTripper
	Component   string
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	MaxAttempts int

	// Optional test hooks; when nil production timer + math/rand/v2 are used.
	Sleep   func(context.Context, time.Duration) error
	JitterN func(n int64) int64
	Now     func() time.Time
}

// WrapClient returns a shallow copy (or new client if nil) whose Transport is
// wrapped with retry Transport without mutating http.DefaultClient or double-wrapping.
func WrapClient(client *http.Client, component string) *http.Client {
	if client == nil {
		return &http.Client{
			Timeout:   30 * time.Second,
			Transport: NewTransport(http.DefaultTransport, component),
		}
	}
	clone := *client
	if existing, ok := clone.Transport.(*Transport); ok {
		if existing.Component == component || component == "" {
			return &clone
		}
		// Keep existing base rather than nesting two retry loops.
		cp := *existing
		cp.Component = component
		clone.Transport = &cp
		return &clone
	}
	clone.Transport = NewTransport(clone.Transport, component)
	return &clone
}

// NewTransport creates a Transport with standard Reconcilyx retry defaults.
func NewTransport(base http.RoundTripper, component string) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	if component == "" {
		component = "http"
	}
	return &Transport{
		Base:        base,
		Component:   component,
		BaseDelay:   DefaultBaseDelay,
		MaxDelay:    DefaultMaxDelay,
		MaxAttempts: DefaultMaxAttempts,
	}
}

// RoundTrip implements http.RoundTripper with full-jitter exponential backoff.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	maxAttempts := t.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	baseDelay := t.BaseDelay
	if baseDelay <= 0 {
		baseDelay = DefaultBaseDelay
	}
	maxDelay := t.MaxDelay
	if maxDelay <= 0 {
		maxDelay = DefaultMaxDelay
	}
	component := t.Component
	if component == "" {
		component = "http"
	}

	if err := ensureReplayableBody(req); err != nil {
		return nil, err
	}
	tracing.InjectRequest(req)

	var lastResp *http.Response
	var lastErr error
	retryableMethod := isRetryableMethod(req)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if req.Context().Err() != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, req.Context().Err()
		}

		attemptReq, err := cloneForAttempt(req)
		if err != nil {
			return nil, err
		}

		resp, err := base.RoundTrip(attemptReq)
		lastResp = resp
		lastErr = err

		retry, statusLabel := shouldRetry(resp, err)
		if !retryableMethod || !retry || attempt == maxAttempts-1 {
			return resp, err
		}

		delay := t.computeDelay(attempt, baseDelay, maxDelay, resp)
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
		}

		pipelinemetrics.RetriesTotal.WithLabelValues(component, statusLabel).Inc()

		if sleepErr := t.sleepCtx(req.Context(), delay); sleepErr != nil {
			return nil, sleepErr
		}
	}

	return lastResp, lastErr
}

func ensureReplayableBody(req *http.Request) error {
	if req == nil || req.Body == nil || req.Body == http.NoBody || req.GetBody != nil {
		return nil
	}
	// Read up to maxBufferedBodyBytes+1 to avoid buffering unbounded streams.
	limited := io.LimitReader(req.Body, maxBufferedBodyBytes+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if int64(len(buf)) > maxBufferedBodyBytes {
		// Restore combined reader for single-shot transmission (not safely replayable).
		req.Body = struct {
			io.Reader
			io.Closer
		}{
			Reader: io.MultiReader(bytes.NewReader(buf), req.Body),
			Closer: req.Body,
		}
		return nil
	}
	_ = req.Body.Close()
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(buf)), nil
	}
	req.Body = io.NopCloser(bytes.NewReader(buf))
	return nil
}

func cloneForAttempt(req *http.Request) (*http.Request, error) {
	cloned := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		cloned.Body = body
	}
	return cloned, nil
}

func isRetryableMethod(req *http.Request) bool {
	if req == nil {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(req.Method)) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	case http.MethodPost:
		return strings.TrimSpace(req.Header.Get("X-Reconcilyx-Idempotency-Key")) != ""
	default:
		return false
	}
}

func shouldRetry(resp *http.Response, err error) (bool, string) {
	if err != nil {
		// Do not retry if the caller already canceled the context.
		if err == context.Canceled || err == context.DeadlineExceeded {
			return false, ""
		}
		return true, "transport_error"
	}
	if resp == nil {
		return false, ""
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true, strconv.Itoa(resp.StatusCode)
	default:
		return false, ""
	}
}

func (t *Transport) computeDelay(attempt int, baseDelay, maxDelay time.Duration, resp *http.Response) time.Duration {
	if resp != nil {
		if ra := strings.TrimSpace(resp.Header.Get("Retry-After")); ra != "" {
			if d, ok := t.parseRetryAfter(ra); ok {
				if d > maxDelay {
					return maxDelay
				}
				if d > 0 {
					return d
				}
			}
		}
	}

	exp := baseDelay
	for i := 0; i < attempt; i++ {
		if exp >= maxDelay/2 {
			exp = maxDelay
			break
		}
		exp *= 2
	}
	if exp > maxDelay {
		exp = maxDelay
	}
	if exp <= 0 {
		return 0
	}
	jitterFn := t.JitterN
	if jitterFn == nil {
		jitterFn = func(n int64) int64 { return rand.Int64N(n) }
	}
	return time.Duration(jitterFn(int64(exp) + 1))
}

func (t *Transport) parseRetryAfter(val string) (time.Duration, bool) {
	if secs, err := strconv.Atoi(val); err == nil {
		if secs <= 0 {
			return 0, true
		}
		return time.Duration(secs) * time.Second, true
	}
	if when, err := http.ParseTime(val); err == nil {
		now := time.Now()
		if t.Now != nil {
			now = t.Now()
		}
		d := when.Sub(now)
		if d < 0 {
			return 0, true
		}
		return d, true
	}
	return 0, false
}

func (t *Transport) sleepCtx(ctx context.Context, d time.Duration) error {
	if t.Sleep != nil {
		return t.Sleep(ctx, d)
	}
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
