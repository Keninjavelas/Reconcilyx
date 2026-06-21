# Actuation Safety V2

This document details the safety architecture that must exist before Reconcilyx is allowed to mutate any Kubernetes workload. 

## 1. Why V1 Was Unsafe
The initial Actuation architecture (V1) relied on the `Spec.ObservedResources` field to define the rollback state. This was fundamentally flawed:
- **The Stale Snapshot Hazard:** If a proposal was generated at T=0, but sat in a Cooldown queue until T=30, any manual scaling performed by an operator at T=15 would be erased if Reconcilyx rolled back to the T=0 state. It constituted a secondary, rogue mutation.
- **Missing Global Stop:** There was no immediate way to halt the operator if telemetry was fundamentally corrupted cluster-wide.

## 2. The Execution Snapshot Strategy
To guarantee a mathematically perfect rollback, Reconcilyx V2 implements a **Just-In-Time Execution Snapshot**.
- 1 millisecond before executing the Server-Side Apply patch, the Reconciler deeply copies the live `ContainerResources`, `ReplicaCount`, `Generation`, and `Annotations` of the target workload into `Status.ExecutionSnapshot`.
- **Rollback Guarantee:** If a rollback is triggered, Reconcilyx patches the workload back to the exact snapshot recorded at the time of execution. It completely ignores the Engine's original observations.

## 3. Global Kill-Switch Model
Reconcilyx implements a globally distributed Kill-Switch using a ConfigMap: `reconcilyx-system/reconcilyx-config`.
- **Key:** `actuation_paused: "true"`
- **Behavior:** The `ActuationControl` interface checks this ConfigMap before any mutation. If paused, all `Scheduled` proposals halt, but *Evaluation* continues. This allows operators to see what Reconcilyx *would* do without letting it actuate.
- **Visibility:** Emits `Warning` events and Prometheus metrics indicating executions are paused.

## 4. Ownership Tracking & GitOps Yield
Reconcilyx must play nicely with ArgoCD and Flux.
- **Field Manager:** SSA is executed using `fieldManager: reconcilyx-optimizer`.
- **Annotations:** An annotation `reconcilyx.io/last-action: execute` is injected into the workload.
- **Conflict Resolution:** During the `Verifying` and `Stabilizing` phases, Reconcilyx checks if the workload's resources match its intended SSA payload. If ArgoCD reverts the patch, Reconcilyx yields immediately, transitioning to `Failed: GitOpsOverride`. It never fights in a loop.
