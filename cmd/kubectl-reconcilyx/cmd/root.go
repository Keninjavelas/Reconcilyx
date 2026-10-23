package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	apiServerURL string
	apiToken     string
	namespace    string
)

var RootCmd = &cobra.Command{
	Use:           "kubectl-reconcilyx",
	Short:         "kubectl plugin for Reconcilyx autonomous FinOps & carbon optimization",
	SilenceErrors: true,
	SilenceUsage:  true,
	Long: `kubectl reconcilyx allows cluster administrators and DevOps engineers to
scan Kubernetes workloads for resource waste, estimate carbon emission savings,
and inspect or approve AI-driven optimization proposals directly from the CLI.`,
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		code := 1
		var exitErr *exitCodeError
		if errors.As(err, &exitErr) {
			code = exitErr.code
		}
		os.Exit(code)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&apiServerURL, "server", "s", "http://localhost:8082", "Reconcilyx API server endpoint")
	RootCmd.PersistentFlags().StringVarP(&apiToken, "token", "t", "", "API key token for authentication")
	RootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Target Kubernetes namespace (default: all namespaces)")
}
