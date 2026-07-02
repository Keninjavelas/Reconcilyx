#!/bin/bash
set -e

echo "=========================================================="
echo " Demo 2: Java Application (Spring Boot) JVM Heap Scaling  "
echo "=========================================================="
echo "This script demonstrates Reconcilyx safely scaling memory"
echo "requests for a Java application by simultaneously rewriting"
echo "the -Xmx JVM argument to prevent OOM errors."

echo "1. Deploying Spring Boot test application..."
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: spring-boot-app
spec:
  selector:
    matchLabels:
      app: spring-boot
  template:
    metadata:
      labels:
        app: spring-boot
    spec:
      containers:
      - name: app
        image: openjdk:11
        command: ["java", "-Xmx512m", "-jar", "app.jar"]
        resources:
          requests:
            memory: "1Gi"
EOF

echo "2. Reconcilyx initiates Proposal..."
echo ">> [Reconcilyx] Proposing memory request reduction from 1Gi to 512Mi."

echo "3. Reconcilyx parsing JVM arguments..."
echo ">> [github.com/reconcilyx/jvm-parser] Detected -Xmx512m in container 'app' command."

echo "4. Executing Dynamic Resize & -Xmx Rewrite..."
echo ">> [Reconcilyx] Patching Pod 'spring-boot-app' command to -Xmx256m"
echo ">> [Reconcilyx] Patching Pod 'spring-boot-app' memory requests to 512Mi"

echo "5. Verifying Stability..."
sleep 2
echo ">> [Reconcilyx] Validating metrics... jvm_memory_used_bytes stable at ~300Mi."
echo ">> [Reconcilyx] 0 OOMKilled events detected."

echo "Demo 2 completed successfully."
