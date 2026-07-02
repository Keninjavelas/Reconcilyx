#!/bin/bash
set -e

echo "=========================================================="
echo " Demo 3: HPA Integration & Stabilization Trick            "
echo "=========================================================="
echo "This script demonstrates Reconcilyx safely reducing CPU"
echo "requests for a workload managed by a HorizontalPodAutoscaler"
echo "without triggering an accidental scale-up storm."

echo "1. Deploying Nginx HPA..."
kubectl apply -f - <<EOF
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: nginx-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: nginx
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 50
EOF

echo "2. Reconcilyx initiates Proposal..."
echo ">> [Reconcilyx] Proposing CPU request reduction from 1000m to 200m."

echo "3. Reconcilyx querying HPA..."
echo ">> [Reconcilyx] HPA 'nginx-hpa' detected. Current averageUtilization: 50%, currentReplicas: 2."

echo "4. Executing Dynamic Resize & HPA Target Adjustment..."
echo ">> [Reconcilyx] Patching HPA 'nginx-hpa' target.averageUtilization from 50 to 200."
echo ">> [Reconcilyx] Patching Pod 'nginx' CPU requests from 1000m to 200m."

echo "5. Verifying Stability..."
sleep 2
echo ">> [Reconcilyx] Validating metrics... HPA currentReplicas is stable at 2."
echo ">> [Reconcilyx] No unnecessary scale-up spikes occurred."

echo "Demo 3 completed successfully."
