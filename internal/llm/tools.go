package llm

func GetReconcilyxTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "getProposals",
				Description: "List active optimization proposals and recommendations across workloads.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"status": map[string]interface{}{
							"type":        "string",
							"description": "Filter by status: PENDING, APPROVED, APPLIED, REJECTED",
						},
						"namespace": map[string]interface{}{
							"type":        "string",
							"description": "Filter by Kubernetes namespace",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "getProposal",
				Description: "Get detailed information for a specific optimization proposal by name.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Name of the proposal",
						},
					},
					"required": []string{"name"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "getSavings",
				Description: "Get aggregate monthly cost ($) and carbon footprint (kg CO2e) savings metrics.",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "forecastWorkload",
				Description: "Predict future resource requirements for a specific workload with 95% Confidence Interval bounds.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"workload": map[string]interface{}{
							"type":        "string",
							"description": "Target workload name (e.g. payment-service)",
						},
					},
					"required": []string{"workload"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "evaluateArbitrage",
				Description: "Evaluate multi-cloud price and carbon matrix (AWS, GCP, Azure) for workload cloud migration.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"workload": map[string]interface{}{
							"type":        "string",
							"description": "Workload name to evaluate",
						},
					},
					"required": []string{"workload"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "getAnomalies",
				Description: "List recent resource spikes and anomalous behavior detected in workloads.",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "getPolicies",
				Description: "List optimization policies and multi-objective RL reward weights.",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "approveProposal",
				Description: "Approve and execute a pending optimization proposal.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Name of the proposal to approve",
						},
					},
					"required": []string{"name"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "rejectProposal",
				Description: "Reject a pending optimization proposal.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Name of the proposal to reject",
						},
					},
					"required": []string{"name"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "rollbackProposal",
				Description: "Rollback an applied resource proposal to its pre-actuation baseline state.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Name of the proposal to rollback",
						},
					},
					"required": []string{"name"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "getCarbon",
				Description: "Get carbon footprint intensity and abatement report for the cluster.",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "getClusters",
				Description: "List all registered Kubernetes clusters in the federated network.",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "getWorkloads",
				Description: "List running workloads in a specific namespace.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"namespace": map[string]interface{}{
							"type":        "string",
							"description": "Target namespace",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "getMetrics",
				Description: "Get raw Prometheus telemetry metrics for a workload.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"workload": map[string]interface{}{
							"type":        "string",
							"description": "Workload name",
						},
					},
					"required": []string{"workload"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "explain",
				Description: "Explain Kubernetes FinOps concept, In-Place Pod Resizing, Q-learning, or Carbon awareness.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"topic": map[string]interface{}{
							"type":        "string",
							"description": "Concept topic to explain",
						},
					},
					"required": []string{"topic"},
				},
			},
		},
	}
}
