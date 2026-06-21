# Engine Design (Phase 1)

## 1. Engine Responsibilities

The Optimization Engine is the intellectual core of Reconcilyx. Its mandate is strictly analytical.

**What the engine owns:**
- Periodically evaluating watched workloads via Telemetry.
- Applying predefined, pluggable optimization rules.
- Calculating quantitative Confidence and Risk scores based on historical data.
- Estimating monetary impact of optimizations.
- Generating and updating `OptimizationProposal` Custom Resources.

**What it must never own:**
- Modifying actual target workloads (e.g., patching Deployments/StatefulSets).
- Mutating its own inputs (e.g., deleting/resetting telemetry data).
- Approving proposals (that is the domain of Policy/Humans).

**Boundaries:**
- **Telemetry Boundary:** The Engine only consumes data through the `TelemetryProvider` interface. It has zero knowledge of Prometheus or PromQL.
- **Controller Boundary:** The Engine does not watch Deployments natively to reconcile them; it is triggered by an overarching operator loop.
- **Policy Boundary:** The Engine does not care *when* a proposal is applied or *if* an environment is locked down. It only cares about generating the mathematically correct recommendation.

## 2. Analysis Pipeline

The end-to-end pipeline operates as follows:

1. **Discovery:** The controller loop triggers the Engine for a specific `corev1.ObjectReference`.
2. **Telemetry Collection:** The Engine calls `TelemetryProvider.GetWorkloadMetrics(ctx, ref)`.
3. **Container Iteration:** The Engine iterates over every `ContainerMetrics` entry in the `WorkloadMetrics.Containers` map.
4. **Rule Evaluation:** Each container is passed through the `RuleRegistry`. Rules run independently.
5. **Finding Aggregation:** Violations are collected as `Findings`.
6. **Scoring:** The Engine evaluates the aggregate telemetry to calculate a `ConfidenceScore` and `RiskScore`.
7. **Impact Calculation:** Proposed deltas are calculated against arbitrary static cost metrics.
8. **Proposal Generation:** An `OptimizationProposal` is generated with `RecommendationType`, `ProposedResources`, and `DecisionMetadata`.

## 3. Rule Framework

The `Rule` interface defines a pluggable strategy:

```go
type Rule interface {
    Name() string
    Description() string
    Evaluate(container string, metrics *telemetry.ContainerMetrics) (*Finding, error)
}
```

### Initial Rules:
- **CPU Overprovisioned:** Detects if `RequestedCores > (UsageCores * 1.2)`. Recommends a 20% buffer over peak usage.
- **Memory Overprovisioned:** Detects if `RequestedBytes > (WorkingSetBytes * 1.2)`. Recommends a 20% buffer over peak working set.
- **Idle Workload:** Detects if `UsageCores < 0.01` and `WorkingSetBytes < 50MB`. Recommends a global `ScaleToZero`.

## 4. Scoring Framework

Scoring allows platform engineers (and policies) to filter out mathematically correct but operationally dangerous recommendations.

- **Confidence Score (0-100):** Starts at 100.
  - `-50` if telemetry is partially missing (e.g., `ErrPartialTelemetry`).
  - `-20` if `Restarts > 0` (volatile workload).
- **Risk Score (0-100):** Starts at 0.
  - `+50` if `ThrottledSecs > 0` (actively starving for CPU).
  - `+80` if `OOMCount > 0` (actively dying for Memory).
- **Expected Impact:** 
  - CPU Savings = (Requested - Proposed) * $0.05/hr * 730 hr/mo.
  - Memory Savings = (Requested - Proposed) * $0.01/GB/hr * 730 hr/mo.

## 5. Proposal Generation Model

A `Finding` maps to an `OptimizationProposal` modification. If multiple containers have findings, the `OptimizationProposal.Spec.ProposedResources` is built container-by-container, preserving the original resources for containers without findings.

## 6. Engine Runtime Model

- **Concurrency:** The Engine itself is stateless and synchronous per request. The overarching controller handles concurrency.
- **Failure Handling:** If Telemetry returns `ErrPrometheusUnavailable`, the Engine aborts evaluation and returns an error to the controller (triggering a Requeue).
- **Idempotency:** Re-evaluating the same telemetry yields the exact same proposal limits.

## 7. Testing Strategy

- **Rule Unit Tests:** Each rule is tested independently with mocked `ContainerMetrics`.
- **Engine Tests:** The `Analyzer` is tested with mocked `RuleRegistry` and `TelemetryProvider`.
- **Integration Tests:** (Later phase) using `envtest` to verify `OptimizationProposal` creation against an API server.
