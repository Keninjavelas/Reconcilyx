# Reconcilyx Security & Threat Model (`v4.1.0-rc1`)

Reconcilyx separates **analytical intelligence** (`diff`, `scan`, `what-if`, `explain`, read-only shadow recommendations) from **runtime cluster mutation** (`pods/resize`, GitOps PR automation). This document outlines STRIDE threat categories and the concrete controls enforced in code and Helm deployment defaults.

## 1. Trust Boundaries

1. **Developer / CI Boundary -> Manifest Diff (`kubectl-reconcilyx diff`)**
   - Reads YAML from workspace and `git show <base>:<path>`. No Kubernetes API credentials required; supports deterministic offline catalog evaluation (`--offline`).
2. **Browser / Operator Console & CLI -> Control Plane Hub API (`:8082`)**
   - **Zero-Trust Multi-Identity Inbound RBAC (`pkg/security/auth`):**
     - **Kubernetes `TokenReview` + `SubjectAccessReview`:** Default path for `kubectl-reconcilyx`, forwarding the active `kubeconfig` Bearer token (`X-Reconcilyx-Auth-Mode: k8s-token`). Admin requires `update pods/resize`; read-only requires `get pods`.
     - **Stateless OIDC / GitHub Actions JWKS JWT Verification:** Corporate OIDC (`RECONCILYX_OIDC_ISSUER_URL`, `RECONCILYX_OIDC_CLIENT_ID`) maps `groups` claims to `viewer`, `ci_auditor`, or `admin`. GitHub Actions OIDC (`https://token.actions.githubusercontent.com`) validates `repository` allowlists into `ci_auditor` (no static CI secrets).
     - **Scoped Hashed API Keys:** Prefix-tagged tokens (`rcx_admin_`, `rcx_ci_`, `rcx_viewer_`, `rcx_spoke_<cluster>_`) with optional `@RFC3339` TTL compared in constant time against SHA-256 hashes. Legacy `RECONCILYX_API_KEY` remains supported with a startup deprecation warning.
   - Route-level fail-closed RBAC enforces 4 roles (`viewer`, `ci_auditor`, `spoke_agent`, `admin`), rejecting cross-role mutations with `403 Forbidden`.
   - Protected by strict CORS allowlist (`RECONCILYX_CORS_ORIGINS`), Content-Security-Policy, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, conditional HSTS, 1 MiB `http.MaxBytesReader`, and per-IP token bucket rate limiting (`429 Too Many Requests`).
3. **Control Plane -> Managed Workloads (Cluster RBAC & Execution Gates)**
   - **Split-RBAC Default:** `charts/reconcilyx` deploys in **read-only mode** (`actuation.inPlaceResizeEnabled: false`, `config.enableInPlaceResizing: false`) with only `get/list/watch` on core workloads, HPA, VPA, and KEDA `ScaledObject`s.
   - Opt-in actuation grants strictly `pods/resize` (`patch`) rather than broad `pods` or `apps/deployments` writes.
   - **Namespace Deny-List (`RECONCILYX_EXCLUDED_NAMESPACES`):** `defaultExecutionGate` rejects actuation in `kube-system`, `kube-public`, `kube-node-lease`, `reconcilyx-system`, `flux-system`, and `argocd`.
   - **Autoscaler Conflict Gate:** Blocks in-place actuation when an HPA (`autoscaling/v2` CPU/Memory metrics), VPA (`Auto`/`Recreate`/`Initial`), or KEDA `ScaledObject` (`cpu`/`memory` triggers) manages the target workload.
   - **Memory OOM / Headroom Guard:** Asymmetric p99 memory headroom and a `>= 1.35 * workingSetBytes` in-place memory shrink check (`MemoryShrinkBlockedError`) prevent kubelet OOMKills.
4. **Spoke Clusters <-> Federation Hub (`/api/federated/*`, `/api/v1/federation/*`)**
   - Cryptographic identity via **mTLS SPIFFE URI SAN / CN client certificates** (`RECONCILYX_TLS_CLIENT_CA`) or cluster-scoped tokens (`rcx_spoke_<cluster>_*`) binds each caller to `RoleSpokeAgent` and its own `ClusterName`. Submitting deltas for another `clusterName` fails closed with `403 Forbidden`.
   - Poisoned or outlier spoke Q-table deltas are filtered using coordinate-wise trimmed-mean aggregation (`ByzantineTrimmedMean`) with monotonic snapshot versioning and SHA-256 checksum verification (`VerifyChecksum`).
5. **External Git Providers -> Auto-Merge Webhook (`:8090/webhook`) & Outbound GitOps PRs**
   - Validates HMAC-SHA256 (`X-Hub-Signature-256`) for GitHub and constant-time SHA-256 token comparison (`X-Gitlab-Token`) for GitLab.
   - Outbound GitHub PR creation supports **ephemeral GitHub App installation tokens** (`RECONCILYX_GITOPS_GITHUB_APP_ID`, `RECONCILYX_GITOPS_GITHUB_INSTALLATION_ID`, RS256 JWT assertion exchanging for 1-hour installation tokens) with PEM keys mounted read-only from tmpfs (`/var/run/secrets/reconcilyx/github/app_private_key.pem`).
   - Enforces 1 MiB body limit, duplicate delivery ID replay window (10m TTL via `X-GitHub-Delivery` / `X-Gitlab-Event-UUID`), optional `X-Reconcilyx-Timestamp` skew window (5m), and mandatory `reconcilyx/` head branch + embedded auto-merge policy marker before merging.
6. **External Cloud Pricing / Carbon APIs -> Outbound Secret Isolation & Workload Identity**
   - **`SecretString` Memory & Log Redaction (`pkg/security/credentials`):** API credentials implement `fmt.Stringer`, `fmt.GoStringer`, `json.Marshaler`, `TextMarshaler`, and `slog.LogValuer` returning `"[REDACTED]"` so crash dumps or structured logs never print secrets.
   - **File-Mounted Rotation (`fsnotify`):** Credentials resolve `/var/run/secrets/reconcilyx/<provider>/*` (`mode 0400` projected tmpfs) before environment fallbacks and hot-reload on atomic Kubernetes secret swaps with zero restarts.
   - **Keyless Cloud Identity:** AWS EC2 Pricing supports IRSA `AssumeRoleWithWebIdentity` (`AWS_ROLE_ARN` + `AWS_WEB_IDENTITY_TOKEN_FILE`); GCP Cloud Billing supports Workload Identity metadata OAuth2 tokens (`RECONCILYX_GCP_WORKLOAD_IDENTITY`).
   - System trust store + custom enterprise CA bundles (`RECONCILYX_CA_BUNDLE` / `SSL_CERT_FILE`) are honored with TLS verification always active (never `InsecureSkipVerify`). API failures fail closed or fall back deterministically to versioned bundled catalogs with explicit provenance.

## 2. Container & Supply-Chain Posture

- `Dockerfile` compiles the UI (`node:20-alpine`) and statically linked Go binary (`CGO_ENABLED=0`) into `gcr.io/distroless/static:nonroot` (`USER 65532:65532`, `HOME=/tmp`).
- Helm pod/container security contexts enforce `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, and `capabilities.drop: ["ALL"]`.
- CI runs `go vet`, unit/envtest suites with `-race`, Vite/Node UI tests, Helm linting + template assertions, and `govulncheck`.
