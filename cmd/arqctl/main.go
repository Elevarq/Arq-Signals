package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/elevarq/arq-signals/internal/safety"
)

var (
	apiAddr  string
	apiToken string
)

func main() {
	root := &cobra.Command{
		Use:   "arqctl",
		Short: "CLI for arq-signals",
	}

	defaultToken := os.Getenv("ARQ_SIGNALS_API_TOKEN")
	root.PersistentFlags().StringVar(&apiAddr, "api-addr", "http://127.0.0.1:8081", "arq-signals API address")
	root.PersistentFlags().StringVar(&apiToken, "api-token", defaultToken, "API bearer token (default: $ARQ_SIGNALS_API_TOKEN)")

	root.AddCommand(versionCmd())
	root.AddCommand(statusCmd())
	root.AddCommand(collectCmd())
	root.AddCommand(exportCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("arqctl %s (%s) built %s\n", safety.Version, safety.Commit, safety.BuildDate)
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show collector status",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := apiGet("/status")
			if err != nil {
				return fmt.Errorf("status request failed: %w", err)
			}
			defer resp.Body.Close()

			var data map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}

			out, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}
}

func collectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Collection management",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "now",
		Short: "Trigger immediate collection",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := apiRequestWithTimeout("POST", "/collect/now", nil, 10*time.Second)
			if err != nil {
				return fmt.Errorf("collect request failed: %w", err)
			}
			defer resp.Body.Close()

			var result map[string]any
			json.NewDecoder(resp.Body).Decode(&result)
			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	})

	return cmd
}

func exportCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export collected data as ZIP",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := apiGet("/export")
			if err != nil {
				return fmt.Errorf("export request failed: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("export failed: HTTP %d: %s", resp.StatusCode, body)
			}

			if output == "" {
				output = fmt.Sprintf("arq-export-%s.zip", time.Now().UTC().Format("20060102-150405"))
			}

			f, err := os.Create(output)
			if err != nil {
				return fmt.Errorf("create output file: %w", err)
			}
			defer f.Close()

			n, err := io.Copy(f, resp.Body)
			if err != nil {
				return fmt.Errorf("write export: %w", err)
			}

			fmt.Printf("Export saved to %s (%d bytes)\n", output, n)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path (default: arq-export-<timestamp>.zip)")
	return cmd
}

func apiRequestWithTimeout(method, path string, body io.Reader, timeout time.Duration) (*http.Response, error) {
	req, err := http.NewRequest(method, apiAddr+path, body)
	if err != nil {
		return nil, err
	}
	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}
	client := &http.Client{Timeout: timeout}
	return client.Do(req)
}

func apiGet(path string) (*http.Response, error) {
	return apiRequestWithTimeout("GET", path, nil, 30*time.Second)
}
