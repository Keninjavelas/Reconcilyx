package telemetry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/reconcilyx/operator/pkg/pipelinemetrics"
)

// DefaultBackfillStep defines the fixed PromQL step used for historical gap recovery.
const DefaultBackfillStep = 5 * time.Minute

// UtilizationBucket stores one reconstructed telemetry bucket keyed idempotently
// by (WorkloadUID, WindowStartUnix).
type UtilizationBucket struct {
	WorkloadUID     string  `json:"workloadUid"`
	Namespace       string  `json:"namespace"`
	Workload        string  `json:"workload"`
	WindowStartUnix int64   `json:"windowStartUnix"`
	WindowEndUnix   int64   `json:"windowEndUnix"`
	CPUCores        float64 `json:"cpuCores"`
	MemoryBytes     float64 `json:"memoryBytes"`
}

// BackfillRequest parameterizes a watermarked historical telemetry recovery run.
type BackfillRequest struct {
	Namespace string        `json:"namespace,omitempty"`
	From      time.Time     `json:"from"`
	To        time.Time     `json:"to"`
	Step      time.Duration `json:"step,omitempty"`
}

// BackfillReport summarizes buckets ingested vs idempotently skipped.
type BackfillReport struct {
	Namespace               string    `json:"namespace"`
	From                    time.Time `json:"from"`
	To                      time.Time `json:"to"`
	Step                    string    `json:"step"`
	WindowsEvaluated        int       `json:"windowsEvaluated"`
	BucketsIngested         int       `json:"bucketsIngested"`
	BucketsSkippedDuplicate int       `json:"bucketsSkippedDuplicate"`
	NewWatermark            time.Time `json:"newWatermark"`
}

// BackfillEngine tracks per-namespace evaluation watermarks and replays historical
// PromQL range windows without double-counting previously processed buckets.
type BackfillEngine struct {
	mu             sync.RWMutex
	client         WindowedMetricsClient
	step           time.Duration
	watermarks     map[string]time.Time
	processed      map[string]UtilizationBucket
	processedOrder []string
	maxProcessed   int
}

// NewBackfillEngine constructs a watermarked backfill engine around a WindowedMetricsClient.
func NewBackfillEngine(client WindowedMetricsClient) *BackfillEngine {
	return &BackfillEngine{
		client:         client,
		step:           DefaultBackfillStep,
		watermarks:     make(map[string]time.Time),
		processed:      make(map[string]UtilizationBucket),
		processedOrder: make([]string, 0, 1024),
		maxProcessed:   100000,
	}
}

// RecordEvaluation advances the lastEvaluatedTimestamp watermark monotonically for namespace.
func (e *BackfillEngine) RecordEvaluation(namespace string, ts time.Time) {
	if e == nil || ts.IsZero() {
		return
	}
	ns := normalizeNamespace(namespace)
	e.mu.Lock()
	defer e.mu.Unlock()
	if cur, ok := e.watermarks[ns]; !ok || ts.After(cur) {
		e.watermarks[ns] = ts.UTC()
	}
}

// GetWatermark returns the lastEvaluatedTimestamp recorded for namespace.
func (e *BackfillEngine) GetWatermark(namespace string) (time.Time, bool) {
	if e == nil {
		return time.Time{}, false
	}
	ns := normalizeNamespace(namespace)
	e.mu.RLock()
	defer e.mu.RUnlock()
	wm, ok := e.watermarks[ns]
	return wm, ok
}

// NeedsBackfill returns true when namespace has a recorded watermark whose gap to now
// exceeds 2 * scrapeInterval.
func (e *BackfillEngine) NeedsBackfill(namespace string, now time.Time, scrapeInterval time.Duration) bool {
	if e == nil || scrapeInterval <= 0 {
		return false
	}
	wm, ok := e.GetWatermark(namespace)
	if !ok {
		return false
	}
	return now.Sub(wm) > 2*scrapeInterval
}

