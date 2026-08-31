package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

// NOTE (v5.0 planned): The EBPFMetricsStreamer is currently a STUB.
// Real eBPF kernel tracing (via Cilium / Tetragon / Pixie gRPC) is
// scheduled for Reconcilyx v5.0. For v4.0-beta, workload CPU and
// memory telemetry comes from Prometheus (see PrometheusMetricsClient
// and WorkloadInspector). Callers that want real sub-second samples
// should enable a v5.0 flag: `RECONCILYX_V5_EBPF_ENABLED=1`.
var ebpfStubWarned atomic.Int32

func ebpfWarnOnce(label string) {
	if ebpfStubWarned.Add(1) == 1 {
		l := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
		l.Warn(fmt.Sprintf("[ebpf v5.0 stub] %s returning synthetic samples; real Cilium/Pixie gRPC streaming is scheduled for v5.0. Set RECONCILYX_V5_EBPF_ENABLED=1 to opt in to the real path once available.", label))
	}
}

type EBPFSample struct {
	PodName        string    `json:"podName"`
	Namespace      string    `json:"namespace"`
	KernelCPUNanos uint64    `json:"kernelCpuNanos"`
	SocketRxBytes  uint64    `json:"socketRxBytes"`
	SocketTxBytes  uint64    `json:"socketTxBytes"`
	PageFaultCount uint64    `json:"pageFaultCount"`
	Timestamp      time.Time `json:"timestamp"`
}

type EBPFMetricsStreamer struct{}

func NewEBPFMetricsStreamer() *EBPFMetricsStreamer {
	return &EBPFMetricsStreamer{}
}

func (e *EBPFMetricsStreamer) StreamRealtimeMetrics(ctx context.Context, podName, namespace string) (<-chan EBPFSample, error) {
	ebpfWarnOnce(fmt.Sprintf("StreamRealtimeMetrics(%s/%s)", namespace, podName))
	out := make(chan EBPFSample, 100)

	go func() {
		defer close(out)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				sample := EBPFSample{
					PodName:        podName,
					Namespace:      namespace,
					KernelCPUNanos: 4500000,
					SocketRxBytes:  10240,
					SocketTxBytes:  8192,
					PageFaultCount: 0,
					Timestamp:      t,
				}
				select {
				case out <- sample:
				default:
				}
			}
		}
	}()

	return out, nil
}
