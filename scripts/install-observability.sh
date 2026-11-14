#!/bin/bash
set -e

GRAFANA_ENABLED="${OBSERVABILITY_GRAFANA_ENABLED:-true}"
WAIT_TIMEOUT="${OBSERVABILITY_WAIT_TIMEOUT:-10m}"

echo "Adding Prometheus community Helm repository..."
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

echo "Installing kube-prometheus-stack..."
# Pinning version for repeatable setup
# Disabling alertmanager, and optionally Grafana, to keep the footprint small
helm upgrade --install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --version 69.4.1 \
  --set alertmanager.enabled=false \
  --set grafana.enabled="${GRAFANA_ENABLED}" \
  --set prometheus.prometheusSpec.scrapeInterval="15s" \
  --set prometheus.prometheusSpec.evaluationInterval="15s" \
  --set prometheus.prometheusSpec.resources.requests.cpu="50m" \
  --set prometheus.prometheusSpec.resources.requests.memory="256Mi" \
  --set prometheusOperator.resources.requests.cpu="25m" \
  --set prometheusOperator.resources.requests.memory="64Mi" \
  --set prometheus-node-exporter.resources.requests.cpu="10m" \
  --set prometheus-node-exporter.resources.requests.memory="32Mi" \
  --set kube-state-metrics.resources.requests.cpu="10m" \
  --set kube-state-metrics.resources.requests.memory="32Mi" \
  --set kubeStateMetrics.enabled=true \
  --set nodeExporter.enabled=true \
  --wait --timeout "${WAIT_TIMEOUT}"

# The operator defaults to http://prometheus-k8s.monitoring.svc:9090, but the
# Helm chart names the Prometheus service prometheus-kube-prometheus-prometheus.
# Keep a stable alias so installing the stack is sufficient for telemetry.
echo "Creating the prometheus-k8s service alias..."
kubectl apply -f - <<'EOF'
apiVersion: v1
kind: Service
metadata:
  name: prometheus-k8s
  namespace: monitoring
  labels:
    app.kubernetes.io/name: prometheus
    app.kubernetes.io/part-of: reconcilyx-observability
spec:
  type: ClusterIP
  ports:
    - name: http-web
      port: 9090
      targetPort: 9090
  selector:
    operator.prometheus.io/name: prometheus-kube-prometheus-prometheus
EOF

echo "Observability stack installed successfully."
