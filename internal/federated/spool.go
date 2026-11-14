package federated

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/reconcilyx/operator/pkg/pipelinemetrics"
)

// DefaultMaxSpoolBytes caps local spoke WAL disk usage at 50 MiB.
const DefaultMaxSpoolBytes int64 = 50 << 20

// DefaultSpoolDir returns the OS temporary directory path used by spoke agents
// for durable federated batch spooling (/tmp/reconcilyx-spool on Linux).
func DefaultSpoolDir() string {
	return filepath.Join(os.TempDir(), "reconcilyx-spool")
}

// ComputeBatchID returns a deterministic SHA-256 idempotency key for a DeltaBatch
// derived from clusterName, timestamp bounds, and sorted canonical delta signatures.
func ComputeBatchID(clusterName string, deltas []QTableDelta) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "cluster=%s\n", strings.TrimSpace(clusterName))
	if len(deltas) == 0 {
		_, _ = h.Write([]byte("empty"))
		return hex.EncodeToString(h.Sum(nil))
	}

	var minTS, maxTS int64
	parts := make([]string, 0, len(deltas))
	for _, d := range deltas {
		ts := d.Timestamp.UTC().UnixNano()
		if minTS == 0 || (ts > 0 && ts < minTS) {
			minTS = ts
		}
		if ts > maxTS {
			maxTS = ts
		}
		parts = append(parts, fmt.Sprintf("%s|%s|%d|%d|%d|%s|%d",
			d.ClusterName,
			EntryKey(d.StateKey, d.ActionKey),
			d.Epoch,
			ts,
			d.VisitCount,
			strconv.FormatFloat(d.DeltaQ, 'g', -1, 64),
			d.StateKey.CPUBucket,
		))
	}
	sort.Strings(parts)
	_, _ = fmt.Fprintf(h, "min=%d|max=%d\n", minTS, maxTS)
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// SpooledBatch represents one persisted batch ready for FIFO replay.
type SpooledBatch struct {
	Path  string
	Batch DeltaBatch
}

// DiskSpool persists failed spoke DeltaBatch payloads in bounded FIFO JSON files
// so Hub outages do not lose federated RL telemetry.
type DiskSpool struct {
	dir      string
	maxBytes int64
	mu       sync.Mutex
	seq      atomic.Uint64
}

// NewDiskSpool initializes a bounded disk spool inside dir.
func NewDiskSpool(dir string, maxBytes int64) (*DiskSpool, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("federated spool: empty directory")
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxSpoolBytes
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("federated spool mkdir %s: %w", dir, err)
	}
	return &DiskSpool{
		dir:      dir,
		maxBytes: maxBytes,
	}, nil
}

// Enqueue atomically writes a DeltaBatch to the bounded FIFO disk spool.
func (s *DiskSpool) Enqueue(batch DeltaBatch) error {
	if s == nil {
		return nil
	}
	if batch.BatchID == "" {
		batch.BatchID = ComputeBatchID(batch.ClusterName, batch.Deltas)
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	need := int64(len(payload))
	if need > s.maxBytes {
		return fmt.Errorf("federated spool: batch size %d exceeds maxBytes %d", need, s.maxBytes)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return err
	}

	files, totalBytes, err := s.scanLocked()
	if err != nil {
		return err
	}
	// Evict oldest FIFO entries to enforce maxBytes ceiling without exceeding cap.
	for totalBytes+need > s.maxBytes && len(files) > 0 {
		oldest := files[0]
		_ = os.Remove(oldest.path)
		totalBytes -= oldest.size
		files = files[1:]
	}

	idPrefix := batch.BatchID
	if len(idPrefix) > 12 {
		idPrefix = idPrefix[:12]
	}
	seq := s.seq.Add(1)
	name := fmt.Sprintf("%020d-%06d-%s.json", time.Now().UTC().UnixNano(), seq, idPrefix)
	finalPath := filepath.Join(s.dir, name)
	tmpPath := finalPath + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	pipelinemetrics.SpokeSpoolQueuedBatches.WithLabelValues(clusterLabel(batch.ClusterName)).Set(float64(len(files) + 1))
	return nil
}

// List returns persisted batches ordered oldest-first (FIFO).
func (s *DiskSpool) List() ([]SpooledBatch, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	files, _, err := s.scanLocked()
	if err != nil {
		return nil, err
	}
	out := make([]SpooledBatch, 0, len(files))
	for _, f := range files {
		raw, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}
		var b DeltaBatch
		if err := json.Unmarshal(raw, &b); err != nil {
			// Discard corrupted file rather than blocking FIFO drain permanently.
			_ = os.Remove(f.path)
			continue
		}
		if b.BatchID == "" {
			b.BatchID = ComputeBatchID(b.ClusterName, b.Deltas)
		}
		out = append(out, SpooledBatch{Path: f.path, Batch: b})
	}
	return out, nil
}

// Ack deletes a successfully transmitted spooled batch file.
func (s *DiskSpool) Ack(path, clusterName string) error {
	if s == nil || path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	files, _, _ := s.scanLocked()
	pipelinemetrics.SpokeSpoolQueuedBatches.WithLabelValues(clusterLabel(clusterName)).Set(float64(len(files)))
	return nil
}

// Count returns the current number of queued spool batch files.
func (s *DiskSpool) Count() (int, error) {
	if s == nil {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	files, _, err := s.scanLocked()
	return len(files), err
}

type spoolFileInfo struct {
	path string
	name string
	size int64
}

func (s *DiskSpool) scanLocked() ([]spoolFileInfo, int64, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	files := make([]spoolFileInfo, 0, len(entries))
	var total int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, spoolFileInfo{
			path: filepath.Join(s.dir, e.Name()),
			name: e.Name(),
			size: info.Size(),
		})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].name < files[j].name
	})
	return files, total, nil
}

func clusterLabel(cluster string) string {
	if strings.TrimSpace(cluster) == "" {
		return "unknown"
	}
	return cluster
}
