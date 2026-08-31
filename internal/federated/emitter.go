package federated

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

type DeltaObserver func(delta QTableDelta)

type EmitterConfig struct {
	HubBaseURL       string
	ClusterName      string
	APIKey           string
	BufferCapacity   int
	FlushInterval    time.Duration
	FlushThreshold   int
	HTTPClient       *http.Client
}

func (c EmitterConfig) withDefaults() EmitterConfig {
	if c.BufferCapacity <= 0 {
		c.BufferCapacity = 5000
	}
	if c.FlushInterval == 0 {
		c.FlushInterval = time.Hour
	}
	if c.FlushThreshold <= 0 {
		c.FlushThreshold = 100
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return c
}

type Emitter struct {
	cfg      EmitterConfig
	mu       sync.Mutex
	buffer   []QTableDelta
	epoch    int64
	observer DeltaObserver
}

func NewEmitter(cfg EmitterConfig) *Emitter {
	cfg = cfg.withDefaults()
	return &Emitter{
		cfg:    cfg,
		buffer: make([]QTableDelta, 0, cfg.BufferCapacity),
	}
}

func (e *Emitter) SetObserver(fn DeltaObserver) { e.observer = fn }

func (e *Emitter) Emit(delta QTableDelta) int {
	e.mu.Lock()
	delta.ClusterName = e.cfg.ClusterName
	delta.Timestamp = time.Now().UTC()
	if delta.Epoch == 0 {
		e.epoch++
		delta.Epoch = e.epoch
	}
	if e.observer != nil {
		e.observer(delta)
	}
	e.buffer = append(e.buffer, delta)
	shouldFlush := len(e.buffer) >= e.cfg.FlushThreshold
	n := len(e.buffer)
	e.mu.Unlock()
	if shouldFlush {
		_ = e.Flush(context.Background())
	}
	return n
}

func (e *Emitter) takeBatch() []QTableDelta {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.buffer) == 0 {
		return nil
	}
	b := e.buffer
	e.buffer = make([]QTableDelta, 0, e.cfg.BufferCapacity)
	e.epoch++
	return b
}

func (e *Emitter) Flush(ctx context.Context) error {
	batch := e.takeBatch()
	if len(batch) == 0 {
		return nil
	}
	return e.sendBatch(ctx, batch)
}

func (e *Emitter) sendBatch(ctx context.Context, deltas []QTableDelta) error {
	if e.cfg.HubBaseURL == "" {
		return errors.New("federated: HubBaseURL is not configured; dropping batch silently (size=" + itoa(len(deltas)) + ")")
	}
	body := DeltaBatch{
		ClusterName: e.cfg.ClusterName,
		Epoch:       e.epoch,
		Deltas:      deltas,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.HubBaseURL+"/api/federated/deltas", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.cfg.APIKey != "" {
		req.Header.Set("X-API-Key", e.cfg.APIKey)
	}
	resp, err := e.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("federated: hub rejected batch, status=" + resp.Status)
	}
	return nil
}

func (e *Emitter) Run(ctx context.Context) error {
	t := time.NewTicker(e.cfg.FlushInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = e.Flush(flushCtx)
			return nil
		case <-t.C:
			_ = e.Flush(ctx)
		}
	}
}

func (e *Emitter) BufferedCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.buffer)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
