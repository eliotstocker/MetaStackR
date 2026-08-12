package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
		Use:     "git-meta",
		Short:   "MetaStackR CLI for Git submodules and PR synchronization",
		Version: Version,
	}

	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output machine-readable JSON")

	useCmd := "git meta"
	if len(os.Args) > 0 && (os.Args[0] == "./git-meta" || os.Args[0] == "git-meta") && os.Getenv("GIT_EXEC_PATH") == "" {
		useCmd = "git-meta"
	}

	rootCmd.SetVersionTemplate(GetBanner() + useCmd + " version {{.Version}}\n")

	origHelpFunc := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		PrintBanner()
		if useCmd == "git meta" {
			b := &bytes.Buffer{}
			cmd.SetOut(b)
			origHelpFunc(cmd, args)
			cmd.SetOut(os.Stdout)
			outStr := strings.ReplaceAll(b.String(), "git-meta", "git meta")
			fmt.Print(outStr)
		} else {
			origHelpFunc(cmd, args)
		}
	})

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		cmdName := cmd.Name()
		if cmdName == "init" || cmdName == "version" || cmdName == "help" || cmdName == "agents" || cmdName == "completion" || cmdName == "git-meta" {
			return
		}

		cwd, err := os.Getwd()
		if err != nil {
			return
		}

		if !IsInitialized(cwd) {
			warnMsg := fmt.Sprintf("⚠️ WARNING: Repository has not been initialized with '%s init'. Run '%s init' to register webhooks and enable backend PR tracking.", useCmd, useCmd)
			if jsonOutput {
				fmt.Fprintln(os.Stderr, warnMsg)
			} else {
				fmt.Println(warningStyle.Render(warnMsg))
				fmt.Println()
			}
		}
	}

	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newCheckoutCmd())
	rootCmd.AddCommand(newCommitCmd())
	rootCmd.AddCommand(newPushCmd())
	rootCmd.AddCommand(newCreatePRCmd())
	rootCmd.AddCommand(newSyncCmd())
	rootCmd.AddCommand(newRebaseCmd())
	rootCmd.AddCommand(newRetryMergeCmd())
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newSettingsCmd())
	rootCmd.AddCommand(newInstallHooksCmd())
	rootCmd.AddCommand(newAgentsCmd())
	rootCmd.AddCommand(newSetupWebhookCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newCompletionCmd(rootCmd))

	return rootCmd
}

func newCompletionCmd(rootCmd *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate the autocompletion script for git-meta and git meta",
		Long: `To load completions:

  # Zsh:
  # Add to your ~/.zshrc:
  source <(git meta completion zsh)

  # Bash:
  # Add to your ~/.bashrc:
  source <(git meta completion bash)

  # Fish:
  # Add to your ~/.config/fish/config.fish:
  git meta completion fish | source
`,
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				var buf bytes.Buffer
				if err := rootCmd.GenBashCompletionV2(&buf, true); err != nil {
					return err
				}
				buf.WriteString("\n# Git subcommand completion for 'git meta'\n_git_meta() {\n    _git-meta \"$@\"\n}\ncomplete -F _git_meta git-meta\n")
				fmt.Print(buf.String())
			case "zsh":
				var buf bytes.Buffer
				if err := rootCmd.GenZshCompletion(&buf); err != nil {
					return err
				}
				buf.WriteString("\n# Git subcommand completion for 'git meta'\n_git_meta() {\n    _git-meta \"$@\"\n}\ncompdef _git_meta git-meta\ncompdef _git_meta 'git meta'\n")
				fmt.Print(buf.String())
			case "fish":
				return rootCmd.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
			}
			return nil
		},
	}
	return cmd
}

func GetRepoRoot(dir string) string {
	root, err := gitutils.ExecGit(dir, "rev-parse", "--show-toplevel")
	if err == nil && strings.TrimSpace(root) != "" {
		return strings.TrimSpace(root)
	}
	return dir
}

func IsInitialized(rootDir string) bool {
	root := GetRepoRoot(rootDir)

	val, err := gitutils.ExecGit(root, "config", "--get", "metastackr.initialized")
	if err == nil && strings.TrimSpace(val) == "true" {
		return true
	}

	postCheckoutPath := filepath.Join(root, ".git", "hooks", "post-checkout")
	if data, err := os.ReadFile(postCheckoutPath); err == nil && strings.Contains(string(data), "git-meta") {
		return true
	}

	agentsPath := filepath.Join(root, "AGENTS.md")
	if data, err := os.ReadFile(agentsPath); err == nil && (strings.Contains(string(data), "MetaStackR") || strings.Contains(string(data), "git-meta")) {
		return true
	}

	return false
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

			remoteMetaPR := fetchRemotePRStatus(serverURL, metaRepoName, localStatus.MetaBranch)

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

	return cmd
}

