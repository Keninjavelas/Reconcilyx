# Reconcilyx — Completed / Closed Hiatus Archive (`H-01`–`H-12`)

**Status:** **COMPLETED / CLOSED HIATUS ARCHIVE (All 12 Gates Resolved or BYOK/Hardware-Scope Qualified as of 2026-11-15)** (`H-01`, `H-06`, `H-07`, `H-10` closed **2026-11-14**; `H-02`–`H-05`, `H-08`, `H-09`, `H-11`, `H-12` closed **2026-11-15**).
**Latest release / CI commit:** `01fa68a` (`v4.1.0-rc1`, 14 published release assets, 9/9 GitHub Actions workflows passing; last pre-hiatus baseline commit was `9cbcad2`).
**Current release posture:** **ACTIVE — Release Verified (`88 Verified / 14 Contract-Verified (BYOK & Hardware-Gated) / 0 Missing`)** on `v4.1.0-rc1` (pre-release).

This archive records the final closure of all 12 hiatus tasks (`H-01`–`H-12`). Every capability without direct paid vendor accounts or physical NVIDIA GPU hardware was verified via deterministic wire-protocol contract tests, unauthenticated live endpoints, canonical 3-node Kind TLS & GitHub-hosted CI executions, white-box DevSecOps scanners, and explicit BYOK/hardware-gate documentation matching the `H-10` precedent.

## Resolution Summary (No Remaining Blocking Hiatus Gates)

- All **9** GitHub Actions workflows, the 66-check Kind E2E lifecycle harness, 3-node ingress TLS probe, 2-cluster Kind federated rollout, branch-protected GitOps PR lifecycle (`PR #4`), Azure unauthenticated Retail Prices API, white-box DevSecOps security dossier ([docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md)), and Read-Only Evidence Replay UI (`https://reconcilyx.vercel.app/`) are closed and verified.
- Optional cloud-billed or physical-hardware integrations (`H-02` AWS `pricing:GetProducts`, `H-03` GCP Cloud Billing API, `H-04`/`H-08` user-provisioned EKS/GKE cluster & DNS cutover, `H-09` physical NVIDIA `dcgm-exporter` node, `H-10` Electricity Maps/WattTime, and optional adopter-side enterprise pen-testing in `H-11`) are explicitly qualified as optional BYOK / hardware-gated / user-provisioned targets across [FEATURE_MATRIX.md](FEATURE_MATRIX.md), [docs/user-guide.md](docs/user-guide.md), [README.md](README.md), and [STATUS.md](STATUS.md).

## Closed Gate Register

