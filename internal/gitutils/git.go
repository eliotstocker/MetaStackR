package gitutils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type SubmoduleStatus struct {
	Path              string `json:"path"`
	Branch            string `json:"branch"`
	HasUncommitted    bool   `json:"has_uncommitted"`
	UnpushedCommits   int    `json:"unpushed_commits"`
	CurrentHeadCommit string `json:"current_head_commit"`
}

type MetaLocalStatus struct {
	MetaRepoPath      string            `json:"meta_repo_path"`
	MetaBranch        string            `json:"meta_branch"`
	HasUncommitted    bool              `json:"has_uncommitted"`
	UnpushedCommits   int               `json:"unpushed_commits"`
	Submodules        []SubmoduleStatus `json:"submodules"`
}

func ExecGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
	}

	return strings.TrimSpace(out.String()), nil
}

func GetGHToken() string {
	if token := os.Getenv("GH_TOKEN"); token != "" {
		return token
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func GetMetaRepoName(dir string) (string, error) {
	url, err := ExecGit(dir, "config", "--get", "remote.origin.url")
	if err != nil {
		return "", err
	}
	return parseRepoOwnerAndName(url), nil
}

func parseRepoOwnerAndName(rawURL string) string {
	rawURL = strings.TrimSuffix(rawURL, ".git")
	if strings.HasPrefix(rawURL, "git@github.com:") {
		return strings.TrimPrefix(rawURL, "git@github.com:")
	}
	if strings.HasPrefix(rawURL, "https://github.com/") {
		return strings.TrimPrefix(rawURL, "https://github.com/")
	}
	parts := strings.Split(rawURL, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return rawURL
}

func GetLocalStatus(rootDir string) (*MetaLocalStatus, error) {
	metaBranch, _ := ExecGit(rootDir, "rev-parse", "--abbrev-ref", "HEAD")
	uncommittedRaw, _ := ExecGit(rootDir, "status", "--porcelain")
	unpushedRaw, _ := ExecGit(rootDir, "log", "@{u}..HEAD", "--oneline")

	unpushedCount := 0
	if strings.TrimSpace(unpushedRaw) != "" {
		unpushedCount = len(strings.Split(strings.TrimSpace(unpushedRaw), "\n"))
	}

	status := &MetaLocalStatus{
		MetaRepoPath:    rootDir,
		MetaBranch:      metaBranch,
		HasUncommitted:  strings.TrimSpace(uncommittedRaw) != "",
		UnpushedCommits: unpushedCount,
		Submodules:      []SubmoduleStatus{},
	}

	subOut, err := ExecGit(rootDir, "submodule", "status")
	if err == nil && strings.TrimSpace(subOut) != "" {
		lines := strings.Split(strings.TrimSpace(subOut), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			sha := strings.TrimPrefix(parts[0], "-")
			sha = strings.TrimPrefix(sha, "+")
			
			rest := strings.TrimSpace(line[len(parts[0]):])
			subPath := rest
			if idx := strings.LastIndex(rest, " ("); idx != -1 {
				subPath = rest[:idx]
			}

			subDir := rootDir + "/" + subPath
			subBranch, _ := ExecGit(subDir, "rev-parse", "--abbrev-ref", "HEAD")
			subUncommitted, _ := ExecGit(subDir, "status", "--porcelain")
			subUnpushed, _ := ExecGit(subDir, "log", "@{u}..HEAD", "--oneline")

			subUnpushedCount := 0
			if strings.TrimSpace(subUnpushed) != "" {
				subUnpushedCount = len(strings.Split(strings.TrimSpace(subUnpushed), "\n"))
			}

			status.Submodules = append(status.Submodules, SubmoduleStatus{
				Path:              subPath,
				Branch:            subBranch,
				HasUncommitted:    strings.TrimSpace(subUncommitted) != "",
				UnpushedCommits:   subUnpushedCount,
				CurrentHeadCommit: sha,
			})
		}
	}

	return status, nil
}

func CheckoutBranch(rootDir string, branchName string, create bool) error {
	args := []string{"checkout"}
	if create {
		args = append(args, "-b")
	}
	args = append(args, branchName)

	if _, err := ExecGit(rootDir, args...); err != nil {
		return fmt.Errorf("failed root checkout: %w", err)
	}

	subOut, err := ExecGit(rootDir, "submodule", "status")
	if err != nil || strings.TrimSpace(subOut) == "" {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(subOut), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		rest := strings.TrimSpace(line[len(parts[0]):])
		subPath := rest
		if idx := strings.LastIndex(rest, " ("); idx != -1 {
			subPath = rest[:idx]
		}
		subDir := rootDir + "/" + subPath

		if _, err := ExecGit(subDir, args...); err != nil {
			if create {
				if _, fallbackErr := ExecGit(subDir, "checkout", branchName); fallbackErr == nil {
					continue
				}
			}
			return fmt.Errorf("failed checkout in submodule '%s': %w", subPath, err)
		}
	}

	return nil
}

func CommitAtomic(rootDir, message string) error {
	status, err := GetLocalStatus(rootDir)
	if err != nil {
		return err
	}

	// 1. Commit all modified submodules
	var modifiedSubmodules []string
	for _, sub := range status.Submodules {
		subDir := rootDir + "/" + sub.Path
		if sub.HasUncommitted {
			if _, err := ExecGit(subDir, "add", "-A"); err != nil {
				return fmt.Errorf("failed git add in submodule %s: %w", sub.Path, err)
			}
			if _, err := ExecGit(subDir, "commit", "-m", message); err != nil {
				return fmt.Errorf("failed git commit in submodule %s: %w", sub.Path, err)
			}
			modifiedSubmodules = append(modifiedSubmodules, sub.Path)
		}
	}

	// 2. Stage updated submodule references and commit parent repo
	if len(modifiedSubmodules) > 0 {
		for _, path := range modifiedSubmodules {
			if _, err := ExecGit(rootDir, "add", path); err != nil {
				return fmt.Errorf("failed to add submodule ref %s to parent: %w", path, err)
			}
		}
	}

	// Also add any other modified files in parent repo
	if _, err := ExecGit(rootDir, "commit", "-am", message); err != nil {
		return fmt.Errorf("failed parent git commit: %w", err)
	}

	return nil
}

func PushBottomUp(rootDir string) error {
	status, err := GetLocalStatus(rootDir)
	if err != nil {
		return err
	}

	for _, sub := range status.Submodules {
		subDir := rootDir + "/" + sub.Path
		if sub.UnpushedCommits > 0 || sub.HasUncommitted {
			if _, err := ExecGit(subDir, "push", "origin", sub.Branch); err != nil {
				return fmt.Errorf("ABORTING PUSH: Failed to push submodule '%s': %w", sub.Path, err)
			}
		}
	}

	if _, err := ExecGit(rootDir, "push", "origin", status.MetaBranch); err != nil {
		return fmt.Errorf("failed to push root meta-repo: %w", err)
	}

	return nil
}

func SyncUpstream(rootDir string) error {
	// 1. Fetch parent origin
	if _, err := ExecGit(rootDir, "fetch", "origin"); err != nil {
		return err
	}

	status, err := GetLocalStatus(rootDir)
	if err != nil {
		return err
	}

	// 2. Sync submodules
	for _, sub := range status.Submodules {
		subDir := rootDir + "/" + sub.Path
		_, _ = ExecGit(subDir, "fetch", "origin")
		// Fast-forward rebase local changes onto matching origin branch
		if _, err := ExecGit(subDir, "rebase", "origin/main"); err != nil {
			return fmt.Errorf("failed to rebase submodule %s: %w", sub.Path, err)
		}
	}

	// 3. Stage changes
	if _, err := ExecGit(rootDir, "add", "--all"); err == nil {
		_, _ = ExecGit(rootDir, "commit", "-m", "chore: sync submodule references")
	}

	return nil
}

func RebaseUpstream(rootDir, upstream string) error {
	status, err := GetLocalStatus(rootDir)
	if err != nil {
		return err
	}

	// Rebase submodules
	for _, sub := range status.Submodules {
		subDir := rootDir + "/" + sub.Path
		if _, err := ExecGit(subDir, "rebase", "origin/"+upstream); err != nil {
			return fmt.Errorf("failed submodule rebase in %s: %w", sub.Path, err)
		}
	}

	// Rebase parent and resolve pointers
	if _, err := ExecGit(rootDir, "rebase", "origin/"+upstream); err != nil {
		return fmt.Errorf("failed parent rebase: %w", err)
	}

	return nil
}

func RegisterGitHubWebhook(rootDir, targetURL, secret, token string) error {
	repoName, err := GetMetaRepoName(rootDir)
	if err != nil {
		return fmt.Errorf("failed to get repository name: %w", err)
	}

	if token == "" {
		token = GetGHToken()
	}
	if token == "" {
		return fmt.Errorf("missing GitHub token. Please run 'gh auth login' or set GITHUB_TOKEN environment variable")
	}

	registerOne := func(name string) error {
		payload := map[string]interface{}{
			"name":   "web",
			"active": true,
			"events": []string{"pull_request", "pull_request_review", "check_run", "workflow_run"},
			"config": map[string]interface{}{
				"url":          targetURL,
				"content_type": "json",
				"secret":       secret,
				"insecure_ssl": "0",
			},
		}

		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		url := fmt.Sprintf("https://api.github.com/repos/%s/hooks", name)
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(jsonBytes))
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", "token "+token)
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make HTTP request to GitHub: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			var errResp struct {
				Message string `json:"message"`
				Errors  []struct {
					Resource string `json:"resource"`
					Code     string `json:"code"`
					Message  string `json:"message"`
				} `json:"errors"`
			}
			bodyBytes, _ := io.ReadAll(resp.Body)
			_ = json.Unmarshal(bodyBytes, &errResp)

			if resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusConflict {
				isAlreadyExists := false
				if strings.Contains(strings.ToLower(errResp.Message), "already exists") || strings.EqualFold(errResp.Message, "Validation Failed") {
					isAlreadyExists = true
				}
				for _, e := range errResp.Errors {
					if strings.Contains(strings.ToLower(e.Message), "already exists") {
						isAlreadyExists = true
						break
					}
				}
				if isAlreadyExists {
					fmt.Printf("  ℹ️ Webhook already registered for repository '%s'. Continuing...\n", name)
					return nil
				}
			}

			if errResp.Message != "" {
				return fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, errResp.Message)
			}
			return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
		}
		return nil
	}

	fmt.Printf("Registering webhook for parent repository: %s...\n", repoName)
	if err := registerOne(repoName); err != nil {
		return fmt.Errorf("parent repository registration failed: %w", err)
	}
	fmt.Printf("  ✅ Parent repository webhook registered.\n")

	status, err := GetLocalStatus(rootDir)
	if err != nil {
		fmt.Printf("  ⚠️ Could not query submodules: %v\n", err)
		return nil
	}

	for _, sub := range status.Submodules {
		subDir := rootDir + "/" + sub.Path
		subRepo, err := GetMetaRepoName(subDir)
		if err != nil {
			fmt.Printf("  ⚠️ Could not resolve repository name for submodule '%s' (%s): %v\n", sub.Path, subDir, err)
			continue
		}

		fmt.Printf("Registering webhook for submodule: %s...\n", subRepo)
		if err := registerOne(subRepo); err != nil {
			fmt.Printf("  ⚠️ Submodule registration failed: %v\n", err)
		} else {
			fmt.Printf("  ✅ Submodule webhook registered.\n")
		}
	}

	return nil
}
