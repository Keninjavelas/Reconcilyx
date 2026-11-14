provider "kubernetes" {
  config_path    = var.kubeconfig_path
  config_context = var.kubeconfig_context != "" ? var.kubeconfig_context : null
}

provider "helm" {
  kubernetes {
    config_path    = var.kubeconfig_path
    config_context = var.kubeconfig_context != "" ? var.kubeconfig_context : null
  }
}

locals {
  workload_identity_annotations = merge(
    var.cloud_provider == "aws-eks" && var.aws_irsa_role_arn != "" ? {
      "eks.amazonaws.com/role-arn" = var.aws_irsa_role_arn
    } : {},
    var.cloud_provider == "gcp-gke" && var.gcp_service_account_email != "" ? {
      "iam.gke.io/gcp-service-account" = var.gcp_service_account_email
    } : {},
  )

  bootstrap_secret_name = var.bootstrap_scoped_api_keys != "" ? kubernetes_secret_v1.scoped_keys[0].metadata[0].name : var.scoped_api_keys_secret_name

  helm_values = {
    replicaCount = var.replica_count
    image = {
      repository = var.image_repository
      tag        = var.image_tag
      digest     = var.image_digest
    }
    serviceAccount = {
      create      = true
      annotations = local.workload_identity_annotations
    }
    config = {
      prometheusUrl         = var.prometheus_url
      enableInPlaceResizing = var.enable_in_place_resize
    }
    actuation = {
      inPlaceResizeEnabled = var.enable_in_place_resize
    }
    auth = {
      scopedKeys = {
        secretName = local.bootstrap_secret_name
      }
    }
    federation = {
      enabled     = var.federation_role != "standalone"
      role        = var.federation_role
      clusterName = var.cluster_name
    }
    pdb = {
      enabled      = var.replica_count > 1
      minAvailable = 1
    }
    networkPolicy = {
      enabled = var.enable_network_policy
    }
    monitoring = {
      prometheusRule = {
        enabled = var.enable_prometheus_rule
      }
    }
  }
}

resource "kubernetes_namespace_v1" "reconcilyx" {
  metadata {
    name = var.namespace
    labels = {
      "app.kubernetes.io/part-of"                   = "reconcilyx"
      "pod-security.kubernetes.io/enforce"          = "restricted"
      "pod-security.kubernetes.io/enforce-version"  = "latest"
      "pod-security.kubernetes.io/audit"            = "restricted"
      "pod-security.kubernetes.io/warn"             = "restricted"
    }
  }
}

resource "kubernetes_secret_v1" "scoped_keys" {
  count = var.bootstrap_scoped_api_keys != "" ? 1 : 0

  metadata {
    name      = "${var.release_name}-scoped-keys"
    namespace = kubernetes_namespace_v1.reconcilyx.metadata[0].name
    labels = {
      "app.kubernetes.io/part-of" = "reconcilyx"
    }
  }

  type = "Opaque"

  data = {
    RECONCILYX_SCOPED_API_KEYS = var.bootstrap_scoped_api_keys
  }
}

resource "helm_release" "reconcilyx" {
  name             = var.release_name
  namespace        = kubernetes_namespace_v1.reconcilyx.metadata[0].name
  chart            = var.helm_chart_path
  atomic           = true
  cleanup_on_fail  = true
  wait             = true
  timeout          = 300
  create_namespace = false

  values = [yamlencode(local.helm_values)]

  depends_on = [
    kubernetes_namespace_v1.reconcilyx,
    kubernetes_secret_v1.scoped_keys,
  ]
}
