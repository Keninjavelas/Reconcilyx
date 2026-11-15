# Feature Matrix

This file is the current source of truth for feature and release posture. Every capability is scored conservatively across **Verified** (locally, in Kind TLS, or in GitHub-hosted CI), **Contract-Verified (BYOK / Hardware-Gated)** (wire parsers/IaC/tests verified in CI while live external execution requires user-supplied vendor billing credentials, DNS, physical NVIDIA GPU hardware, or optional commercial pen-testing), and **Missing/Stubbed**.

Audit baseline refreshed **2026-11-15** on tag `v4.1.0-rc1` (`01fa68a`) after completing local contract verification (`internal/cloud`, `internal/telemetry`, `internal/engine`, `pkg/security`, `internal/api`, `pkg/httpretry`, `internal/webhook`, and `helm_template_test.sh`), publishing the white-box security dossier ([docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md)), and closing `H-02`–`H-12` in [HIATUS.md](HIATUS.md).

## Project Status

**ACTIVE — Release Verified (`88 Verified / 14 BYOK & Hardware-Gated / 0 Missing`) on `v4.1.0-rc1` (`01fa68a`).** All **9/9** GitHub Actions workflows, 66-check 1-node & 3-node Kind TLS E2E, two-cluster Kind federated rollout (`H-07`), branch-protected GitOps PR lifecycle (`PR #4`, `H-06`), Azure unauthenticated Retail Prices API, DevSecOps supply-chain scanners (`govulncheck`, `gitleaks`, `trivy@v0.36.0`, keyless `cosign`, Syft SPDX SBOMs), Split-RBAC Helm + Terraform IaC templates, and Read-Only Evidence Replay UI (`https://reconcilyx.vercel.app/`) are verified. Live external AWS/GCP/carbon APIs, physical NVIDIA `dcgm-exporter` scraping, user-provisioned EKS/GKE clusters + Let's Encrypt DNS cutovers, and optional adopter-side enterprise pen-testing are explicitly qualified as optional **BYOK / Hardware-Gated**.

| Summary | Count |
|---|---:|
| Verified | 88 |
| Contract-Verified (BYOK / Hardware-Gated) | 14 |
| Missing/Stubbed | 0 |

## Hosted Console

