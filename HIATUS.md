# Reconcilyx — Project Hiatus and Remaining Work

**Status:** ON HIATUS as of **2026-10-26**.
**Last code commit before the hiatus:** `9cbcad2` (`fix(cloud): ignore non-on-demand Azure meters`).
**Current release posture:** `v4.0.0-beta`, not production-ready. The audit baseline is unchanged at **0 Verified / 72 Partial / 30 Missing**.

This file is the single list of work that remains. It is deliberately conservative: an item is complete only when dated, reproducible evidence exists in [STATUS.md](STATUS.md), not when code compiles or a unit test passes.

## What the hiatus means

- No new external validation is being attempted. No cloud accounts, provider credentials, GPU hosts, or CI minutes are assumed.
- The repository remains public and the local evidence already recorded stays valid.
- No feature is promoted to `Verified` during the hiatus. [FEATURE_MATRIX.md](FEATURE_MATRIX.md) and [STATUS.md](STATUS.md) remain the sources of truth.
- Documentation may still be corrected for accuracy, but no new claims are added.
- Resuming requires the prerequisites in [Resume prerequisites](#resume-prerequisites); the tasks below are ordered so that independent work can run in parallel once unblocked.

## Completed before the hiatus (no action required)

| Area | Evidence |
|---|---|
| Local build, vet, and tests | Recorded in [STATUS.md](STATUS.md); one Windows filesystem flake is documented and was resolved on rerun |
| Single-node Kind e2e | `scripts/validation/e2e.sh` run `20261024155951`: 66 checks, 0 failures, `Completed`, finalized `costSavedUSD=4106m` receipt |
| Three-node Kind TLS e2e | Run `multinode-tls-20261026055631`: 66 checks, 0 failures through ingress-nginx with a locally trusted CA |
| Console over TLS | All six views rendered over a verified TLS 1.3 endpoint on Kind, with zero Reconcilyx API failures |
| Live GitOps PR | PR #1 on the maintainer-owned `Keninjavelas/Reconcilyx` repo; closed unmerged, branch deleted, proposal `Rejected`, no local actuation |
| Azure live pricing | Anonymous Retail Prices API validated: `Standard_F2s_v2/eastus` `$0.0846/h`, `Standard_B2s/eastus` `$0.0416/h`, `Standard_D2s_v5/eastus` `$0.0960/h`; a meter-selection defect was fixed with a regression test |
| CI blocker diagnosis (2026-10-26) | Historical: five workflows failed in about three seconds with no runner assigned when the account hit 2,043 of 2,000 included Linux minutes for 2026-10 (superseded by H-01 closure on 2026-11-14) |
| GitHub-hosted CI & Kind E2E (`H-01`, 2026-11-14) | All five core workflows completed `success` on GitHub-hosted Ubuntu runners for commit `38373ac1d309f542c7d2fd4270043df9d3380e00`: `Build` (`43606567760`), `Lint & Security` (`43606567683`), `Test` (`43606567661` with Linux `-race`), `E2E Tests` (`43606567637`), and `Validate E2E Harness` (`43606567673`) |
| Carbon truthfulness qualification (`H-10`, 2026-11-14) | [FEATURE_MATRIX.md](FEATURE_MATRIX.md), [docs/user-guide.md](docs/user-guide.md), and [README.md](README.md) explicitly qualify live Electricity Maps / WattTime as optional and credential-gated (`RECONCILYX_CARBON_PROVIDER`) with deterministic static default (`static` / `static-regional-fallback`) |
| Two-cluster Kind federation rollout (`H-07`, 2026-11-14) | [`.github/workflows/validate-federation.yml`](.github/workflows/validate-federation.yml) run [`43610765983`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43610765983) (`4e09995`, `success`): Hub & Spoke Kind v1.37.0 clusters, `received=10`, compacted `v2` (`modelHash: b3277814cf4dbbfe`, `isStable: true`, `outlierRejectedCount: 1`), `stable`/`canary` header verification, and Spoke `Federated bootstrap loaded global Q-table` |
| Branch-protected GitOps PR lifecycle (`H-06`, 2026-11-14) | `Keninjavelas/Reconcilyx` public; branch `gitops-protected-target` protected (`build` required check + `1` required approval + `enforce_admins: true`): PR `#4` ran check `132498371777` (`success`), blocked `AutoMerge` (`405` / `mergeable_state=blocked`), and closed unmerged → `Rejected` with `cpu-burner` unchanged (`500m`/`128Mi`) |

## Resume prerequisites

| Prerequisite | Needed for | Notes |
|---|---|---|
| Cloud account with billing and a Kubernetes service | H-04, H-06, H-08 | AKS Free has a free control plane but billed nodes; Oracle Always Free currently advertises 2 OCPU / 12 GB Ampere A1 and 200 GB block storage, but verify current terms and OKE control-plane eligibility; any managed Kubernetes free tier is acceptable |
| Domain plus DNS control | H-04, H-05 | Required for a publicly trusted certificate. Cloudflare Quick Tunnel needs no account but is testing-only, has no SLA and documented limits, so it cannot support a production-like TLS claim |
| AWS IAM credentials with `pricing:GetProducts` | H-02 | The existing local profile returned `InvalidClientTokenId`, so it is not usable. Never commit keys or session tokens |
| GCP project with Cloud Billing API enabled and a restricted API key | H-03 | The provider reads `RECONCILYX_PRICING_GCP_API_KEY` or `GCP_BILLING_API_KEY`; restrict the key to the Cloud Billing API |
| Electricity Maps or WattTime credentials (optional) | H-10 *(closed 2026-11-14 via static-default qualification)* | Live carbon is optional and credential-gated (`RECONCILYX_CARBON_PROVIDER`); docs updated to document the static default |
| NVIDIA GPU host with a DCGM exporter | H-09 | No local GPU: `nvidia-smi` is not present on the workstation (checked 2026-10-26) |
| Repo admin rights and Docker daemon for branch protection & multi-cluster Kind | *(H-06 & H-07 closed 2026-11-14)* | Repo set public (`H-06` passed on `gitops-protected-target`), and GitHub-hosted Kind runners closed `H-07` (`43610765983`) |
| An independent security auditor | H-11 | A self-review is not an independent audit |

No credential, token, API key, or private key belongs in this repository or in evidence files. Supply secrets through environment variables or a secret manager at runtime.

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

### H-02 — Validate AWS live pricing

- **Priority:** P1 integration evidence.
- **Blocked by:** Valid AWS credentials.
- **Why it remains open:** The client is implemented and unit-tested, but the only local credential profile returned `InvalidClientTokenId`. No live AWS quote has been verified, so the AWS row remains unverified in [GITHUB_ISSUES.md](GITHUB_ISSUES.md) Phase 7.
- **What to do:** Provide an IAM principal with `pricing:GetProducts`, run the provider against `us-east-1` for a known instance (for example `c6i.large`), and compare the returned on-demand rate with the AWS Price List page or CLI for the same timestamp. Confirm SigV4 signing and that a missing/denied credential falls back to the bundled catalog with a warning.
- **Acceptance evidence:** Dated live price output, region, instance type, and the command used; record it in [STATUS.md](STATUS.md) and Phase 7. Keep the response as gitignored evidence, not a committed artifact.
- **Notes:** Retry the request on this workstation with an explicit public CA bundle if the local TLS interception is active; do not disable verification.

### H-03 — Validate GCP live pricing

- **Priority:** P1 integration evidence.
- **Blocked by:** A GCP project with the Cloud Billing API enabled and a restricted API key.
- **Why it remains open:** The client pages through the Cloud Billing Catalog API and totals per-vCPU and per-GB component SKUs, but no key is configured, so only the bundled fallback has been exercised.
- **What to do:** Enable `cloudbilling.googleapis.com`, create an API key restricted to that API, set `RECONCILYX_PRICING_GCP_API_KEY`, and quote a known type such as `e2-standard-2` in `us-central1`. Confirm the component SKUs, pagination, and the computed total.
- **Acceptance evidence:** Dated live output with region and instance type, compared against the Cloud Billing catalog; record it in [STATUS.md](STATUS.md) and Phase 7.
- **Notes:** If the key is unavailable, keep the documentation wording explicit that GCP pricing is credential-gated and uses the bundled fallback.

### H-04 — Exercise a public TLS terminator on a production-like cloud cluster

- **Priority:** P0 release evidence.
- **Blocked by:** A cloud account with billing, a Kubernetes cluster, and a domain/DNS.
- **Why it remains open:** TLS was validated only on Kind: first behind a local Node proxy with a local CA, then through ingress-nginx on a three-node Kind cluster with a local CA. No publicly trusted certificate, cloud load balancer, or managed ingress has been exercised. The 9-check probe and six-view console pass have therefore never run against a cloud endpoint.
- **What to do:** Deploy the Helm chart to a cloud cluster, install an ingress controller and cert-manager (or use a managed ingress), issue a certificate for a real hostname with Let's Encrypt, and point DNS at it. Re-run the 9-check TLS probe with system trust, then render all six console views at 1440 px.
- **Acceptance evidence:** Public hostname, certificate issuer and chain, TLS version/cipher, probe result, and screenshots; record the run in [STATUS.md](STATUS.md) and Phase 17/18. Verification must stay enabled.
- **Notes:** AKS Free has a free control plane but billed worker nodes. Oracle Always Free is a low-cost option, but confirm OKE control-plane terms and reclamation behavior first. Cloudflare Quick Tunnel is not sufficient for this claim.

### H-05 — Publish a VPS or public instance (optional path)

- **Priority:** P2 deployment option.
- **Blocked by:** A VPS, a domain, and DNS control.
- **Why it remains open:** The deployment guide describes a VPS with Caddy/Nginx, but no such deployment has been operated. The repository deliberately claims no public instance.
- **What to do:** Choose either this path or H-04, not both, unless a second environment is useful. Install the Hub on a VPS, terminate TLS with Caddy or cert-manager, set `RECONCILYX_API_KEY`, set `RECONCILYX_CORS_ORIGINS`, and verify the console and API over HTTPS.
- **Acceptance evidence:** A reachable HTTPS URL, certificate details, 401/200 API behavior, CORS allow/deny behavior, and a dated run; then decide whether a public instance is actually desired before documenting it.
- **Notes:** Do not publish an API key or a demo credential in the repository or screenshots.

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
- **Status:** Closed on **2026-11-14** on GitHub-hosted Ubuntu Kind clusters via [`.github/workflows/validate-federation.yml`](.github/workflows/validate-federation.yml) (run [`43610765983`](https://github.com/Keninjavelas/Reconcilyx/actions/runs/43610765983), commit `4e099958ee4f9a95a4411c4b9ff70b4d503d1234`, `conclusion: success`, artifact `14120307394`).
- **Acceptance evidence:**
  - Deployed Hub (`reconcilyx-hub`) and Spoke (`reconcilyx-spoke`) across two live Kind v1.37.0 clusters connected via Hub NodePort `http://172.18.0.2:31633` on the Kind Docker bridge network.
  - Verified delta batch acceptance (`received: 10`), auto-compaction at threshold 10 (`compacted: true`, `v1`, `modelHash: 1a6bc49e92d542e6`), and explicit `POST /api/v1/federation/compact` (`v2`, deterministic `modelHash: b3277814cf4dbbfe`, `entries: 1`, `isStable: true`, `convergenceScore: 0.00047080979284364055`, `outlierRejectedCount: 1`, `stable: "v2"`, `canary: ""`).
  - Verified `GET /api/federated/global-model?channel=stable` (`X-Reconcilyx-Model-Channel: stable`, `X-Reconcilyx-Model-Version: v2`) and `channel=canary` fallback to `stable`, plus Spoke startup bootstrap (`Federated bootstrap loaded global Q-table {"entries": 1, "version": 2, "cluster": "spoke-kind-1", "channel": "stable"}`).
- **Notes:** Keep the Federated Intelligence row `Partial` in [FEATURE_MATRIX.md](FEATURE_MATRIX.md) until a multi-cluster production-like cloud validation completes.

### H-08 — Run the 66-check e2e harness on a production-like cloud cluster

- **Priority:** P0 release evidence.
- **Blocked by:** A cloud Kubernetes cluster with Prometheus and the Helm-deployed manager.
- **Why it remains open:** The harness passed on Kind, including a three-node TLS cluster, but not on a managed cloud cluster. Managed distributions can differ in the `pods/resize` feature gate, admission behavior, and node configuration.
- **What to do:** Install Prometheus and the manager on the cloud cluster, confirm the Kubernetes version enables in-place pod resize, seed live pod requests that differ from the Deployment template, and run `make validate-e2e` with the Hub URL and API key supplied through the environment. If in-place resize is unavailable, record the limitation instead of switching mutation strategies.
- **Acceptance evidence:** Run ID, 66/66 checks, `Completed`, live-pod snapshot, sidecar and limit preservation, unchanged pod UIDs, zero restarts, finalized receipt, audit chain, and events; record it in [STATUS.md](STATUS.md) and the release gate.
- **Notes:** H-04 and this task can share one cluster. Do not count a Kind run as production-like evidence.

### H-09 — Validate a live GPU/DCGM exporter

- **Priority:** P1 integration evidence.
- **Blocked by:** An NVIDIA GPU host with a DCGM exporter.
- **Why it remains open:** `InspectGPU` and `ScrapeGPUMetrics` query DCGM series and no longer fabricate values, but they have only been exercised against mock series. No local GPU exists (`nvidia-smi` not found on 2026-10-26).
- **What to do:** Install a DCGM exporter on a GPU node, scrape the documented metrics into Prometheus, and run `InspectGPU`/`ScrapeGPUMetrics` against the workload. Confirm real utilization and framebuffer values reach the learning buckets and that a missing exporter yields zeroes rather than synthetic data.
- **Acceptance evidence:** Prometheus series names and values, inspect output, and the resulting learning buckets; record it in [STATUS.md](STATUS.md) and the Application Intelligence row.
- **Notes:** Cloud GPU time is billable; a physical GPU host is equally valid.

### H-10 — Validate a live carbon provider (conditional) **[CLOSED 2026-11-14 via documentation qualification]**

- **Priority:** P1 truthfulness.
- **Status:** Closed on **2026-11-14** via the explicit documentation-qualification path.
- **Qualification evidence:** [FEATURE_MATRIX.md](FEATURE_MATRIX.md) (`Integrations`, line 39), [docs/user-guide.md](docs/user-guide.md) (Section 6), and [README.md](README.md) (`Beta Status`, line 19) explicitly state that live carbon providers (`Electricity Maps` / `WattTime`) are optional and credential-gated via `RECONCILYX_CARBON_PROVIDER`, and that when credentials are absent or live requests fail the carbon factory defaults to the deterministic static regional provider (`static`; offline `kubectl-reconcilyx diff` tags `static-regional-fallback`). Live external provider accounts remain unvalidated.

### H-11 — Complete an independent security audit

- **Priority:** P0 release gate.
- **Blocked by:** An independent auditor.
- **Why it remains open:** Internal reviews exist, but the release posture explicitly requires an independent audit before production use. Auth, RBAC, webhook signatures, GitOps token handling, secret handling, deny-lists, and TLS should all be in scope.
- **What to do:** Commission the audit, provide the threat model and deployment guide, remediate findings, and publish a summary that distinguishes fixed, accepted, and out-of-scope items.
- **Acceptance evidence:** Audit report or attestation, remediation commits, and an updated release posture. Do not claim production readiness before this exists.
- **Notes:** Never weaken an admin gate, deny-list, or secret handler to satisfy a test or an audit finding; fix the test or the design instead.

### H-12 — Resume, close the hiatus, and decide the release

- **Priority:** P0 close-out.
- **Blocked by:** Completion of H-01 through H-11 as applicable.
- **Why it remains open:** The hiatus should end deliberately, not by drift.
- **What to do:** Re-read [STATUS.md](STATUS.md) and this file, rerun `go build ./...`, `go vet ./...`, `go test ./...`, and the UI build/tests, then work the tasks in dependency order. Update the audit counts only when production-like evidence exists. Remove the hiatus banners from the README, STATUS, FEATURE_MATRIX, and roadmap; record the final release decision.
- **Acceptance evidence:** A dated close-out entry in [STATUS.md](STATUS.md), updated trackers, and a release note that does not exceed the evidence.

## Dependency order

1. **Credentials and accounts:** H-02, H-03, H-10 can start independently as soon as their credentials exist.
2. **CI:** H-01 unblocks H-06's required-check path.
3. **Cloud cluster:** H-04 and H-08 share an environment; H-04 also needs DNS and a public certificate.
4. **Repository settings:** H-06 needs repo admin consent and CI capacity.
5. **Local-only:** H-07 can run without cloud spend but is paused by the hiatus decision.
6. **Hardware:** H-09 needs a GPU host.
7. **External party:** H-11 should follow the cloud and repository validations so the auditor sees the real deployment.
8. **Close-out:** H-12 is last.

## Non-claims

Until the relevant tasks are complete, do not describe Reconcilyx as production-ready, fully verified, publicly hosted, multi-cloud validated, GPU-validated, or independently audited. Do not treat Kind evidence, unit tests, or a compiled package as production-like validation. Do not publish credentials, tokens, private keys, or screenshots containing them.

## Related documents

- [STATUS.md](STATUS.md) — dated verification record and remediation log.
- [GITHUB_ISSUES.md](GITHUB_ISSUES.md) — phase tracker; Phase 19 consolidates these deferred items.
- [FEATURE_MATRIX.md](FEATURE_MATRIX.md) — conservative feature inventory and release gate.
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) — deployment models, including TLS and CORS requirements.
- [docs/VALIDATION_PLAN.md](docs/VALIDATION_PLAN.md) — validation scenarios used to design the remaining runs.