type RemotePRInfo struct {
	MetaPR    *db.MetaPR
	ServerURL string
	Message   string
	Reachable bool
}

func fetchRemotePRStatus(serverURL, repo, branch string) *RemotePRInfo {
	urls := []string{}
	if serverURL != "" {
		urls = append(urls, serverURL)
	}
	if env := os.Getenv("METASTACKR_SERVER_URL"); env != "" && env != serverURL {
		urls = append(urls, env)
	}
	urls = append(urls, "https://api.metastac.kr", "http://localhost:8080")

	client := &http.Client{Timeout: 2 * time.Second}
	for _, target := range urls {
		target = strings.TrimSuffix(target, "/")
		url := fmt.Sprintf("%s/api/v1/prs/status?repo=%s&branch=%s", target, repo, branch)
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			var res struct {
				MetaPR  *db.MetaPR `json:"meta_pr"`
				Message string     `json:"message"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
				resp.Body.Close()
				return &RemotePRInfo{
					MetaPR:    res.MetaPR,
					ServerURL: target,
					Message:   res.Message,
					Reachable: true,
				}
			}
			resp.Body.Close()
		}
	}
	return &RemotePRInfo{Reachable: false}
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func renderMergedTable(local *gitutils.MetaLocalStatus, remote *RemotePRInfo) {
	col1Width := 24
	col2Width := 18
	col3Width := 18
	col4Width := 12
	col5Width := 14

	for _, sub := range local.Submodules {
		if w := lipgloss.Width(sub.Path) + 4; w > col1Width {
			col1Width = w
		}
	}

	header := fmt.Sprintf("%s | %s | %s | %s | %s",
		padRight(headerStyle.Render("Submodule Path"), col1Width),
		padRight(headerStyle.Render("Local Branch"), col2Width),
		padRight(headerStyle.Render("Local Drift"), col3Width),
		padRight(headerStyle.Render("Child PR"), col4Width),
		padRight(headerStyle.Render("PR Status"), col5Width),
	)
	fmt.Println(header)
	fmt.Println(strings.Repeat("-", lipgloss.Width(header)))

	if len(local.Submodules) == 0 {
		fmt.Println(cellStyle.Render("No submodules found in current workspace."))
		return
	}

	childPRMap := make(map[string]db.ChildPR)
	if remote != nil && remote.MetaPR != nil {
		for _, c := range remote.MetaPR.ChildPRs {
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
		} else {
			subDir := local.MetaRepoPath + "/" + sub.Path
			if direct := gitutils.GetDirectPRStatus(subDir, sub.Branch); direct != nil {
				prText = fmt.Sprintf("#%d", direct.PRNumber)
				statusText = direct.Status
			}
		}

		fmt.Printf("%s | %s | %s | %s | %s\n",
			padRight(cellStyle.Render(sub.Path), col1Width),
			padRight(cellStyle.Render(sub.Branch), col2Width),
			padRight(cellStyle.Render(drift), col3Width),
			padRight(cellStyle.Render(prText), col4Width),
			padRight(cellStyle.Render(statusText), col5Width),
		)
	}

	fmt.Println()
	if remote != nil && remote.MetaPR != nil {
		fmt.Printf("Backend Meta PR Status: %s (Lock Version: %d) via %s\n",
			lipgloss.NewStyle().Bold(true).Render(remote.MetaPR.Status),
			remote.MetaPR.LockVersion,
			remote.ServerURL,
		)
	} else if remote != nil && remote.Reachable {
		fmt.Printf("Backend PR Status: Connected to %s (No active Meta PR tracked for branch '%s')\n", remote.ServerURL, local.MetaBranch)
		if remote.Message == "Repo not tracked" {
			fmt.Println(cellStyle.Render("   💡 Tip: Run 'git meta init' to register repository & GitHub webhooks with MetaStackr."))
		}
	} else {
		fmt.Println("Backend PR Status: Server unreachable (Local mode). Start 'metastackrd' or check network connection.")
	}

	var unaligned []string
	for _, sub := range local.Submodules {
		if sub.Branch != local.MetaBranch && sub.Branch != "" && sub.Branch != "HEAD" {
			unaligned = append(unaligned, fmt.Sprintf("%s (%s)", sub.Path, sub.Branch))
		}
	}
	if len(unaligned) > 0 {
		fmt.Println()
		fmt.Println(warningStyle.Render(fmt.Sprintf("⚠️ Branch Mismatch Warning: %d submodule(s) are on a different branch than meta-repo '%s': %s", len(unaligned), local.MetaBranch, strings.Join(unaligned, ", "))))
		fmt.Println(cellStyle.Render("   Run 'git meta checkout " + local.MetaBranch + "' to align all submodules to the meta-repo branch."))
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

func newCreatePRCmd() *cobra.Command {
	var opts gitutils.CreatePROptions

	cmd := &cobra.Command{
		Use:     "create-pr",
		Aliases: []string{"pr", "open-pr"},
		Short:   "Open or create GitHub Pull Requests across all modified submodules and meta-repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			opts.MergeMethod = strings.ToLower(strings.TrimSpace(opts.MergeMethod))
			if opts.MergeMethod != "" && opts.MergeMethod != "merge" && opts.MergeMethod != "squash" && opts.MergeMethod != "rebase" {
				return fmt.Errorf("invalid merge method '%s'. Allowed values: merge, squash, rebase", opts.MergeMethod)
			}

			opts.Interactive = !jsonOutput
			results, err := gitutils.CreatePRs(cwd, opts)
			if err != nil {
				if jsonOutput {
					printJSON(false, err.Error(), nil)
					return nil
				}
				return err
			}

			if jsonOutput {
				printJSON(true, fmt.Sprintf("Processed %d pull request target(s)", len(results)), map[string]interface{}{
					"prs": results,
				})
				return nil
			}

			fmt.Println(titleStyle.Render("⚡ MetaStackR Pull Request Creator"))
			for _, pr := range results {
				if pr.Created {
					fmt.Printf("  ✅ %s (%s): %s\n", pr.RepoName, pr.HeadBranch, successStyle.Render(pr.URL))
				} else if pr.OpenedWeb {
					fmt.Printf("  🌐 %s (%s): Opened in browser -> %s\n", pr.RepoName, pr.HeadBranch, cellStyle.Render(pr.URL))
				} else if pr.Error != "" {
					fmt.Printf("  ℹ️ %s (%s): %s (%s)\n", pr.RepoName, pr.HeadBranch, warningStyle.Render(pr.Error), pr.URL)
				} else {
					fmt.Printf("  🔗 %s (%s): %s\n", pr.RepoName, pr.HeadBranch, pr.URL)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Pull request title (defaults to latest commit message)")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Pull request description body")
	cmd.Flags().StringVar(&opts.BaseBranch, "base", "main", "Target base branch for pull requests")
	cmd.Flags().StringVar(&opts.MergeMethod, "merge-method", "merge", "Merge method to use when auto-merging on GitHub (merge, squash, rebase)")
	cmd.Flags().BoolVarP(&opts.Draft, "draft", "d", false, "Create draft pull requests")
	cmd.Flags().BoolVarP(&opts.ForceWeb, "web", "w", false, "Open PR compare pages in default web browser")

	return cmd
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

func newSettingsCmd() *cobra.Command {
	return newConfigCmd()
}

func newConfigCmd() *cobra.Command {
	var serverURL string
	var repoOverride string
	var listFlag bool
	var unsetFlag bool

	var mergeMethodFlag string
	var requiredChecksFlag string
	var requireRootApprovalFlag string
	var autoMergeFlag string
	var submoduleChangesOnlyFlag string

	cmd := &cobra.Command{
		Use:     "config [key] [value]",
		Aliases: []string{"settings", "policy"},
		Short:   "Get, set, or list repository policy settings (git config style)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			repoName := repoOverride
			if repoName == "" {
				repoName, err = gitutils.GetMetaRepoName(cwd)
				if err != nil || repoName == "" {
					return fmt.Errorf("could not determine meta-repo origin: %w", err)
				}
			}

			// 1. Fetch current settings first
			getURL := fmt.Sprintf("%s/api/v1/repos/settings?repo=%s", serverURL, url.QueryEscape(repoName))
			resp, err := http.Get(getURL)
			if err != nil {
				if jsonOutput {
					printJSON(false, fmt.Sprintf("failed to fetch repo settings: %v", err), nil)
					return nil
				}
				return fmt.Errorf("failed to contact backend server: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				errStr := fmt.Sprintf("failed to fetch settings (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
				if jsonOutput {
					printJSON(false, errStr, nil)
					return nil
				}
				return fmt.Errorf("%s", errStr)
			}

			var currentSettings struct {
				ID                   string   `json:"id"`
				RepoOwner            string   `json:"repo_owner"`
				RepoName             string   `json:"repo_name"`
				RepoFullName         string   `json:"repo_full_name"`
				InstallationID       string   `json:"installation_id"`
				IsEnabled            bool     `json:"is_enabled"`
				AllowCodePull        bool     `json:"allow_code_pull"`
				RequireRootApproval  bool     `json:"require_root_approval"`
				AutoMergeEnabled     bool     `json:"auto_merge_enabled"`
				SubmoduleChangesOnly bool     `json:"submodule_changes_only"`
				VCSToken             string   `json:"vcs_token"`
				VCSProvider          string   `json:"vcs_provider"`
				RequiredChecks       []string `json:"required_checks"`
				DefaultMergeMethod   string   `json:"default_merge_method"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&currentSettings); err != nil {
				if jsonOutput {
					printJSON(false, fmt.Sprintf("failed to parse settings response: %v", err), nil)
					return nil
				}
				return err
			}

			normalizeKey := func(k string) string {
				k = strings.ToLower(strings.TrimSpace(k))
				switch k {
				case "auto-merge", "automerge", "auto_merge", "auto_merge_enabled":
					return "auto-merge"
				case "require-root-approval", "require-approval", "root-approval", "require_root_approval":
					return "require-root-approval"
				case "submodule-changes-only", "submodule-only", "require-submodule-only", "submodule_changes_only":
					return "submodule-changes-only"
				case "merge-method", "method", "default-merge-method", "default_merge_method":
					return "merge-method"
				case "required-checks", "checks", "required_checks":
					return "required-checks"
				case "vcs-provider", "vcs_provider", "vcs", "provider":
					return "vcs-provider"
				default:
					return k
				}
			}

			// Mode 1: Unset key (git config --unset key)
			if unsetFlag {
				if len(args) == 0 {
					return fmt.Errorf("you must specify a key to unset (e.g. 'git meta config --unset required-checks')")
				}
				key := normalizeKey(args[0])
				reqApproval := currentSettings.RequireRootApproval
				autoMerge := currentSettings.AutoMergeEnabled
				subOnly := currentSettings.SubmoduleChangesOnly
				method := currentSettings.DefaultMergeMethod
				reqChecks := currentSettings.RequiredChecks

				switch key {
				case "require-root-approval":
					reqApproval = false
				case "auto-merge":
					autoMerge = true
				case "submodule-changes-only":
					subOnly = true
				case "merge-method":
					method = "merge"
				case "required-checks":
					reqChecks = []string{}
				case "vcs-token":
					// unset token
				case "vcs-provider":
					_, _ = gitutils.ExecGit(cwd, "config", "--unset", "metastackr.vcs-provider")
					_, _ = gitutils.ExecGit(cwd, "config", "--unset", "metastackr.vcs")
					return updateRepoSettings(serverURL, repoName, reqApproval, autoMerge, method, reqChecks, subOnly, "", "github")
				default:
					return fmt.Errorf("unknown config key '%s'. Supported keys: auto-merge, require-root-approval, submodule-changes-only, merge-method, required-checks, vcs-token, vcs-provider", key)
				}

				return updateRepoSettings(serverURL, repoName, reqApproval, autoMerge, method, reqChecks, subOnly, "", currentSettings.VCSProvider)
			}

			// Mode 2: No args or --list -> List all config key-values (git config --list)
			if (len(args) == 0 && !cmd.Flags().Changed("require-root-approval") && !cmd.Flags().Changed("auto-merge") && !cmd.Flags().Changed("merge-method") && !cmd.Flags().Changed("required-checks") && !cmd.Flags().Changed("submodule-changes-only")) || listFlag {
				if jsonOutput {
					printJSON(true, "Current repository policy settings", currentSettings)
					return nil
				}

				checksVal := strings.Join(currentSettings.RequiredChecks, ",")
				fmt.Printf("auto-merge=%t\n", currentSettings.AutoMergeEnabled)
				fmt.Printf("require-root-approval=%t\n", currentSettings.RequireRootApproval)
				fmt.Printf("submodule-changes-only=%t\n", currentSettings.SubmoduleChangesOnly)
				fmt.Printf("merge-method=%s\n", currentSettings.DefaultMergeMethod)
				fmt.Printf("required-checks=%s\n", checksVal)
				if currentSettings.VCSToken != "" {
					fmt.Println("vcs-token=********")
				}
				vcsVal := currentSettings.VCSProvider
				if vcsVal == "" {
					if localVal, err := gitutils.ExecGit(cwd, "config", "--get", "metastackr.vcs-provider"); err == nil && strings.TrimSpace(localVal) != "" {
						vcsVal = strings.TrimSpace(localVal)
					} else {
						vcsVal = "github"
					}
				}
				fmt.Printf("vcs-provider=%s\n", vcsVal)
				return nil
			}

			// Mode 3: Single key arg -> GET specific key value (git config key)
			if len(args) == 1 && !cmd.Flags().Changed("require-root-approval") && !cmd.Flags().Changed("auto-merge") && !cmd.Flags().Changed("merge-method") && !cmd.Flags().Changed("required-checks") && !cmd.Flags().Changed("submodule-changes-only") {
				key := normalizeKey(args[0])
				var val string
				switch key {
				case "auto-merge":
					val = fmt.Sprintf("%t", currentSettings.AutoMergeEnabled)
				case "require-root-approval":
					val = fmt.Sprintf("%t", currentSettings.RequireRootApproval)
				case "submodule-changes-only":
					val = fmt.Sprintf("%t", currentSettings.SubmoduleChangesOnly)
				case "merge-method":
					val = currentSettings.DefaultMergeMethod
				case "required-checks":
					val = strings.Join(currentSettings.RequiredChecks, ",")
				case "vcs-token":
					if currentSettings.VCSToken != "" {
						val = "********"
					} else {
						val = ""
					}
				case "vcs-provider":
					val = currentSettings.VCSProvider
					if val == "" {
						if localVal, err := gitutils.ExecGit(cwd, "config", "--get", "metastackr.vcs-provider"); err == nil && strings.TrimSpace(localVal) != "" {
							val = strings.TrimSpace(localVal)
						} else {
							val = "github"
						}
					}
				default:
					return fmt.Errorf("unknown config key '%s'. Supported keys: auto-merge, require-root-approval, submodule-changes-only, merge-method, required-checks, vcs-token, vcs-provider", key)
				}

				if jsonOutput {
					printJSON(true, "", map[string]string{"key": key, "value": val})
				} else {
					fmt.Println(val)
				}
				return nil
			}

			// Mode 4: Key + Value args or Flags -> SET config key value (git config key value)
			reqApproval := currentSettings.RequireRootApproval
			autoMerge := currentSettings.AutoMergeEnabled
			subOnly := currentSettings.SubmoduleChangesOnly
			method := currentSettings.DefaultMergeMethod
			reqChecks := currentSettings.RequiredChecks
			vcsProvider := currentSettings.VCSProvider
			vcsToken := ""

			if len(args) >= 2 {
				key := normalizeKey(args[0])
				val := args[1]

				switch key {
				case "auto-merge":
					autoMerge = strings.ToLower(val) == "true" || val == "1"
				case "require-root-approval":
					reqApproval = strings.ToLower(val) == "true" || val == "1"
				case "submodule-changes-only":
					subOnly = strings.ToLower(val) == "true" || val == "1"
				case "merge-method":
					valLower := strings.ToLower(strings.TrimSpace(val))
					if valLower != "merge" && valLower != "squash" && valLower != "rebase" {
						return fmt.Errorf("invalid merge method '%s'. Allowed values: merge, squash, rebase", val)
					}
					method = valLower
				case "required-checks":
					reqChecks = nil
					if strings.TrimSpace(val) != "" {
						for _, c := range strings.Split(val, ",") {
							if trimmed := strings.TrimSpace(c); trimmed != "" {
								reqChecks = append(reqChecks, trimmed)
							}
						}
					}
				case "vcs-token":
					vcsToken = strings.TrimSpace(val)
				case "vcs-provider":
					valLower := strings.ToLower(strings.TrimSpace(val))
					if valLower != "github" && valLower != "gitlab" {
						return fmt.Errorf("invalid vcs-provider '%s'. Allowed values: github, gitlab", val)
					}
					vcsProvider = valLower
					_, _ = gitutils.ExecGit(cwd, "config", "metastackr.vcs-provider", valLower)
				default:
					return fmt.Errorf("unknown config key '%s'. Supported keys: auto-merge, require-root-approval, submodule-changes-only, merge-method, required-checks, vcs-token, vcs-provider", key)
				}
			}

			// Handle optional flag overrides
			if cmd.Flags().Changed("require-root-approval") {
				reqApproval = strings.ToLower(requireRootApprovalFlag) == "true" || requireRootApprovalFlag == "1"
			}
			if cmd.Flags().Changed("auto-merge") {
				autoMerge = strings.ToLower(autoMergeFlag) == "true" || autoMergeFlag == "1"
			}
			if cmd.Flags().Changed("submodule-changes-only") {
				subOnly = strings.ToLower(submoduleChangesOnlyFlag) == "true" || submoduleChangesOnlyFlag == "1"
			}
			if cmd.Flags().Changed("merge-method") {
				method = strings.ToLower(strings.TrimSpace(mergeMethodFlag))
			}
			if cmd.Flags().Changed("required-checks") {
				reqChecks = nil
				if strings.TrimSpace(requiredChecksFlag) != "" {
					for _, c := range strings.Split(requiredChecksFlag, ",") {
						if trimmed := strings.TrimSpace(c); trimmed != "" {
							reqChecks = append(reqChecks, trimmed)
						}
					}
				}
			}

			return updateRepoSettings(serverURL, repoName, reqApproval, autoMerge, method, reqChecks, subOnly, vcsToken, vcsProvider)
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "https://api.metastac.kr", "MetaStackr backend server URL")
	cmd.Flags().StringVar(&repoOverride, "repo", "", "Meta-repository full name (e.g. owner/repo)")
	cmd.Flags().BoolVarP(&listFlag, "list", "l", false, "List all config key-values")
	cmd.Flags().BoolVar(&unsetFlag, "unset", false, "Unset a config key")
	cmd.Flags().StringVar(&requireRootApprovalFlag, "require-root-approval", "", "Require root PR approval before auto-merging (true|false)")
	cmd.Flags().StringVar(&autoMergeFlag, "auto-merge", "", "Enable auto cascade merge (true|false)")
	cmd.Flags().StringVar(&submoduleChangesOnlyFlag, "submodule-changes-only", "", "Auto-merge only when changes are restricted to submodules (true|false)")
	cmd.Flags().StringVar(&mergeMethodFlag, "merge-method", "", "Default merge method (merge|squash|rebase)")
	cmd.Flags().StringVar(&requiredChecksFlag, "required-checks", "", "Comma-separated list of required status check names")

	return cmd
}

