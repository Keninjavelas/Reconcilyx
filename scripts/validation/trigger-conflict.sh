#!/bin/bash
set -e

echo "Simulating GitOps / Human Conflict..."

# Forcefully overwrite the CPU requests on a workload currently being optimized by Reconcilyx
# By using field-manager "kubectl-edit" (the default for this command), we seize ownership.

kubectl patch deployment cpu-hog -n reconcilyx-demo --type='json' -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources/requests/cpu", "value":"1500m"}]'

echo "Conflict injected. Reconcilyx Stabilization phase should detect drift and yield."