// Backfill steps [req.From, req.To] in 5m windows, reconstructing utilization buckets
// idempotently by (workloadUID, windowStartUnix).
func (e *BackfillEngine) Backfill(ctx context.Context, req BackfillRequest) (*BackfillReport, error) {
	if e == nil || e.client == nil {
		return nil, errors.New("backfill engine or telemetry range client is not initialized")
	}
	from := req.From.UTC()
	to := req.To.UTC()
	if from.IsZero() || to.IsZero() || !from.Before(to) {
		return nil, fmt.Errorf("invalid backfill time range: from (%s) must be before to (%s)", from, to)
	}
	step := req.Step
	if step <= 0 {
		step = e.step
	}
	if step <= 0 {
		step = DefaultBackfillStep
	}
	ns := normalizeNamespace(req.Namespace)
	nsLabel := ns
	if nsLabel == "" {
		nsLabel = "all"
	}

	cpuQuery := buildRangeCPUQuery(ns)
	memQuery := buildRangeMemQuery(ns)

	// Step through [from, to] in deterministic 5m sub-windows (up to 1h query slices
	// containing multiple 5m points so Prometheus evaluates cleanly).
	chunkDuration := step * 12 // 1h chunks of 5m steps
	var windowsEvaluated, ingested, skipped int

	for cursor := from; cursor.Before(to); {
		chunkEnd := cursor.Add(chunkDuration)
		if chunkEnd.After(to) {
			chunkEnd = to
		}

		cpuVal, err := e.client.QueryRangeWindow(ctx, cpuQuery, cursor, chunkEnd, step)
		if err != nil && !errors.Is(err, ErrNoData) {
			return nil, fmt.Errorf("backfill cpu range query failed (%s..%s): %w", cursor.Format(time.RFC3339), chunkEnd.Format(time.RFC3339), err)
		}
		memVal, err := e.client.QueryRangeWindow(ctx, memQuery, cursor, chunkEnd, step)
		if err != nil && !errors.Is(err, ErrNoData) {
			return nil, fmt.Errorf("backfill memory range query failed (%s..%s): %w", cursor.Format(time.RFC3339), chunkEnd.Format(time.RFC3339), err)
		}

		merged := mergeRangeSamples(ns, step, cpuVal, memVal)
		windowsEvaluated += countWindowSteps(cursor, chunkEnd, step)

		e.mu.Lock()
		for _, b := range merged {
			key := bucketKey(b.WorkloadUID, b.WindowStartUnix)
			if _, exists := e.processed[key]; exists {
				skipped++
				pipelinemetrics.BackfillWindowsTotal.WithLabelValues(nsLabel, "skipped_duplicate").Inc()
				pipelinemetrics.BackfillDedupedTotal.WithLabelValues(nsLabel).Inc()
				continue
			}
			if e.maxProcessed > 0 && len(e.processedOrder) >= e.maxProcessed {
				evict := e.processedOrder[0]
				e.processedOrder = e.processedOrder[1:]
				delete(e.processed, evict)
			}
			e.processed[key] = b
			e.processedOrder = append(e.processedOrder, key)
			ingested++
			pipelinemetrics.BackfillWindowsTotal.WithLabelValues(nsLabel, "ingested").Inc()
		}
		e.mu.Unlock()

		cursor = chunkEnd
	}

	e.RecordEvaluation(ns, to)
	wm, _ := e.GetWatermark(ns)

	return &BackfillReport{
		Namespace:               nsLabel,
		From:                    from,
		To:                      to,
		Step:                    step.String(),
		WindowsEvaluated:        windowsEvaluated,
		BucketsIngested:         ingested,
		BucketsSkippedDuplicate: skipped,
		NewWatermark:            wm,
	}, nil
}

// BucketCount returns the total number of deduplicated utilization buckets stored.
func (e *BackfillEngine) BucketCount() int {
	if e == nil {
		return 0
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.processed)
}

// ListBuckets returns all stored UtilizationBuckets ordered deterministically.
func (e *BackfillEngine) ListBuckets(namespace string) []UtilizationBucket {
	if e == nil {
		return nil
	}
	ns := normalizeNamespace(namespace)
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]UtilizationBucket, 0, len(e.processed))
	for _, b := range e.processed {
		if ns != "" && b.Namespace != ns {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].WindowStartUnix != out[j].WindowStartUnix {
			return out[i].WindowStartUnix < out[j].WindowStartUnix
		}
		return out[i].WorkloadUID < out[j].WorkloadUID
	})
	return out
}

func normalizeNamespace(ns string) string {
	trimmed := strings.TrimSpace(ns)
	if trimmed == "*" || strings.EqualFold(trimmed, "all") {
		return ""
	}
	return trimmed
}