func updateRepoSettings(serverURL, repoName string, reqApproval, autoMerge bool, method string, reqChecks []string, subOnly bool, vcsToken string, vcsProvider string) error {
	updatePayload := map[string]interface{}{
		"repo":                   repoName,
		"require_root_approval":  reqApproval,
		"auto_merge_enabled":     autoMerge,
		"submodule_changes_only": subOnly,
		"required_checks":        reqChecks,
		"default_merge_method":   method,
		"vcs_token":              vcsToken,
		"vcs_provider":           vcsProvider,
	}

	jsonBytes, _ := json.Marshal(updatePayload)
	postURL := serverURL + "/api/v1/repos/settings"
	postResp, err := http.Post(postURL, "application/json", bytes.NewReader(jsonBytes))
	if err != nil {
		if jsonOutput {
			printJSON(false, err.Error(), nil)
			return nil
		}
		return fmt.Errorf("failed to contact backend server: %w", err)
	}
	defer postResp.Body.Close()

	if postResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(postResp.Body)
		errStr := fmt.Sprintf("failed to update settings (HTTP %d): %s", postResp.StatusCode, string(bodyBytes))
		if jsonOutput {
			printJSON(false, errStr, nil)
			return nil
		}
		return fmt.Errorf("%s", errStr)
	}

	var updateResult map[string]interface{}
	_ = json.NewDecoder(postResp.Body).Decode(&updateResult)

	if jsonOutput {
		printJSON(true, "Repository policy settings updated successfully", updateResult)
	}
	return nil
}

