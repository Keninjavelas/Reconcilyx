package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	llmProvider string
	llmKey      string
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start interactive FinOps ChatOps session with Reconcilyx AI Assistant",
	Long: `Start an interactive natural language chat session directly from the CLI.
Ask questions about cluster costs, carbon footprint, workload forecasts, and cross-cloud migration options.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("🤖 Reconcilyx FinOps AI ChatOps Terminal Session")
		fmt.Printf("   Provider: %s | API Server: %s\n", llmProvider, apiServerURL)
		fmt.Println("   Type 'exit' or 'quit' to end session.")
		fmt.Println()

		scanner := bufio.NewScanner(os.Stdin)
		client := &http.Client{Timeout: 30 * time.Second}

		for {
			fmt.Print("reconcilyx-ai > ")
			if !scanner.Scan() {
				break
			}
			query := strings.TrimSpace(scanner.Text())
			if query == "exit" || query == "quit" {
				fmt.Println("Goodbye! 👋")
				break
			}
			if query == "" {
				continue
			}

			payload := map[string]string{
				"message":  query,
				"provider": llmProvider,
				"apiKey":   llmKey,
			}
			data, _ := json.Marshal(payload)

			req, err := http.NewRequest("POST", apiServerURL+"/api/chat", bytes.NewReader(data))
			if err != nil {
				fmt.Printf("Error building request: %v\n", err)
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			if apiToken != "" {
				req.Header.Set("X-API-Key", apiToken)
			}

			resp, err := client.Do(req)
			var res struct {
				Answer               string `json:"answer"`
				Function             string `json:"function"`
				FastPath             bool   `json:"fastPath"`
				ConfirmationRequired bool   `json:"confirmationRequired"`
				ConfirmationToken    string `json:"confirmationToken"`
				ConfirmationDesc     string `json:"confirmationDescription"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
				resp.Body.Close()
				fmt.Printf("⚠️ Failed to parse API server response: %v\n\n", err)
				continue
			}
			resp.Body.Close()

			if res.Answer != "" {
				fmt.Printf("\n%s\n\n", res.Answer)
			}

			if res.ConfirmationRequired && res.ConfirmationToken != "" {
				fmt.Printf("⚠️  Safety Confirmation Required:\n    %s\n", res.ConfirmationDesc)
				fmt.Print("Proceed with execution? [y/N]: ")
				if scanner.Scan() {
					ans := strings.TrimSpace(strings.ToLower(scanner.Text()))
					if ans == "y" || ans == "yes" {
						confirmPayload := map[string]string{
							"message":      query,
							"provider":     llmProvider,
							"apiKey":       llmKey,
							"confirmToken": res.ConfirmationToken,
						}
						cData, _ := json.Marshal(confirmPayload)
						cReq, cErr := http.NewRequest("POST", apiServerURL+"/api/chat", bytes.NewReader(cData))
						if cErr != nil {
							fmt.Printf("⚠️ Error building confirmation request: %v\n\n", cErr)
							continue
						}
						cReq.Header.Set("Content-Type", "application/json")
						if apiToken != "" {
							cReq.Header.Set("X-API-Key", apiToken)
						}
						cResp, cErr := client.Do(cReq)
						if cErr != nil {
							fmt.Printf("⚠️ Confirmation execution failed: %v\n\n", cErr)
							continue
						}
						var cRes struct {
							Answer string `json:"answer"`
						}
						_ = json.NewDecoder(cResp.Body).Decode(&cRes)
						cResp.Body.Close()
						fmt.Printf("\n%s\n\n", cRes.Answer)
					} else {
						fmt.Println("Action cancelled.")
					}
				}
			}
		}

		return nil
	},
}

func init() {
	chatCmd.Flags().StringVarP(&llmProvider, "provider", "p", "ollama", "LLM Provider (ollama, openai, anthropic, groq, cohere)")
	chatCmd.Flags().StringVarP(&llmKey, "llm-key", "k", "", "API key for cloud LLM provider")
	RootCmd.AddCommand(chatCmd)
}