func bucketKey(workloadUID string, windowStartUnix int64) string {
	sum := sha256.Sum256([]byte(workloadUID + "|" + strconv.FormatInt(windowStartUnix, 10)))
	return hex.EncodeToString(sum[:])
}

func countWindowSteps(start, end time.Time, step time.Duration) int {
	if !start.Before(end) || step <= 0 {
		return 0
	}
	n := int(end.Sub(start) / step)
	if end.Sub(start)%step != 0 {
		n++
	}
	if n < 1 {
		n = 1
	}
	return n
}

func buildRangeCPUQuery(namespace string) string {
	if namespace != "" {
		return fmt.Sprintf(`max(rate(container_cpu_usage_seconds_total{namespace=%q, container!="POD", container!=""}[5m])) by (namespace, pod, uid)`, namespace)
	}
	return `max(rate(container_cpu_usage_seconds_total{container!="POD", container!=""}[5m])) by (namespace, pod, uid)`
}

func buildRangeMemQuery(namespace string) string {
	if namespace != "" {
		return fmt.Sprintf(`max(container_memory_working_set_bytes{namespace=%q, container!="POD", container!=""}) by (namespace, pod, uid)`, namespace)
	}
	return `max(container_memory_working_set_bytes{container!="POD", container!=""}) by (namespace, pod, uid)`
}

func mergeRangeSamples(defaultNS string, step time.Duration, cpuVal, memVal model.Value) []UtilizationBucket {
	buckets := make(map[string]*UtilizationBucket)
	addValue := func(val model.Value, isCPU bool) {
		if val == nil {
			return
		}
		switch v := val.(type) {
		case model.Matrix:
			for _, stream := range v {
				uid, ns, wl := extractStreamIdentity(stream.Metric, defaultNS)
				for _, pair := range stream.Values {
					f := float64(pair.Value)
					if math.IsNaN(f) || math.IsInf(f, 0) {
						continue
					}
					ts := pair.Timestamp.Time().UTC()
					start := ts.Truncate(step).Unix()
					key := bucketKey(uid, start)
					b, ok := buckets[key]
					if !ok {
						b = &UtilizationBucket{
							WorkloadUID:     uid,
							Namespace:       ns,
							Workload:        wl,
							WindowStartUnix: start,
							WindowEndUnix:   start + int64(step.Seconds()),
						}
						buckets[key] = b
					}
					if isCPU {
						b.CPUCores = f
					} else {
						b.MemoryBytes = f
					}
				}
			}
		case model.Vector:
			for _, sample := range v {
				uid, ns, wl := extractStreamIdentity(sample.Metric, defaultNS)
				f := float64(sample.Value)
				if math.IsNaN(f) || math.IsInf(f, 0) {
					continue
				}
				start := sample.Timestamp.Time().UTC().Truncate(step).Unix()
				key := bucketKey(uid, start)
				b, ok := buckets[key]
				if !ok {
					b = &UtilizationBucket{
						WorkloadUID:     uid,
						Namespace:       ns,
						Workload:        wl,
						WindowStartUnix: start,
						WindowEndUnix:   start + int64(step.Seconds()),
					}
					buckets[key] = b
				}
				if isCPU {
					b.CPUCores = f
				} else {
					b.MemoryBytes = f
				}
			}
		}
	}

	addValue(cpuVal, true)
	addValue(memVal, false)

	out := make([]UtilizationBucket, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].WindowStartUnix != out[j].WindowStartUnix {
			return out[i].WindowStartUnix < out[j].WindowStartUnix
		}
		return out[i].WorkloadUID < out[j].WorkloadUID
	})
	return out
}

func extractStreamIdentity(m model.Metric, defaultNS string) (uid, ns, workload string) {
	ns = string(m["namespace"])
	if ns == "" {
		ns = defaultNS
	}
	if ns == "" {
		ns = "default"
	}
	for _, label := range []model.LabelName{"workload", "deployment", "pod", "container"} {
		if v := string(m[label]); v != "" {
			workload = v
			break
		}
	}
	if workload == "" {
		workload = "unknown"
	}
	uid = string(m["uid"])
	if uid == "" {
		uid = ns + "/" + workload
	}
	return uid, ns, workload
}
