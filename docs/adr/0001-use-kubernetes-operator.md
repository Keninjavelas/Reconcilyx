# ADR 001: Building as a Kubernetes Operator
* **Context**: We need a way to continuously monitor state, evaluate rules, and apply changes to Kubernetes workloads.
* **Decision**: We will build Reconcilyx as a native Kubernetes Operator using Golang and Kubebuilder / controller-runtime.
* **Rationale**: This is the idiomatic way to extend Kubernetes. It gives us free leader election, robust client libraries, caching (informers), and natural integration with the K8s API (CRDs). Writing standalone scripts or external cronjobs lacks the reliability and event-driven nature of a controller.
