# Reconcilyx Demo Script

**Objective:** A tightly paced 5-minute technical demonstration proving the complete autonomous lifecycle of Reconcilyx from Telemetry ingestion to Server-Side Apply actuation.

## Preparation
- Kind cluster running.
- Prometheus installed.
- Reconcilyx Controller deployed.
- Dummy workloads running (via `scripts/validation/deploy-workloads.sh`).

---

## 1. Introduction (0:00 - 0:30)
**Narrative:** "Reconcilyx is an autonomous Kubernetes right-sizing platform. It closes the loop between observability and actuation using a strictly governed policy engine and Server-Side Apply."
**Action:** Show the architecture diagram from `ARCHITECTURE.md`.

## 2. The Setup (0:30 - 1:30)
**Narrative:** "We have a highly overprovisioned `nginx` deployment. It's requesting 2 CPUs but barely using 50 millicores."
**Action:** 
```bash
kubectl get deploy cpu-hog -o custom-columns=NAME:.metadata.name,CPU_REQ:.spec.template.spec.containers[0].resources.requests.cpu
```
*Output clearly shows `2000m`.*

## 3. The Proposal Engine (1:30 - 2:30)
**Narrative:** "Behind the scenes, our Engine aggregates P95 telemetry from Prometheus and outputs an `OptimizationProposal`."
**Action:** 
```bash
kubectl get optimizationproposals -w
```
*Wait for the Proposal to appear. Then inspect it:*
```bash
kubectl describe optimizationproposal cpu-hog-reduction
```
*Highlight:*
- `Recommendation: ReduceCPURequest`
- `ProposedResources: 100m`
- `RiskScore: 12`
- `ConfidenceScore: 95`

## 4. Policy Governance (2:30 - 3:30)
**Narrative:** "Because the Confidence is high and the Risk is low, our `OptimizationPolicy` automatically approves it. Let's look at the lifecycle state machine."
**Action:**
```bash
kubectl get optimizationproposal cpu-hog-reduction -o yaml | grep "phase:"
```
*Show transition from `Evaluating` → `Approved` → `Scheduled`.*

## 5. Safe Actuation (3:30 - 4:30)
**Narrative:** "Reconcilyx is now executing. It acquires an optimistic lock, takes a deep-copy snapshot for safe rollbacks, and submits a targeted Server-Side Apply patch modifying ONLY the CPU requests."
**Action:** 
```bash
kubectl get events --field-selector involvedObject.name=cpu-hog-reduction
```
*Output shows:*
- `Normal ExecutionStarted Acquired lock and captured execution snapshot`
- `Normal ExecutionApplied Server-Side Apply patch submitted`
- `Normal ExecutionVerified Mutation matches intended state`

## 6. Verification (4:30 - 5:00)
**Narrative:** "Let's verify the workload. The CPU is lowered, but nothing else was touched."
**Action:**
```bash
kubectl get deploy cpu-hog -o yaml | grep "requests:" -A 2
```
*Output shows `cpu: 100m`.*
**Action:**
```bash
kubectl get deploy cpu-hog -o yaml | grep "reconcilyx.io/last-action"
```
*Output shows Reconcilyx ownership annotation.*

**Narrative:** "A complete, autonomous, mathematically governed optimization without human intervention."
