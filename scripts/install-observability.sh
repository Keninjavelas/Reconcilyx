#!/bin/bash
set -e

echo "Adding Prometheus community Helm repository..."
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

echo "Installing kube-prometheus-stack..."
# Pinning version for repeatable setup
# Disabling alertmanager, grafana default dashboards, etc., to keep local footprint small
helm upgrade --install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --version 69.4.1 \
  --set alertmanager.enabled=false \
  --set grafana.enabled=true \
  --set prometheus.prometheusSpec.scrapeInterval="15s" \
  --set prometheus.prometheusSpec.evaluationInterval="15s" \
  --set kubeStateMetrics.enabled=true \
  --set nodeExporter.enabled=true \
  --wait

echo "Observability stack installed successfully."
