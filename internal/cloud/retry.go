package cloud

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/reconcilyx/operator/pkg/pipelinemetrics"
)

// RetryPolicy controls retries for idempotent live pricing requests.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func (p RetryPolicy) withDefaults() RetryPolicy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = 200 * time.Millisecond
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = 5 * time.Second
	}
	return p
}

func (p RetryPolicy) delay(attempt int) time.Duration {
	d := p.BaseDelay << uint(attempt-1)
	if d > p.MaxDelay || d <= 0 {
		d = p.MaxDelay
	}
	if d <= 0 {
		return 0
	}
	// Full jitter: uniform [0, exp].
	return time.Duration(rand.Int64N(int64(d) + 1))
}

// doWithRetry issues requests built by newRequest and retries network errors,
// 429 and 5xx responses, and AWS 400 throttling responses with exponential
// backoff. The request is rebuilt per attempt so signed bodies stay valid.
func doWithRetry(ctx context.Context, client *http.Client, newRequest func() (*http.Request, error), policy RetryPolicy) (*http.Response, error) {
	policy = policy.withDefaults()
	if client == nil {
		return nil, fmt.Errorf("pricing: HTTP client is nil")
	}

	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		req, err := newRequest()
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt == policy.MaxAttempts {
				break
			}
			pipelinemetrics.RetriesTotal.WithLabelValues("cloud_pricing", "transport_error").Inc()
			if err := sleepContext(ctx, policy.delay(attempt)); err != nil {
				return nil, err
			}
			continue
		}

		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			// AWS reports throttling as a 400 with a ThrottlingException body.
			if resp.StatusCode == http.StatusBadRequest {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
				resp.Body.Close()
				resp.Body = io.NopCloser(bytes.NewReader(body))
				if !bytes.Contains(body, []byte("Throttling")) {
					return resp, nil
				}
			} else {
				return resp, nil
			}
		}

		lastErr = fmt.Errorf("pricing api status %d", resp.StatusCode)
		delay := retryAfter(resp, policy, attempt)
		statusCode := resp.StatusCode
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		if attempt == policy.MaxAttempts {
			break
		}
		pipelinemetrics.RetriesTotal.WithLabelValues("cloud_pricing", strconv.Itoa(statusCode)).Inc()
		if err := sleepContext(ctx, delay); err != nil {
			return nil, err
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("pricing api request failed")
	}
	return nil, lastErr
}

func retryAfter(resp *http.Response, policy RetryPolicy, attempt int) time.Duration {
	value := resp.Header.Get("Retry-After")
	if value == "" {
		return policy.delay(attempt)
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		d := time.Duration(seconds) * time.Second
		if d > policy.MaxDelay {
			return policy.MaxDelay
		}
		return d
	}
	if when, err := http.ParseTime(value); err == nil {
		d := time.Until(when)
		if d < 0 {
			return 0
		}
		if d > policy.MaxDelay {
			return policy.MaxDelay
		}
		return d
	}
	return policy.delay(attempt)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