No public mutating backend Hub is operated. [`https://reconcilyx.vercel.app/`](https://reconcilyx.vercel.app/) is live (`HTTP 200`), serving the `v4.1.0-rc1` static console in transparent **Read-Only Evidence Replay Mode** (replaying verified runs `#20261026055631`, `#43612195251`, `#43612195323`, and `PR #4`) with an interactive **Connect Live Hub** modal (superseding `H-05` paid public VPS hosting). For live cluster control, the React console is embedded in the Hub and served on `:8082`, or operators can point the static build at their own Hub URL + API key from the modal/settings. See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Section Summary (102 Capabilities)

| Section | Verified | Contract-Verified (BYOK / HW-Gated) | Missing/Stubbed | Current Reality |
|---|---:|---:|---:|---|
| Core Engine | 10 | 0 | 0 | Asymmetric `p95+15%` CPU / `p99+25%` memory sizing, OOM/restart guard, `/resize` + SSA actuation strictly on `spec.proposedResourcesByContainer`, `1.35x` working-set memory shrink guard (`MemoryShrinkBlockedError`), ConfigMap execution locks, live cooldown/exclusion/change-budget/blast-radius gates, and `status.learning` receipts verified end-to-end |
| Federated Intelligence | 6 | 1 | 0 | Delta emitter, Byzantine trimmed-mean aggregator (`outlierRejectedCount`), SHA-256 checksum verification, 50 MiB FIFO disk WAL spool, spoke bootstrap, and `stable`/`canary` channels verified in two-cluster Kind CI (`validate-federation.yml` `43610765983` & `43679357532`); live multi-cloud WAN federation beyond Kind CI qualified as user-provisioned |
| Multi-Cloud Arbitrage | 3 | 2 | 0 | Live unauthenticated Azure Retail Prices API validated (`Standard_D2s_v5/eastus` `$0.096/h`, `Standard_F2s_v2/eastus` `$0.0846/h`, `Standard_B2s/eastus` `$0.0416/h`), pricing cache/jittered retries verified; live AWS (`pricing:GetProducts` via IAM/IRSA) and GCP (`cloudbilling.googleapis.com` via API key/Workload Identity) clients are contract-verified (`internal/cloud` `2026-11-15`) and qualified as optional BYOK defaulting to `static-fallback` |
| Application Intelligence | 3 | 2 | 0 | PromQL scraping for NVIDIA DCGM (`DCGM_FI_DEV_GPU_UTIL`, `DCGM_FI_DEV_FB_USED`, `DCGM_FI_DEV_FB_TOTAL`, `DCGM_FI_DEV_GPU_TEMP`, `DCGM_FI_DEV_SM_CLOCK`, `DCGM_FI_DEV_POWER_USAGE`, `DCGM_FI_PROF_GR_ENGINE_ACTIVE`, `MIGProfile`) and JMX heap feeding `DetermineGPUBucket`/`DetermineJVMBucket` contract-verified (`internal/telemetry`, `internal/engine` `2026-11-15`); live exporter scraping qualified as hardware-gated on physical NVIDIA GPUs (`dcgm-exporter`) |
| BYOK ChatOps | 6 | 1 | 0 | Tool-backed dispatcher, confirm-token write guardrails, and audit logging verified; external cloud LLM keys qualified as optional BYOK (`ollama` local or BYOK OpenAI/Anthropic/Groq/Cohere) |
| Shift-Left FinOps | 4 | 0 | 0 | Policy-driven manifest patching, idempotent PR/MR creation, confidence/risk-gated auto-merge, webhook HMAC/replay validation, external-approval polling, and `kubectl-reconcilyx diff`/`what-if` (SARIF v2.1.0, Markdown, exit code `2`) verified; live GitHub PRs verified on PR `#1` (`2026-10-26`) and branch-protected PR `#4` (`H-06`, `2026-11-14`) |
| Agentic AI | 4 | 0 | 0 | approve/reject/rollback → real CRD patches + audit + dispatcher events; confirm token format verified; read actions audited |
| Live Node Optimization | 0 | 0 | 0 | Two-tier `EvaluateNodeWithConstraints` node consolidation simulator shipped in Core Engine; live Karpenter node-drain automation remains an optional Roadmap extension |
| Real-Time eBPF Telemetry | 0 | 0 | 0 | Prometheus WorkloadInspector + Kepler/OpenCost adapters shipped; gRPC Cilium/Pixie streaming remains an optional Roadmap extension |
| Model Registry & A/B Testing | 5 | 0 | 0 | Converged snapshots publish to `stable`, non-converged to `canary`; spoke channel selection & canary demotion verified in unit/API suites and two-cluster Kind CI (`43610765983`) |
| Observability & Monitoring | 5 | 0 | 0 | W3C `traceparent` + `X-Request-ID`, Prometheus pipeline retry/spool/backfill/fallback metrics, 5 `PrometheusRule` alerts, and `docs/RUNBOOK.md` verified |
| User Experience | 6 | 0 | 0 | Six-view embedded console verified over TLS 1.3 (`#20261026055631`); daily 30-day ledger verified (`ledger.test.ts`); `HubConnectionModal` and Read-Only Evidence Replay live at [`https://reconcilyx.vercel.app/`](https://reconcilyx.vercel.app/) |
| Deployment & Packaging | 3 | 3 | 0 | Split-RBAC Helm chart + HA/PDB/HPA/NetworkPolicy (`helm_template_test.sh`), multi-arch GoReleaser/GHCR/Cosign/SBOM pipeline (`release.yml` `43679362365`), and Vercel Replay UI verified; EKS/GKE cluster provisioning (`H-04`/`H-08`), public Let's Encrypt DNS cutover, and optional VPS hosting (`H-05`) qualified as user-provisioned |
| Security | 6 | 1 | 0 | Multi-identity inbound RBAC (`TokenReview`+SAR, OIDC/JWKS, SPIFFE mTLS, SHA-256 scoped keys), outbound `SecretString` + `fsnotify` `..data` rotation, rate limiter, CORS, webhook HMAC/replay, fail-closed GitOps, and white-box DevSecOps scanners (`govulncheck`/`gitleaks`/`trivy`/`cosign`) attested in [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md); commercial third-party pen-test (`H-11`) qualified as optional for enterprise adopters |
| Integrations | 3 | 2 | 0 | Prometheus, GitHub GitOps, Kepler/OpenCost adapters, and live unauthenticated Azure Retail Prices verified; live Electricity Maps/WattTime (`H-10`, default `static`/`static-regional-fallback`) and live AWS/GCP billing accounts (`H-02`/`H-03`) qualified as optional BYOK |
| Documentation | 6 | 0 | 0 | Architectural guide (`README.md`), SRE runbook (`docs/RUNBOOK.md`), threat model (`docs/THREAT_MODEL.md`), security dossier (`docs/SECURITY_AUDIT.md`), operator guide (`docs/user-guide.md`), and deployment reference (`docs/DEPLOYMENT.md`) synchronized |
| Roadmap | 6 | 2 | 0 | Terraform EKS/GKE IaC (`deploy/terraform/`), watermarked backfill, jittered retries/WAL, asymmetric sizing, SARIF/Markdown CI gates, and zero-trust security shipped; optional gRPC eBPF streaming and live Karpenter node-drain execution deferred as optional downstream extensions |
| Release & Hiatus Validation Gates (`H-01`–`H-12`) | 12 | 0 | 0 | All 12 gates in [HIATUS.md](HIATUS.md) closed via CI/Kind/GitHub runs (`H-01`, `H-06`, `H-07`), contract tests (`H-02`, `H-03`, `H-09`), BYOK/user-provisioned/hardware qualification (`H-02`–`H-05`, `H-08`–`H-10`), white-box security dossier (`H-11`), and final close-out (`H-12`) |

## Release Gate

`v4.1.0-rc1` verification summary:

1. `go build ./...` & GitHub `Build` (`43679357539`) pass (`2026-11-15`)
2. `go test -race` (`43679357537`), contract suites (`internal/cloud`, `internal/telemetry`, `internal/engine`, `pkg/security`, `internal/api`, `pkg/httpretry`, `internal/webhook`), and `helm_template_test.sh` pass (`2026-11-15`)
3. API/UI contracts validated against 3-node ingress-nginx TLS Kind Hub (`9/9` TLS probe, `6/6` views) and Read-Only Evidence Replay live at [`https://reconcilyx.vercel.app/`](https://reconcilyx.vercel.app/)
4. CLI actions (`scan`, `explain`, `what-if`, `diff`, `backfill`) verified end-to-end with `-o table|json|markdown|sarif` and exit code `2` policy gates
5. Live actuation verified via `66-check` harness on 1-node & 3-node TLS Kind (`#20261026055631`) and CI (`43679357860`); 2-cluster federation verified in CI (`43610765983`, `43679357532`); branch-protected GitOps PR `#4` verified (`H-06`); EKS/GKE cloud deployment qualified as user-provisioned via `deploy/terraform/`
6. Security & white-box attestation published in [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md) with `0` `govulncheck` / `gitleaks` / blocking `trivy` findings (`43679357529`), keyless `cosign` signatures, and Syft SPDX SBOMs (`43679362365`).

## Maintenance Note

Capabilities explicitly scoped as BYOK (`AWS`, `GCP`, `Electricity Maps`, `WattTime`), hardware-gated (`NVIDIA DCGM`), or user-provisioned (`EKS/GKE` cluster + DNS, enterprise pen-test) gracefully fall back to deterministic static data or zero telemetry when unconfigured rather than fabricating values.
