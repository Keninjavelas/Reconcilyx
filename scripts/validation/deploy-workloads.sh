#!/bin/bash
set -e

echo "Deploying Validation Workloads for Reconcilyx Demo..."

# Create namespace
kubectl create namespace reconcilyx-demo --dry-run=client -o yaml | kubectl apply -f -

# 1. Overprovisioned CPU Workload
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cpu-hog
  namespace: reconcilyx-demo
  labels:
    app: cpu-hog
spec:
  replicas: 1
  selector:
    matchLabels:
      app: cpu-hog
  template:
    metadata:
      labels:
        app: cpu-hog
    spec:
      containers:
      - name: nginx
        image: nginx:latest
        resources:
          requests:
            cpu: "2000m"
            memory: "512Mi"
          limits:
            cpu: "2000m"
            memory: "512Mi"
EOF

# 2. Overprovisioned Memory Workload
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: memory-leak
  namespace: reconcilyx-demo
  labels:
    app: memory-leak
spec:
  replicas: 1
  selector:
    matchLabels:
      app: memory-leak
  template:
    metadata:
      labels:
        app: memory-leak
    spec:
      containers:
      - name: redis
        image: redis:alpine
        resources:
          requests:
            cpu: "100m"
            memory: "2Gi"
          limits:
            cpu: "100m"
            memory: "2Gi"
EOF

echo "Workloads deployed successfully in 'reconcilyx-demo' namespace."
