output "namespace" {
  description = "Namespace where Reconcilyx is deployed with restricted Pod Security Standards"
  value       = kubernetes_namespace_v1.reconcilyx.metadata[0].name
}

output "helm_release_name" {
  description = "Deployed Helm release name"
  value       = helm_release.reconcilyx.name
}

output "helm_release_revision" {
  description = "Current Helm release revision number"
  value       = helm_release.reconcilyx.revision
}

output "workload_identity_annotations" {
  description = "ServiceAccount annotations configured for AWS IRSA or GCP Workload Identity"
  value       = local.workload_identity_annotations
}

output "actuation_mode" {
  description = "Whether live in-place pods/resize RBAC is enabled"
  value       = var.enable_in_place_resize ? "active-resize-enabled" : "shift-left-read-only"
}
