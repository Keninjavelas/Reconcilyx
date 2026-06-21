#!/bin/bash
set -e

SCRIPT_DIR=$(dirname "$0")

echo "Resetting cluster..."
"${SCRIPT_DIR}/cluster-down.sh"
"${SCRIPT_DIR}/cluster-up.sh"
echo "Cluster reset complete."
