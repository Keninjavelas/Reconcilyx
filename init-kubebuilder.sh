#!/bin/bash
set -e

# Install kubebuilder inside the container
curl -L -o kubebuilder "https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)"
chmod +x kubebuilder && mv kubebuilder /usr/local/bin/

# Initialize project
go mod init github.com/reconcilyx/operator
kubebuilder init --domain reconcilyx.io --repo github.com/reconcilyx/operator

# Create APIs
kubebuilder create api --group optimization --version v1alpha1 --kind OptimizationPolicy --resource --controller --make=false
kubebuilder create api --group optimization --version v1alpha1 --kind OptimizationProposal --resource --controller --make=false

go mod tidy
