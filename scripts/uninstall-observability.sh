#!/bin/bash
set -e

echo "Uninstalling kube-prometheus-stack..."
helm uninstall prometheus --namespace monitoring || true

echo "Removing the prometheus-k8s service alias..."
kubectl delete service prometheus-k8s --namespace monitoring --ignore-not-found

echo "Removing leftover CRDs (Prometheus Operator)..."
kubectl delete crd \
  alertmanagerconfigs.monitoring.coreos.com \
  alertmanagers.monitoring.coreos.com \
  podmonitors.monitoring.coreos.com \
  probes.monitoring.coreos.com \
  prometheusagents.monitoring.coreos.com \
  prometheuses.monitoring.coreos.com \
  prometheusrules.monitoring.coreos.com \
  scrapeconfigs.monitoring.coreos.com \
  servicemonitors.monitoring.coreos.com \
  thanosrulers.monitoring.coreos.com || true

echo "Observability stack uninstalled successfully."
