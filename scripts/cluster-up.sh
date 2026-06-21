#!/bin/bash
set -e

CLUSTER_NAME="reconcilyx-dev"

echo "Checking if Kind cluster '${CLUSTER_NAME}' exists..."
if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
  echo "Cluster '${CLUSTER_NAME}' already exists."
else
  echo "Creating Kind cluster '${CLUSTER_NAME}'..."
  kind create cluster --name "${CLUSTER_NAME}"
fi

echo "Installing metrics-server (required for telemetry)..."
# Using the insecure-tls version for local kind environments
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl patch -n kube-system deployment metrics-server --type=json \
  -p '[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls"}]'

echo "Creating essential namespaces..."
kubectl create namespace reconcilyx-system --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -

echo "Waiting for metrics-server to be ready..."
kubectl wait --for=condition=available deployment/metrics-server -n kube-system --timeout=120s

echo "Cluster '${CLUSTER_NAME}' is up and ready."
