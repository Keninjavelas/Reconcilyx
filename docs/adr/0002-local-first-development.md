# ADR 002: Local-First Development
* **Context**: Cloud resources cost money, and complex setups deter users and interviewers from trying the project.
* **Decision**: The primary development and demo environment will be based on local tools (Kind or K3d, local Prometheus).
* **Rationale**: A portfolio project is useless if an interviewer can't run it easily. By providing a single `make demo` command that spins up a local cluster, injects traffic, and demonstrates reconciliation, we drastically increase the project's impact.
