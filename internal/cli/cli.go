package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"metastackr/internal/db"
	"metastackr/internal/gitutils"
)

var (
	jsonOutput bool

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			MarginBottom(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	cellStyle = lipgloss.NewStyle().
			Padding(0, 1)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFB100")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF4672")).
			Bold(true)
)

type CLIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func printJSON(success bool, message string, data interface{}) {
	resp := CLIResponse{
		Success: success,
		Message: message,
		Data:    data,
	}
	bytes, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Println(string(bytes))
}

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "git-meta",
		Short: "MetaStackR CLI for Git submodules and PR synchronization",
	}

	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output machine-readable JSON")

	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newCheckoutCmd())
	rootCmd.AddCommand(newCommitCmd())
	rootCmd.AddCommand(newPushCmd())
	rootCmd.AddCommand(newSyncCmd())
	rootCmd.AddCommand(newRebaseCmd())
	rootCmd.AddCommand(newRetryMergeCmd())
	rootCmd.AddCommand(newInstallHooksCmd())
	rootCmd.AddCommand(newAgentsCmd())
	rootCmd.AddCommand(newSetupWebhookCmd())
	rootCmd.AddCommand(newInitCmd())

	return rootCmd
}

func newStatusCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Display local Git submodule status merged with remote backend PR/CI status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			localStatus, err := gitutils.GetLocalStatus(cwd)
			if err != nil {
				if jsonOutput {
					printJSON(false, err.Error(), nil)
					return nil
				}
				return fmt.Errorf("failed to read local git status: %w", err)
			}

			metaRepoName, _ := gitutils.GetMetaRepoName(cwd)
			if metaRepoName == "" {
				metaRepoName = "meta-repo"
			}

			var remoteMetaPR *db.MetaPR
			if serverURL != "" {
				remoteMetaPR, _ = fetchRemotePRStatus(serverURL, metaRepoName, localStatus.MetaBranch)
			}

			if jsonOutput {
				printJSON(true, "", map[string]interface{}{
					"local_status": localStatus,
					"remote_pr":    remoteMetaPR,
				})
				return nil
			}

			fmt.Println(titleStyle.Render("⚡ MetaStackR Status"))
			fmt.Printf("Meta Repo: %s | Branch: %s\n\n",
				lipgloss.NewStyle().Bold(true).Render(metaRepoName),
				lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Render(localStatus.MetaBranch),
			)

			renderMergedTable(localStatus, remoteMetaPR)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", os.Getenv("METASTACKR_SERVER_URL"), "MetaStackr backend server URL")
	if serverURL == "" {
		serverURL = "https://api.metastac.kr"
		cmd.Flags().Set("server", serverURL)
	}

	return cmd
}

