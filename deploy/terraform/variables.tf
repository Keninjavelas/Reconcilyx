variable "cloud_provider" {
  description = "Target cluster identity/deployment mode: aws-eks, gcp-gke, or generic-kubeconfig"
  type        = string
  default     = "generic-kubeconfig"

  validation {
    condition     = contains(["aws-eks", "gcp-gke", "generic-kubeconfig"], var.cloud_provider)
    error_message = "cloud_provider must be aws-eks, gcp-gke, or generic-kubeconfig."
  }
}

variable "kubeconfig_path" {
  description = "Path to kubeconfig when authenticating via kubeconfig context"
  type        = string
  default     = "~/.kube/config"
}

variable "kubeconfig_context" {
  description = "Optional kubeconfig context override"
  type        = string
  default     = ""
}

variable "namespace" {
  description = "Kubernetes namespace for Reconcilyx (enforces restricted Pod Security Standards)"
  type        = string
  default     = "reconcilyx-system"
}

variable "release_name" {
  description = "Helm release name"
  type        = string
  default     = "reconcilyx"
}

variable "helm_chart_path" {
  description = "Path to local Reconcilyx Helm chart (or OCI/HTTPS chart reference)"
  type        = string
  default     = "../../charts/reconcilyx"
}

variable "replica_count" {
  description = "Number of Reconcilyx controller/API replicas (use >= 2 for Hub HA with PDB)"
  type        = number
  default     = 1

  validation {
    condition     = var.replica_count >= 1 && var.replica_count <= 10
    error_message = "replica_count must be between 1 and 10."
  }
}

variable "image_repository" {
  description = "Container image repository for Reconcilyx manager"
  type        = string
  default     = "ghcr.io/reconcilyx/reconcilyx"
}

variable "image_tag" {
  description = "Container image tag or digest-qualified tag"
  type        = string
  default     = "4.1.0-rc1"
}

variable "image_digest" {
  description = "Optional immutable sha256 digest (e.g. sha256:...) pinned alongside tag"
  type        = string
  default     = ""
}

variable "aws_irsa_role_arn" {
  description = "IAM Role ARN for EKS IRSA (used when cloud_provider == aws-eks)"
  type        = string
  default     = ""
}

variable "gcp_service_account_email" {
  description = "Google Service Account email for GKE Workload Identity (used when cloud_provider == gcp-gke)"
  type        = string
  default     = ""
}

variable "prometheus_url" {
  description = "Prometheus HTTP endpoint URL consumed by the Reconcilyx telemetry scraper"
  type        = string
  default     = ""
}

variable "enable_in_place_resize" {
  description = "Opt-in toggle granting pods/resize patch RBAC and enabling live actuation (default false = Shift-Left/Advisory mode)"
  type        = bool
  default     = false
}

variable "enable_network_policy" {
  description = "Deploy Kubernetes NetworkPolicy enforcing default-deny ingress/restricted egress"
  type        = bool
  default     = true
}

variable "enable_prometheus_rule" {
  description = "Deploy PrometheusRule SLO/pipeline alerts (requires monitoring.coreos.com/v1 CRD)"
  type        = bool
  default     = false
}

variable "scoped_api_keys_secret_name" {
  description = "Existing Kubernetes Secret (e.g. synced by External Secrets Operator / Vault) holding RECONCILYX_SCOPED_API_KEYS"
  type        = string
  default     = ""
}

variable "bootstrap_scoped_api_keys" {
  description = "Optional initial comma-separated scoped keys when External Secrets is not managing secrets (omit in production gitops)"
  type        = string
  default     = ""
  sensitive   = true
}

variable "federation_role" {
  description = "Federation mode: standalone, hub, or spoke"
  type        = string
  default     = "standalone"

  validation {
    condition     = contains(["standalone", "hub", "spoke"], var.federation_role)
    error_message = "federation_role must be standalone, hub, or spoke."
  }
}

variable "cluster_name" {
  description = "Logical cluster identifier reported in federation and audit records"
  type        = string
  default     = "prod-cluster"
}
