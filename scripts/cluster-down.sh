#!/bin/bash
set -e

CLUSTER_NAME="reconcilyx-dev"

echo "Deleting Kind cluster '${CLUSTER_NAME}'..."
kind delete cluster --name "${CLUSTER_NAME}"
echo "Cluster '${CLUSTER_NAME}' deleted."