func GetAgentsMDContent() string {
	return `# Repository Agent Guidelines

This repository is a **Meta-Repo** managed by **MetaStackr** (` + "`" + `git-meta` + "`" + `).

## About MetaStackr
MetaStackr orchestrates development across multi-repository meta-repos with Git submodules. It coordinates local workflows, tracks submodule drift, synchronizes PRs, and handles atomic cascade merges on GitHub.

## Rules for AI Agents
- **Do NOT run raw ` + "`" + `git checkout` + "`" + ` or ` + "`" + `git commit` + "`" + ` commands directly inside nested submodule directories.**
  - Running raw git commands directly inside submodules breaks pointer alignment and creates state drift.
- **Use ` + "`" + `git-meta` + "`" + ` for all multi-repo operations.**
  - Use ` + "`" + `git-meta` + "`" + ` commands to manage branches, commits, pushes, and synchronization across the meta-repo and submodules.
- **Always supply ` + "`" + `--json` + "`" + ` to ` + "`" + `git-meta` + "`" + ` CLI commands for deterministic state parsing.**
  - All ` + "`" + `git-meta` + "`" + ` subcommands accept ` + "`" + `--json` + "`" + ` to return structured JSON payloads.

## Key Operations

- **Inspect State & Submodule Drift**:
  ` + "`" + `git meta status --json` + "`" + `
  Returns local submodule drift (uncommitted/unpushed changes) merged with remote Meta PR status.

- **Switch/Create Branches System-Wide**:
  ` + "`" + `git meta checkout -b <branch-name> --json` + "`" + `
  Safely creates or switches branches across the parent meta-repo and all submodules.

- **Atomic Commits Across Submodules**:
  ` + "`" + `git meta commit -m "<message>" --json` + "`" + `
  Creates coordinated commits in all modified submodules and updates parent commit pointers.

- **Push Changes (Bottom-Up Enforcement)**:
  ` + "`" + `git meta push --json` + "`" + `
  Pushes submodule commits to remote origin before pushing parent commit pointer updates.

- **Create/Open PRs System-Wide**:
  ` + "`" + `git meta create-pr --json` + "`" + ` (or ` + "`" + `git meta pr` + "`" + `)
  Creates or opens GitHub Pull Requests across modified submodules and parent meta-repo.

- **Sync Upstream Changes**:
  ` + "`" + `git meta sync --json` + "`" + `
  Fetches upstream, fast-forwards/rebases local submodules, and aligns root pointers.

- **Two-Phase Rebase**:
  ` + "`" + `git meta rebase <upstream-branch> --json` + "`" + `
  Rebases child submodules first, then parent meta-repo references.

- **Retry Cascade Merges**:
  ` + "`" + `git meta retry-merge --pr <pr-number> --json` + "`" + `
  Re-triggers cascade merges on partially failed PRs.

- **Configure Auto-Merge Policy Rules**:
  ` + "`" + `git meta settings [--require-root-approval=true|false] [--auto-merge=true|false] [--merge-method=merge|squash|rebase] [--required-checks="ci/build,lint"] --json` + "`" + `
  Inspects or updates repository policy rules and auto-merge settings.
`
}

