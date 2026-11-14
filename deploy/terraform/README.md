# Reconcilyx Terraform / OpenTofu Deployment

Declarative infrastructure provisioning for deploying Reconcilyx into **AWS EKS**, **GCP GKE**, or **any conformant Kubernetes cluster**, with remote encrypted state locking, Pod Security Standards (`restricted`), Workload Identity, and atomic Helm rollouts.

## Features

- **Remote State Backends (`backend.tf.example`):** Preconfigured templates for AWS S3 + DynamoDB state locking and GCS native object locking.
- **Zero Static Cloud Secrets:** Supports AWS EKS IRSA (`eks.amazonaws.com/role-arn`) and GKE Workload Identity (`iam.gke.io/gcp-service-account`) attached directly to the controller ServiceAccount.
- **Namespace Hardening:** Creates the target namespace with `pod-security.kubernetes.io/enforce=restricted`.
- **Fail-Closed Least Privilege:** Defaults to `enable_in_place_resize = false` (`pods/resize` ClusterRole rule omitted).
- **Atomic Release Rollback:** Sets `atomic = true` and `cleanup_on_fail = true` on `helm_release.reconcilyx`.

## Quick Start

1. Configure remote state:
   ```bash
   cp backend.tf.example backend.tf
   # Edit backend.tf with your S3/DynamoDB or GCS bucket
   ```

2. Create a `terraform.tfvars` (never commit secrets — inject via `TF_VAR_bootstrap_scoped_api_keys` or External Secrets Operator):
   ```hcl
   cloud_provider            = "aws-eks"
   aws_irsa_role_arn         = "arn:aws:iam::123456789012:role/reconcilyx-controller"
   prometheus_url            = "http://prometheus-operated.monitoring.svc:9090"
   scoped_api_keys_secret_name = "reconcilyx-scoped-keys"
   replica_count             = 2
   enable_prometheus_rule    = true
   ```

3. Initialize, plan, and apply:
   ```bash
   terraform init
   terraform plan -out=tfplan
   terraform apply tfplan
   ```
