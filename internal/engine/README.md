# Reconcilyx Engine

The `engine` package is the core intelligence of Reconcilyx. It is responsible for:
1. Fetching metrics via the `telemetry` package.
2. Comparing observed metrics against defined limits/requests.
3. Identifying overprovisioned workloads or regressions.
4. Generating `OptimizationProposal` objects.

**MVP Note:** Do not implement logic yet. This is a placeholder for future iterations.