func WriteAgentsMD(repoDir string) error {
	content := GetAgentsMDContent()

	rootPath := filepath.Join(repoDir, "AGENTS.md")
	if err := os.WriteFile(rootPath, []byte(content), 0644); err != nil {
		return err
	}

	agentsDir := filepath.Join(repoDir, ".agents")
	if err := os.MkdirAll(agentsDir, 0755); err == nil {
		dotAgentsPath := filepath.Join(agentsDir, "AGENTS.md")
		_ = os.WriteFile(dotAgentsPath, []byte(content), 0644)
	}

	return nil
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
						"status":      "git meta status --json",
						"checkout":    "git meta checkout -b <branch-name> --json",
						"commit":      "git meta commit -m \"<msg>\" --json",
						"push":        "git meta push --json",
						"create-pr":   "git meta create-pr --json",
						"sync":        "git meta sync --json",
						"rebase":      "git meta rebase <upstream-branch> --json",
						"retry-merge": "git meta retry-merge --pr <pr-number> --json",
					},
				})
				return nil
			}

			fmt.Println(GetAgentsMDContent())
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

			remoteURL, _ := gitutils.ExecGit(cwd, "config", "--get", "remote.origin.url")
			vcsProvider := gitutils.DetectVCSProvider(cwd, remoteURL)
			if vcsProvider == "gitlab" && targetURL == "https://api.metastac.kr/webhooks/github" {
				targetURL = "https://api.metastac.kr/webhooks/gitlab"
			}

			if secret == "" {
				secret = os.Getenv("WEBHOOK_SECRET")
				if secret == "" {
					secret = "ms-secret-" + uuid.New().String()[:12]
				}
			}

			err = gitutils.RegisterVCSWebhook(cwd, targetURL, secret, "")
			if err != nil {
				if jsonOutput {
					printJSON(false, err.Error(), nil)
				}
				return err
			}

			_, _ = gitutils.ExecGit(cwd, "config", "metastackr.initialized", "true")

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
	var skipWebhooks bool

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

			remoteURL, _ := gitutils.ExecGit(cwd, "config", "--get", "remote.origin.url")
			vcsProvider := gitutils.DetectVCSProvider(cwd, remoteURL)
			if vcsProvider == "gitlab" && webhookURL == "https://api.metastac.kr/webhooks/github" {
				webhookURL = fmt.Sprintf("%s/webhooks/gitlab", strings.TrimSuffix(serverURL, "/"))
			}

			// 1. Register repository on remote backend server
			if !jsonOutput {
				fmt.Printf("1. Registering repository '%s' (%s) with MetaStackr server at %s...\n", repoName, vcsProvider, serverURL)
			}

			gitlabToken := os.Getenv("GITLAB_TOKEN")
			if gitlabToken == "" {
				if tok, err := gitutils.ExecGit(cwd, "config", "--get", "metastackr.gitlab-token"); err == nil {
					gitlabToken = strings.TrimSpace(tok)
				}
			}

			trackPayload := map[string]interface{}{
				"full_name":       repoName,
				"allow_code_pull": allowCodePull,
				"vcs_provider":    vcsProvider,
				"vcs_token":       gitlabToken,
			}
			trackBytes, err := json.Marshal(trackPayload)
			if err != nil {
				return err
			}

			var appInstalledOnRepo bool
			var appInstalledOnAccount bool

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
						if appInstalled, ok := trackResp["github_app_installed"].(bool); ok && appInstalled {
							appInstalledOnRepo = true
						}
						if accountInstalled, ok := trackResp["app_installed_on_account"].(bool); ok && accountInstalled {
							appInstalledOnAccount = true
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

			// Register submodules on remote backend server
			localStatus, _ := gitutils.GetLocalStatus(cwd)
			if localStatus != nil {
				for _, sub := range localStatus.Submodules {
					subRepoName, err := gitutils.GetMetaRepoName(filepath.Join(cwd, sub.Path))
					if err == nil && subRepoName != "" && subRepoName != repoName {
						subRemoteURL, _ := gitutils.ExecGit(filepath.Join(cwd, sub.Path), "config", "--get", "remote.origin.url")
						subVCS := gitutils.DetectVCSProvider(filepath.Join(cwd, sub.Path), subRemoteURL)
						if subVCS == "unknown" || subVCS == "" {
							subVCS = vcsProvider
						}
						subTrackPayload := map[string]interface{}{
							"full_name":       subRepoName,
							"allow_code_pull": allowCodePull,
							"vcs_provider":    subVCS,
						}
						if subTrackBytes, err := json.Marshal(subTrackPayload); err == nil {
							if req, err := http.NewRequest(http.MethodPost, trackURL, bytes.NewReader(subTrackBytes)); err == nil {
								req.Header.Set("Content-Type", "application/json")
								if subResp, err := client.Do(req); err == nil {
									subResp.Body.Close()
								}
							}
						}
					}
				}
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

			// 3. Register Webhooks (GitHub vs GitLab)
			if vcsProvider == "gitlab" {
				if !jsonOutput {
					fmt.Println("\n3. GitLab Webhook Setup:")
				}
				err = gitutils.RegisterGitLabWebhook(cwd, webhookURL, secret, "")
				if err != nil && !jsonOutput {
					fmt.Printf("\n   👉 Manual Webhook Setup Instructions for GitLab (if automated setup fails):\n")
					fmt.Printf("      1. Open your project on GitLab -> Settings -> Webhooks\n")
					fmt.Printf("      2. Add Webhook URL: %s\n", webhookURL)
					fmt.Printf("      3. Secret Token:    %s\n", secret)
					fmt.Println("      4. Select Trigger Events: 'Merge request events'")
					fmt.Println("      5. Click 'Add webhook' to complete onboarding!")
				}
			} else if appInstalledOnRepo {
				if !jsonOutput {
					fmt.Println("\n3. ✅ MetaStackr GitHub App is installed and has permission for this repository. Skipping manual webhook setup!")
				}
			} else if skipWebhooks {
				if !jsonOutput {
					fmt.Println("\n3. Skipping per-repository GitHub Webhooks setup (--skip-webhooks flag set).")
				}
			} else {
				if !jsonOutput {
					fmt.Println("\n3. GitHub Webhook Setup:")
					if appInstalledOnAccount {
						fmt.Printf("   ⚠️ MetaStackr GitHub App is installed on your account, but does NOT have permission for repository '%s'.\n", repoName)
						fmt.Printf("   👉 Grant permission to this repository: https://github.com/apps/metastackr\n\n")
					} else {
						fmt.Println("   ℹ️ MetaStackr GitHub App is not active for this repository.")
						fmt.Printf("   👉 Install GitHub App (Recommended): https://github.com/apps/metastackr\n\n")
					}

					setupManual := false
					if !jsonOutput {
						fmt.Println("   Would you like to:")
						fmt.Println("     [1] Install / grant repository access to the MetaStackr GitHub App (Recommended)")
						fmt.Println("     [2] Set up repository webhooks manually now")
						fmt.Print("   Enter choice [1/2] (default 1): ")

						var input string
						_, _ = fmt.Scanln(&input)
						input = strings.TrimSpace(input)
						if input == "2" {
							setupManual = true
						}
					}

					if setupManual {
						fmt.Println("\n   Registering repository webhooks manually...")
						err = gitutils.RegisterGitHubWebhook(cwd, webhookURL, secret, "")
						if err != nil {
							fmt.Printf("   ℹ️ Webhook registration note: %v\n", err)
						} else {
							fmt.Println("   ✅ GitHub webhooks registered successfully.")
						}
					} else {
						fmt.Println("\n   ⏩ Skipped manual webhook setup. Once the GitHub App has access, PRs will sync automatically!")
					}
				}
			}

			// 4. Create/update AGENTS.md guidelines
			if !jsonOutput {
				fmt.Println("\n4. Writing AGENTS.md guidelines...")
			}
			err = WriteAgentsMD(cwd)
			if err != nil {
				if jsonOutput {
					printJSON(false, fmt.Sprintf("Failed to write AGENTS.md: %v", err), nil)
				}
				return err
			}
			if !jsonOutput {
				fmt.Println("  ✅ AGENTS.md guidelines written successfully.")
			}

			_, _ = gitutils.ExecGit(cwd, "config", "metastackr.initialized", "true")

			if !jsonOutput {
				fmt.Printf("\n🎉 Onboarding Complete! MetaStackr is fully set up for this repository.\n")
				fmt.Printf("Webhook Secret used: %s\n", secret)
			} else {
				printJSON(true, "Repository onboarding completed successfully", map[string]interface{}{
					"repo_name":             repoName,
					"hooks_installed":       true,
					"webhooks_registered":   true,
					"agents_md_created":     true,
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
	cmd.Flags().BoolVar(&skipWebhooks, "skip-webhooks", false, "Skip repository-level webhook creation (use when GitHub App is installed)")

	return cmd
}