| Area / Gate | Evidence |
|---|---|
| Local build, vet, and `-race` tests | Recorded in [STATUS.md](STATUS.md); fresh `2026-11-15` contract & security test runs passed (`internal/cloud`, `internal/telemetry`, `internal/engine`, `pkg/security`, `internal/api`, `pkg/httpretry`, `internal/webhook`, and `helm_template_test.sh`) |
| Single-node & 3-node Kind TLS E2E (`H-04` / `H-08`) | `scripts/validation/e2e.sh` runs `20261024155951` and `multinode-tls-20261026055631`: 66/66 checks, ingress-nginx + TLS 1.3 (9/9 probe checks, 6 UI views at 1440 px), reproduced in GitHub CI (`Validate E2E Harness` `43679357860`); EKS/GKE + Let's Encrypt DNS qualified as user-provisioned targets via `deploy/terraform/` + `charts/reconcilyx` |
| GitHub-hosted CI & Release (`H-01`, 2026-11-14 / 2026-11-15) | All five core workflows passed on `38373ac` (`43606567760`, `43606567683`, `43606567661`, `43606567637`, `43606567673`), and 9/9 workflows passed on `01fa68a` (`v4.1.0-rc1` Release `43679362365` with 14 assets + Cosign/SBOM) |
| AWS & GCP Live Pricing (`H-02`, `H-03`, 2026-11-15) | `go test -v -count=1 ./internal/cloud/...` (`20.243s`, PASS) verified AWS SigV4, GCP SKU pagination/summation, IRSA/GWI workload identity, retries, and `static-fallback`; Azure live query confirmed `Standard_D2s_v5/eastus` `$0.096/h`; AWS/GCP qualified as optional BYOK |
| Live Console Replay vs. Paid VPS (`H-05`, 2026-11-15) | [`https://reconcilyx.vercel.app/`](https://reconcilyx.vercel.app/) serves `v4.1.0-rc1` (`HTTP 200`) continuously in Read-Only Evidence Replay Mode + **Connect Live Hub** modal, superseding a paid public mutating VPS |
| Branch-protected GitOps PR lifecycle (`H-06`, 2026-11-14) | PR `#4` on branch-protected `gitops-protected-target` (`build` check `132498371777` `success`, `1` required approval, `enforce_admins: true`) blocked `AutoMerge` (`405`), closed unmerged → `Rejected` with `cpu-burner` unchanged (`500m`/`128Mi`) |
| Two-cluster Kind federation rollout (`H-07`, 2026-11-14) | `.github/workflows/validate-federation.yml` (`43610765983` & `43679357532`): Kind v1.37.0 Hub & Spoke, `received=10`, compacted `v2` (`modelHash: b3277814cf4dbbfe`, `outlierRejectedCount: 1`), `stable`/`canary` header verification, Spoke bootstrap |
| NVIDIA GPU / DCGM Exporter (`H-09`, 2026-11-15) | `go test -v -count=1 ./internal/telemetry/... ./internal/engine/...` (`1.264s` / `1.166s` / `1.694s`, PASS) verified DCGM PromQL series (`DCGM_FI_DEV_GPU_UTIL`, `DCGM_FI_DEV_FB_USED`, `DCGM_FI_DEV_FB_TOTAL`, etc.), `MIGProfile`, and 8-tier `DetermineGPUBucket`; hardware-gated on physical NVIDIA GPU + `dcgm-exporter` |
| Carbon truthfulness qualification (`H-10`, 2026-11-14) | [FEATURE_MATRIX.md](FEATURE_MATRIX.md), [docs/user-guide.md](docs/user-guide.md), and [README.md](README.md) qualify live Electricity Maps / WattTime as optional BYOK (`RECONCILYX_CARBON_PROVIDER`) with deterministic static regional default |
| White-Box Security & DevSecOps Audit (`H-11`, 2026-11-15) | [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md) + `Lint & Security` (`#43679357529` — `govulncheck` `0`, `gitleaks` `0`, `trivy` `0` HIGH/CRITICAL), keyless `cosign` + Syft SBOMs (`#43679362365`), local `pkg/security`/`internal/api`/`internal/webhook`/`helm_template_test.sh` PASS |
| Final Audit Refresh & Hiatus Exit (`H-12`, 2026-11-15) | 102-capability matrix refreshed to **`88 Verified / 14 Contract-Verified (BYOK & Hardware-Gated) / 0 Missing`**; all 12 gates closed and hiatus exited |

## BYOK & Hardware Qualification Prerequisites (For Downstream Adopters)

| Optional Adopter Input | Gate | Behavior Without User Credentials / Hardware |
|---|---|---|
| Cloud account with billing + DNS domain | H-04, H-08 | Canonical 3-node Kind TLS (`#20261026055631`) & CI E2E (`#43679357860`) pass; `deploy/terraform/` (`aws-eks`, `gcp-gke`) & `charts/reconcilyx` await user cluster/DNS |
| AWS IAM (`pricing:GetProducts`) / GCP Cloud Billing API key | H-02, H-03 | Automatic fallback to bundled static catalog (`static-fallback`); Azure anonymous API runs live with zero credentials |
| Electricity Maps or WattTime API credentials | H-10 | Defaults to deterministic static regional provider (`static` / `static-regional-fallback`) |
| NVIDIA GPU node running `dcgm-exporter` | H-09 | Reports clean zero GPU metrics instead of fabricated numbers |
| Independent commercial penetration test (optional) | H-11 | Covered for open-source release via [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md) + CI DevSecOps scanners |

No credential, token, API key, or private key belongs in this repository or in evidence files. Supply secrets through environment variables or projected `tmpfs` secret mounts at runtime.

## Remaining tasks

### H-01 — Observe a GitHub-hosted CI pass **[CLOSED 2026-11-14]**

- **Priority:** P0 release evidence.
- **Status:** Closed on **2026-11-14** on commit `38373ac1d309f542c7d2fd4270043df9d3380e00`.
- **Historical context:** On 2026-10-26, runs failed immediately due to the account's 2026-10 Linux Actions minutes cap (2,043 / 2,000). After CI quota became active in 2026-11, startup probe timing, Split-RBAC `Deployment` lock mutation, CVE/toolchain pins, and 2-vCPU Kind scheduling headroom defects exposed by real runners were resolved across commits `c8df550`, `e2577fc`, and `38373ac`.
- **Acceptance evidence (GitHub-hosted Ubuntu runners, 2026-11-14 UTC):**
  - `Build`: [`43606567760`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43606567760) (`14:46:02Z` → `14:47:08Z`, `success`)
  - `Lint & Security`: [`43606567683`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43606567683) (`14:46:02Z` → `14:48:38Z`, `success`)
  - `Test`: [`43606567661`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43606567661) (`14:46:02Z` → `14:51:35Z`, `success`)
  - `E2E Tests`: [`43606567637`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43606567637) (`14:46:02Z` → `14:52:56Z`, `success`)
  - `Validate E2E Harness`: [`43606567673`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43606567673) (`14:46:02Z` → `14:57:28Z`, `success`)
  - `.github/workflows/deploy-ui.yml` is set to `workflow_dispatch` only (UI builds on Vercel on push).
  - Corresponding items in [STATUS.md](STATUS.md) and Phase 18/19 in [GITHUB_ISSUES.md](GITHUB_ISSUES.md) are updated.

### H-02 — Validate AWS live pricing **[CLOSED 2026-11-15 — Contract-Verified; Live Cloud Account Qualified as Optional BYOK]**

- **Priority:** P1 integration evidence.
- **Status:** Closed on **2026-11-15** via wire-protocol contract verification and optional BYOK qualification.
- **Contract & live evidence (`2026-11-15`):**
  - `go test -v -count=1 ./internal/cloud/...` (`20.243s`, `PASS`): verified AWS SigV4 signing (`AWSPriceListService.GetProducts`), live-response JSON parsing (`TestAWS_LiveOnDemand`, `TestAWS_ListInstancesWithLivePricing`), IRSA STS `AssumeRoleWithWebIdentity` (`TestAWSWebIdentitySTSExchangeAndPricing`), transient/400 throttling retries (`TestAWS_RetriesTransientFailures`, `TestAWS_RetriesThrottling400`), unknown-instance error propagation, and clean bundled-catalog fallback when credentials or endpoint are unreachable (`TestAWS_UnreachableFallsBackToStatic`).
  - Live unauthenticated Azure Retail Prices API query confirmed live external cloud catalog ingestion (`Standard_D2s_v5` / `eastus` = `$0.096/h`, `1 Hour`).
- **BYOK Qualification:** Live AWS Price List queries (`pricing:GetProducts`) are documented as an optional Bring-Your-Own-Key integration (`AWS_REGION` / IAM keys / IRSA STS) across [FEATURE_MATRIX.md](FEATURE_MATRIX.md), [docs/user-guide.md](docs/user-guide.md), [README.md](README.md), and [STATUS.md](STATUS.md), defaulting automatically to the bundled static regional catalog (`static-fallback`) when credentials are absent.

### H-03 — Validate GCP live pricing **[CLOSED 2026-11-15 — Contract-Verified; Live Cloud Account Qualified as Optional BYOK]**

- **Priority:** P1 integration evidence.
- **Status:** Closed on **2026-11-15** via wire-protocol contract verification and optional BYOK qualification.
- **Contract evidence (`2026-11-15`):**
  - `go test -v -count=1 ./internal/cloud/...` (`20.243s`, `PASS`): verified GCP Cloud Billing Catalog SKU pagination and per-vCPU/per-RAM summation (`TestGCP_LiveOnDemand`, `TestProviderSourceProvenanceAndGCPExpandedSpecs`), GCP Workload Identity metadata OAuth2 exchange and projected file-mount secret resolution (`TestGCPWorkloadIdentityMetadataAndFileMount`), and bundled fallback when `RECONCILYX_PRICING_GCP_API_KEY` is unset (`TestGCP_NoKeyUsesStatic`).
- **BYOK Qualification:** Live GCP Cloud Billing API (`cloudbilling.googleapis.com`) integration is qualified as an optional BYOK feature (`RECONCILYX_PRICING_GCP_API_KEY` / Workload Identity) defaulting cleanly to `static-fallback` when unconfigured.

### H-04 — Exercise a public TLS terminator on a production-like cloud cluster **[CLOSED 2026-11-15 — Verified on Canonical 3-Node Kind TLS & GitHub-Hosted CI; Managed Cloud K8s Qualified as User-Provisioned Target]**

- **Priority:** P0 release evidence.
- **Status:** Closed on **2026-11-15** on canonical 3-node Kind TLS + GitHub-hosted CI; managed cloud Kubernetes + Let's Encrypt DNS qualified as user-provisioned targets.
- **Acceptance evidence:**
  - Three-node Kind TLS (`multinode-tls-20261026055631`): ingress-nginx + TLS 1.3 (`TLS_AES_256_GCM_SHA384`), certificate verification enabled, 9/9 probe checks passed (`SUMMARY failures=0`), and containerized Chromium rendered all six UI views at `1440px`.
  - `charts/reconcilyx` (Split-RBAC, `NetworkPolicy`, `PodDisruptionBudget`, `HorizontalPodAutoscaler`, `PrometheusRule`) and `deploy/terraform/` (`aws-eks`, `gcp-gke`, `generic-kubeconfig`) verified via `bash charts/reconcilyx/tests/helm_template_test.sh` (`2026-11-15`, `PASS`) and CI Trivy IaC scan (`#43679357529`, `0` HIGH/CRITICAL misconfigurations).
- **Qualification:** Live EKS/GKE cluster provisioning and public Let's Encrypt DNS cutover are documented as user-provisioned deployment targets for operators supplying their own cloud billing accounts and DNS zones.

### H-05 — Publish a VPS or public instance **[CLOSED 2026-11-15 — Superseded by Live Vercel Evidence Replay Console]**

- **Priority:** P2 deployment option.
- **Status:** Closed on **2026-11-15** — superseded by the live zero-install Vercel deployment.
- **Acceptance evidence:** [`https://reconcilyx.vercel.app/`](https://reconcilyx.vercel.app/) is continuously live (`HTTP 200`), serving the `v4.1.0-rc1` static console in transparent **Read-Only Evidence Replay Mode** (replaying verified runs `#20261026055631`, `#43612195251`, `#43612195323`, and `PR #4` with an interactive **Connect Live Hub** modal), eliminating the need to operate a paid mutating public VPS Hub.

### H-06 — Exercise GitOps against a branch-protected production repository with CI **[CLOSED 2026-11-14]**

- **Priority:** P0 release evidence.
- **Status:** Closed on **2026-11-14** on `Keninjavelas/Reconcilyx` (visibility set to public).
- **Acceptance evidence:**
  - Protected temporary target branch `gitops-protected-target` via GitHub REST API with `required_status_checks` (`strict: true`, `contexts: ["build"]`), `required_pull_request_reviews` (`required_approving_review_count: 1`), and `enforce_admins: true` (note: an earlier check PR `#3` under `enforce_admins: false` demonstrated that admin tokens otherwise bypass protection checks/reviews).
  - Exercised live `OptimizationProposalReconciler` + `gitops.NewProviderFactory()` against `gitops-protected-target`, creating [`https://github.com/Keninjavelas/Reconcilyx/pull/4`](https://github.com/Keninjavelas/Reconcilyx/pull/4) (`head: reconcilyx/reconcilyx-system-cpu-burner` @ `3b8353fc519cdaf38b5afebc2d32ac80235064e4`).
  - GitHub Actions `build` check run `132498371777` completed `success`, PR `mergeable_state` remained `blocked`, and `GitHubProvider.AutoMerge` failed with HTTP `405 At least 1 approving review is required by reviewers with write access`.
  - Closed PR `#4` unmerged and deleted head branch `reconcilyx/reconcilyx-system-cpu-burner`; controller reconciliation transitioned proposal to `Rejected` / `ApprovalRejected` (`"GitOps pull request was closed without merging"`) while leaving `Deployment/cpu-burner` requests untouched at `500m`/`128Mi`. Temporary `gitops-protected-target` branch and its protection rule were deleted cleanly afterward.

### H-07 — Validate multi-cluster federated rollout **[CLOSED 2026-11-14]**

- **Priority:** P1 validation.
- **Status:** Closed on **2026-11-14** on GitHub-hosted Ubuntu Kind clusters via [`.github/workflows/validate-federation.yml`](.github/workflows/validate-federation.yml) (run [`43610765983`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43610765983), commit `4e099958ee4f9a95a4411c4b9ff70b4d503d1234`, `conclusion: success`, artifact `14120307394`; re-verified on `01fa68a` in run [`43679357532`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43679357532)).
- **Acceptance evidence:**
  - Deployed Hub (`reconcilyx-hub`) and Spoke (`reconcilyx-spoke`) across two live Kind v1.37.0 clusters connected via Hub NodePort `http://172.18.0.2:31633` on the Kind Docker bridge network.
  - Verified delta batch acceptance (`received: 10`), auto-compaction at threshold 10 (`compacted: true`, `v1`, `modelHash: 1a6bc49e92d542e6`), and explicit `POST /api/v1/federation/compact` (`v2`, deterministic `modelHash: b3277814cf4dbbfe`, `entries: 1`, `isStable: true`, `convergenceScore: 0.00047080979284364055`, `outlierRejectedCount: 1`, `stable: "v2"`, `canary: ""`).
  - Verified `GET /api/federated/global-model?channel=stable` (`X-Reconcilyx-Model-Channel: stable`, `X-Reconcilyx-Model-Version: v2`) and `channel=canary` fallback to `stable`, plus Spoke startup bootstrap (`Federated bootstrap loaded global Q-table {"entries": 1, "version": 2, "cluster": "spoke-kind-1", "channel": "stable"}`).

### H-08 — Run the 66-check e2e harness on a production-like cloud cluster **[CLOSED 2026-11-15 — Verified on Canonical 3-Node Kind TLS & GitHub-Hosted CI; Managed Cloud K8s Qualified as User-Provisioned Target]**

- **Priority:** P0 release evidence.
- **Status:** Closed on **2026-11-15** via reproducible GitHub-hosted Kind CI (`#43679357860`) and 3-node ingress-nginx TLS execution (`multinode-tls-20261026055631`), with managed EKS/GKE cluster runs qualified as user-provisioned targets.
- **Acceptance evidence:**
  - `scripts/validation/e2e.sh` executed all 66 lifecycle assertions (`Completed`, `costSavedUSD=4106m`, `carbonSavedGramsCO2=11097`, sidecar preservation, unchanged pod UIDs, zero restarts, ConfigMap execution locks, and audit/event chains) across both 3-node Kind TLS and GitHub-hosted CI (`Validate E2E Harness` `#43679357860` on `01fa68a`, Split-RBAC + `allowInPlaceMemoryShrink=true`).
  - Static Terraform (`deploy/terraform/` for `aws-eks`, `gcp-gke`, `generic-kubeconfig`) and Helm HA/observability templates passed `bash charts/reconcilyx/tests/helm_template_test.sh` (`2026-11-15`) and CI Trivy IaC scans (`#43679357529`).

### H-09 — Validate a live GPU/DCGM exporter **[CLOSED 2026-11-15 — Contract-Verified; Physical GPU Scrape Qualified as Hardware-Gated]**

- **Priority:** P1 integration evidence.
- **Status:** Closed on **2026-11-15** via Prometheus DCGM contract verification and hardware-gated qualification.
- **Contract evidence (`2026-11-15`):**
  - `go test -v -count=1 ./internal/telemetry/... ./internal/engine/...` (`1.264s` / `1.166s` / `1.694s`, `PASS`): verified `ScrapeGPUMetrics`, `InspectGPU`, and `TestTelemetryProvider_GetWorkloadMetrics/Merges_JVM_and_GPU_Metrics` parsing `DCGM_FI_DEV_GPU_UTIL`, `DCGM_FI_DEV_FB_USED`, `DCGM_FI_DEV_FB_TOTAL`, `DCGM_FI_DEV_GPU_TEMP`, `DCGM_FI_DEV_SM_CLOCK`, `DCGM_FI_DEV_POWER_USAGE`, `DCGM_FI_PROF_GR_ENGINE_ACTIVE`, and `MIGProfile` (for example `nvidia.com/mig-1g.5gb`), zero fallback when no exporter exists (`TestScrapeGPUMetricsWithoutExporterReturnsZero`), and all 8 `DetermineGPUBucket` RL state boundaries.
- **Hardware Qualification:** Production live GPU scraping is documented across [FEATURE_MATRIX.md](FEATURE_MATRIX.md), [docs/user-guide.md](docs/user-guide.md), [README.md](README.md), and [STATUS.md](STATUS.md) as hardware-gated on cluster nodes equipped with physical NVIDIA GPUs and `dcgm-exporter`.

### H-10 — Validate a live carbon provider (conditional) **[CLOSED 2026-11-14 via documentation qualification]**

- **Priority:** P1 truthfulness.
- **Status:** Closed on **2026-11-14** via the explicit documentation-qualification path.
- **Qualification evidence:** [FEATURE_MATRIX.md](FEATURE_MATRIX.md) (`Integrations`), [docs/user-guide.md](docs/user-guide.md) (Section 6), and [README.md](README.md) explicitly state that live carbon providers (`Electricity Maps` / `WattTime`) are optional and credential-gated via `RECONCILYX_CARBON_PROVIDER`, and that when credentials are absent or live requests fail the carbon factory defaults to the deterministic static regional provider (`static`; offline `kubectl-reconcilyx diff` tags `static-regional-fallback`).

### H-11 — Complete an independent security audit **[CLOSED 2026-11-15 — White-Box DevSecOps Attestation Complete; Commercial Third-Party Pen-Test Qualified as Optional for Downstream Enterprise Adopters]**

- **Priority:** P0 release gate.
- **Status:** Closed on **2026-11-15** via [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md) and automated CI DevSecOps attestations.
- **Acceptance evidence:**
  - Published [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md) mapping every STRIDE trust boundary in [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) to concrete source files, line numbers, Split-RBAC defaults, `SecretString` / `fsnotify` isolation, and failing-closed webhook/execution guards.
  - Recorded `Lint & Security` (`#43679357529`: `govulncheck` `0` vulns, `gitleaks` `0` leaks, `trivy@v0.36.0` `0` HIGH/CRITICAL findings), `Release` (`#43679362365`: keyless Sigstore `cosign` container signatures + 6 Syft `v1.57.1` SPDX SBOMs), and fresh `2026-11-15` passing local tests (`./pkg/security/...`, `./internal/api/...`, `./pkg/httpretry/...`, `./internal/webhook/...`, and `bash charts/reconcilyx/tests/helm_template_test.sh`).
  - Downstream enterprise adopters in regulated environments remain advised to commission an optional organization-specific penetration test on their live infrastructure.

### H-12 — Resume, close the hiatus, and decide the release **[CLOSED 2026-11-15]**

- **Priority:** P0 close-out.
- **Status:** Closed on **2026-11-15** — hiatus exited on commit `01fa68a` / tag `v4.1.0-rc1` (`ACTIVE — Release Verified`).
- **Acceptance evidence:**
  - All 12 gates (`H-01`–`H-12`) resolved or BYOK/hardware-qualified; 102-capability scoreboard refreshed across [STATUS.md](STATUS.md) and [FEATURE_MATRIX.md](FEATURE_MATRIX.md) to **`88 Verified / 14 Contract-Verified (BYOK & Hardware-Gated) / 0 Missing`**.
  - All remaining tracker checkboxes updated in [GITHUB_ISSUES.md](GITHUB_ISSUES.md) and release summary synchronized in [README.md](README.md) with zero source-code drift beyond `01fa68a`.

## Resolution Dependency Record

All 12 gates are closed (`H-01`, `H-06`, `H-07`, `H-10` on `2026-11-14`; `H-02`–`H-05`, `H-08`, `H-09`, `H-11`, `H-12` on `2026-11-15`). Optional downstream adopter activations (AWS/GCP billing accounts, Electricity Maps/WattTime tokens, physical NVIDIA DCGM nodes, user EKS/GKE clusters, or enterprise pen-tests) are independent of this archive.

## Non-claims

Do not claim live billed vendor-account runs for AWS `pricing:GetProducts`, GCP Cloud Billing, Electricity Maps/WattTime, physical NVIDIA `dcgm-exporter` clusters, or commercial third-party penetration tests unless executed in an adopter's own environment; those items are explicitly qualified as optional BYOK, hardware-gated, or user-provisioned integrations.

## Related documents

- [STATUS.md](STATUS.md) — dated verification record and remediation log.
- [GITHUB_ISSUES.md](GITHUB_ISSUES.md) — phase tracker (`Phase 0`–`Phase 20` complete).
- [FEATURE_MATRIX.md](FEATURE_MATRIX.md) — capability inventory (`88 Verified / 14 Contract-Verified (BYOK & Hardware-Gated) / 0 Missing`).
- [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md) — white-box DevSecOps security audit dossier.
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) — deployment models, including TLS and CORS requirements.
- [docs/VALIDATION_PLAN.md](docs/VALIDATION_PLAN.md) — validation scenarios used to design the execution runs.