func fetchRemotePRStatus(serverURL, repo, branch string) (*db.MetaPR, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	url := fmt.Sprintf("%s/api/v1/prs/status?repo=%s&branch=%s", serverURL, repo, branch)

	resp, err := client.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, err
	}
	defer resp.Body.Close()

	var res struct {
		MetaPR *db.MetaPR `json:"meta_pr"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.MetaPR, nil
}

func renderMergedTable(local *gitutils.MetaLocalStatus, remote *db.MetaPR) {
	fmt.Printf("%-20s | %-12s | %-14s | %-8s | %-10s\n",
		headerStyle.Render("Submodule Path"),
		headerStyle.Render("Local Branch"),
		headerStyle.Render("Local Drift"),
		headerStyle.Render("Child PR"),
		headerStyle.Render("PR Status"),
	)
	fmt.Println(strings.Repeat("-", 80))

	if len(local.Submodules) == 0 {
		fmt.Println(cellStyle.Render("No submodules found in current workspace."))
		return
	}

	childPRMap := make(map[string]db.ChildPR)
	if remote != nil {
		for _, c := range remote.ChildPRs {
			childPRMap[c.SubmodulePath] = c
		}
	}

	for _, sub := range local.Submodules {
		drift := successStyle.Render("CLEAN")
		if sub.HasUncommitted && sub.UnpushedCommits > 0 {
			drift = errorStyle.Render("DIRTY+UNPUSHED")
		} else if sub.HasUncommitted {
			drift = warningStyle.Render("UNCOMMITTED")
		} else if sub.UnpushedCommits > 0 {
			drift = warningStyle.Render("UNPUSHED (" + strconv.Itoa(sub.UnpushedCommits) + ")")
		}

		prText := "-"
		statusText := "-"

		if child, exists := childPRMap[sub.Path]; exists {
			prText = fmt.Sprintf("#%d", child.PRNumber)
			statusText = child.Status
		}

		fmt.Printf("%-20s | %-12s | %-14s | %-8s | %-10s\n",
			cellStyle.Render(sub.Path),
			cellStyle.Render(sub.Branch),
			drift,
			cellStyle.Render(prText),
			cellStyle.Render(statusText),
		)
	}

	fmt.Println()
	if remote != nil {
		fmt.Printf("Backend Meta PR Status: %s (Lock Version: %d)\n",
			lipgloss.NewStyle().Bold(true).Render(remote.Status),
			remote.LockVersion,
		)
	} else {
		fmt.Println("Backend PR Status: Server unreachable or no active Meta PR tracked.")
	}
}

func newCheckoutCmd() *cobra.Command {
	var createFlag bool

	cmd := &cobra.Command{
		Use:   "checkout <branch-name>",
		Short: "Safely switch or create branches across root meta-repo and all submodules",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branchName := args[0]
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			err = gitutils.CheckoutBranch(cwd, branchName, createFlag)
			if err != nil {
				if jsonOutput {
					printJSON(false, err.Error(), nil)
					return nil
				}
				return err
			}

			if jsonOutput {
				printJSON(true, fmt.Sprintf("Switched to branch '%s' across all submodules", branchName), nil)
			} else {
				fmt.Printf("✅ Switched to branch '%s' across parent and submodules.\n", branchName)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&createFlag, "create", "b", false, "Create new branch")
	return cmd
}

func newCommitCmd() *cobra.Command {
	var message string

	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Creates coordinated atomic commits in all modified submodules and parent repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			err = gitutils.CommitAtomic(cwd, message)
			if err != nil {
				if jsonOutput {
					printJSON(false, err.Error(), nil)
					return nil
				}
				return err
			}

			if jsonOutput {
				printJSON(true, "Coordinated commit completed successfully", nil)
			} else {
				fmt.Println("✅ Coordinated commit completed successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Commit message")
	_ = cmd.MarkFlagRequired("message")

	return cmd
}

func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push",
		Short: "Enforce bottom-up pushing (pushes all dirty submodules before root meta-repo commit pointers)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			err = gitutils.PushBottomUp(cwd)
			if err != nil {
				if jsonOutput {
					printJSON(false, err.Error(), nil)
					return nil
				}
				return err
			}

			if jsonOutput {
				printJSON(true, "Bottom-up push completed successfully", nil)
			} else {
				fmt.Println("✅ Bottom-up push completed successfully.")
			}
			return nil
		},
	}
}

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Fetches origin/main, fast-forwards/rebases local submodules, and aligns root pointers",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			err = gitutils.SyncUpstream(cwd)
			if err != nil {
				if jsonOutput {
					printJSON(false, err.Error(), nil)
					return nil
				}
				return err
			}

			if jsonOutput {
				printJSON(true, "Upstream sync completed successfully", nil)
			} else {
				fmt.Println("✅ Upstream sync completed successfully.")
			}
			return nil
		},
	}
}

func newRebaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rebase <upstream-branch>",
		Short: "Conducts a two-phase rebase: rebases child submodules first, then parent meta-repo references",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			upstream := args[0]
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			err = gitutils.RebaseUpstream(cwd, upstream)
			if err != nil {
				if jsonOutput {
					printJSON(false, err.Error(), nil)
					return nil
				}
				return err
			}

			if jsonOutput {
				printJSON(true, fmt.Sprintf("Rebase onto '%s' completed successfully", upstream), nil)
			} else {
				fmt.Printf("✅ Rebase onto '%s' completed successfully.\n", upstream)
			}
			return nil
		},
	}

	return cmd
}

func newRetryMergeCmd() *cobra.Command {
	var serverURL string
	var prNumber int

	cmd := &cobra.Command{
		Use:   "retry-merge",
		Short: "Re-trigger cascade merge on a partially-failed Meta PR",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			metaRepoName, err := gitutils.GetMetaRepoName(cwd)
			if err != nil || metaRepoName == "" {
				return fmt.Errorf("could not determine meta-repo origin: %w", err)
			}

			reqPayload := map[string]interface{}{
				"meta_repo": metaRepoName,
				"pr_number": prNumber,
			}
			jsonBytes, _ := json.Marshal(reqPayload)

			resp, err := http.Post(serverURL+"/api/v1/prs/retry-merge", "application/json", bytes.NewReader(jsonBytes))
			if err != nil {
				if jsonOutput {
					printJSON(false, err.Error(), nil)
					return nil
				}
				return fmt.Errorf("failed to contact backend server: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errStr := fmt.Sprintf("backend server returned HTTP %d", resp.StatusCode)
				if jsonOutput {
					printJSON(false, errStr, nil)
					return nil
				}
				return fmt.Errorf("%s", errStr)
			}

			if jsonOutput {
				printJSON(true, "Cascade merge retry initiated successfully", nil)
			} else {
				fmt.Println("🚀 Cascade merge retry initiated successfully!")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "https://api.metastac.kr", "MetaStackr backend server URL")
	cmd.Flags().IntVarP(&prNumber, "pr", "p", 0, "Meta PR number to retry")
	_ = cmd.MarkFlagRequired("pr")

	return cmd
}

func newAgentsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "agents",
		Short: "Print instructions and guidelines for AI agents in MetaStackr workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOutput {
				printJSON(true, "", map[string]interface{}{
					"rules": []string{
						"Do NOT run raw git checkout or git commit commands directly inside nested submodule directories.",
						"Always supply --json to git-meta CLI commands for deterministic state parsing.",
					},
					"operations": map[string]string{
						"status":   "git meta status --json",
						"checkout": "git meta checkout -b <branch-name> --json",
						"commit":   "git meta commit -m \"<msg>\" --json",
						"sync":     "git meta sync --json",
					},
				})
				return nil
			}

			fmt.Println(`# Repository Agent Guidelines

This repository is a **Meta-Repo** managed by ` + "`" + `git-meta` + "`" + `.

## Rules for AI Agents
- Do NOT run raw ` + "`" + `git checkout` + "`" + ` or ` + "`" + `git commit` + "`" + ` commands directly inside nested submodule directories.
- Always supply ` + "`" + `--json` + "`" + ` to ` + "`" + `git-meta` + "`" + ` CLI commands for deterministic state parsing.

## Key Operations
- Inspect state: ` + "`" + `git meta status --json` + "`" + `
- Switch/create branches: ` + "`" + `git meta checkout -b <branch-name> --json` + "`" + `
- Commit changes across system: ` + "`" + `git meta commit -m "<msg>" --json` + "`" + `
- Sync upstream changes: ` + "`" + `git meta sync --json` + "`")
			return nil
		},
	}
}

func newSetupWebhookCmd() *cobra.Command {
	var targetURL string
	var secret string

	cmd := &cobra.Command{
		Use:   "setup-webhook",
		Short: "Automate repository webhook registration with GitHub",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			if secret == "" {
				secret = os.Getenv("WEBHOOK_SECRET")
				if secret == "" {
					secret = "ms-secret-" + uuid.New().String()[:12]
				}
			}

			err = gitutils.RegisterGitHubWebhook(cwd, targetURL, secret, "")
			if err != nil {
				if jsonOutput {
					printJSON(false, err.Error(), nil)
				}
				return err
			}

			if jsonOutput {
				printJSON(true, "Webhook registered successfully", map[string]string{
					"target_url":     targetURL,
					"webhook_secret": secret,
				})
			} else {
				fmt.Printf("✅ Webhook registered successfully!\n")
				fmt.Printf("URL:    %s\n", targetURL)
				fmt.Printf("Secret: %s\n", secret)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&targetURL, "url", "https://api.metastac.kr/webhooks/github", "The webhook target URL")
	cmd.Flags().StringVar(&secret, "secret", "", "Optional webhook signature verification secret key")

	return cmd
}

func newInitCmd() *cobra.Command {
	var serverURL string
	var webhookURL string
	var secret string
	var allowCodePull bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize and onboard a repository to MetaStackr",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			repoName, err := gitutils.GetMetaRepoName(cwd)
			if err != nil {
				if jsonOutput {
					printJSON(false, fmt.Sprintf("Failed to resolve repository name: %v", err), nil)
				}
				return err
			}

			// 1. Register repository on remote backend server
			if !jsonOutput {
				fmt.Printf("1. Registering repository '%s' with MetaStackr server at %s...\n", repoName, serverURL)
			}

			trackPayload := map[string]interface{}{
				"full_name":       repoName,
				"allow_code_pull": allowCodePull,
			}
			trackBytes, err := json.Marshal(trackPayload)
			if err != nil {
				return err
			}

			trackURL := fmt.Sprintf("%s/api/v1/repos/track", serverURL)
			req, err := http.NewRequest(http.MethodPost, trackURL, bytes.NewReader(trackBytes))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			serverRegistered := true
			var regErrMsg string
			var serverRepoID string

			if err != nil {
				serverRegistered = false
				regErrMsg = err.Error()
			} else {
				defer resp.Body.Close()
				if resp.StatusCode >= 400 {
					serverRegistered = false
					regErrMsg = fmt.Sprintf("server returned status %d", resp.StatusCode)
				} else {
					var trackResp map[string]interface{}
					if err := json.NewDecoder(resp.Body).Decode(&trackResp); err == nil {
						if id, ok := trackResp["repo_id"].(string); ok {
							serverRepoID = id
						}
					}
				}
			}

			if !serverRegistered {
				errStr := fmt.Sprintf("Failed to register on server: %s", regErrMsg)
				if jsonOutput {
					printJSON(false, errStr, nil)
				}
				return fmt.Errorf("%s", errStr)
			}

			if !jsonOutput {
				fmt.Printf("  ✅ Repository registered successfully on server (Repo ID: %s).\n", serverRepoID)
			}

			// Determine webhook secret (prioritizing serverRepoID from DB, then secret flag, then env, then generate)
			if secret == "" {
				if serverRepoID != "" {
					secret = serverRepoID
				} else {
					secret = os.Getenv("WEBHOOK_SECRET")
					if secret == "" {
						secret = "ms-secret-" + uuid.New().String()[:12]
					}
				}
			}

			// 2. Install local Git hooks
			if !jsonOutput {
				fmt.Println("\n2. Installing local Git hooks...")
			}
			err = InstallHooks(cwd)
			if err != nil {
				if jsonOutput {
					printJSON(false, fmt.Sprintf("Failed to install hooks: %v", err), nil)
				}
				return err
			}
			if !jsonOutput {
				fmt.Println("  ✅ Git hooks installed successfully.")
			}

			// 3. Register GitHub Webhooks
			if !jsonOutput {
				fmt.Println("\n3. Registering GitHub Webhooks (Parent + Submodules)...")
			}
			err = gitutils.RegisterGitHubWebhook(cwd, webhookURL, secret, "")
			if err != nil {
				if jsonOutput {
					printJSON(false, fmt.Sprintf("Failed to register webhooks: %v", err), nil)
				}
				return err
			}

			if !jsonOutput {
				fmt.Printf("\n🎉 Onboarding Complete! MetaStackr is fully set up for this repository.\n")
				fmt.Printf("Webhook Secret used: %s\n", secret)
			} else {
				printJSON(true, "Repository onboarding completed successfully", map[string]interface{}{
					"repo_name":             repoName,
					"hooks_installed":       true,
					"webhooks_registered":   true,
					"server_registered":     serverRegistered,
					"server_register_error": regErrMsg,
					"webhook_url":           webhookURL,
					"webhook_secret":        secret,
				})
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "https://api.metastac.kr", "MetaStackr backend server URL")
	cmd.Flags().StringVar(&webhookURL, "url", "https://api.metastac.kr/webhooks/github", "The webhook target URL")
	cmd.Flags().StringVar(&secret, "secret", "", "Optional webhook signature verification secret key (defaults to server repo UUID)")
	cmd.Flags().BoolVar(&allowCodePull, "allow-code-pull", false, "Opt-in to allow backend server to pull/clone repo code")

	return cmd
}
