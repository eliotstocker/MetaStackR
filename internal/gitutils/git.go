package gitutils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
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

func DetectVCSProvider(dir string, remoteURL string) string {
	if dir != "" {
		if cfgVal, err := ExecGit(dir, "config", "--get", "metastackr.vcs-provider"); err == nil && strings.TrimSpace(cfgVal) != "" {
			p := strings.ToLower(strings.TrimSpace(cfgVal))
			if p == "github" || p == "gitlab" {
				return p
			}
		}
		if cfgVal, err := ExecGit(dir, "config", "--get", "metastackr.vcs"); err == nil && strings.TrimSpace(cfgVal) != "" {
			p := strings.ToLower(strings.TrimSpace(cfgVal))
			if p == "github" || p == "gitlab" {
				return p
			}
		}
	}
	remoteURL = strings.ToLower(remoteURL)
	if strings.Contains(remoteURL, "gitlab") {
		return "gitlab"
	}
	if strings.Contains(remoteURL, "github") {
		return "github"
	}
	return "unknown"
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
	if strings.HasPrefix(rawURL, "git@gitlab.com:") {
		return strings.TrimPrefix(rawURL, "git@gitlab.com:")
	}
	if idx := strings.Index(rawURL, ":"); idx != -1 && strings.HasPrefix(rawURL, "git@") {
		return rawURL[idx+1:]
	}
	if strings.HasPrefix(rawURL, "https://github.com/") {
		return strings.TrimPrefix(rawURL, "https://github.com/")
	}
	if strings.HasPrefix(rawURL, "https://gitlab.com/") {
		return strings.TrimPrefix(rawURL, "https://gitlab.com/")
	}
	parts := strings.Split(rawURL, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return rawURL
}

func hasNoUpstream(dir string) bool {
	_, err := ExecGit(dir, "rev-parse", "--abbrev-ref", "@{u}")
	return err != nil
}

func getUnpushedCommits(dir string) int {
	out, err := ExecGit(dir, "log", "@{u}..HEAD", "--oneline")
	if err == nil {
		outStr := strings.TrimSpace(out)
		if outStr == "" {
			return 0
		}
		return len(strings.Split(outStr, "\n"))
	}

	branch, err := ExecGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || branch == "" || branch == "HEAD" {
		return 0
	}

	_, remoteErr := ExecGit(dir, "rev-parse", "--verify", "origin/"+branch)
	if remoteErr != nil {
		allCommits, _ := ExecGit(dir, "log", "origin/main..HEAD", "--oneline")
		if strings.TrimSpace(allCommits) == "" {
			allCommits, _ = ExecGit(dir, "log", "origin/master..HEAD", "--oneline")
		}
		if strings.TrimSpace(allCommits) != "" {
			return len(strings.Split(strings.TrimSpace(allCommits), "\n"))
		}
		headCount, _ := ExecGit(dir, "rev-list", "--count", "HEAD")
		if n, _ := strconv.Atoi(strings.TrimSpace(headCount)); n > 0 {
			return n
		}
		return 1
	}

	out, err = ExecGit(dir, "log", "origin/"+branch+"..HEAD", "--oneline")
	if err == nil && strings.TrimSpace(out) != "" {
		return len(strings.Split(strings.TrimSpace(out), "\n"))
	}

	return 0
}

func GetLocalStatus(rootDir string) (*MetaLocalStatus, error) {
	metaBranch, _ := ExecGit(rootDir, "rev-parse", "--abbrev-ref", "HEAD")
	uncommittedRaw, _ := ExecGit(rootDir, "status", "--porcelain")
	unpushedCount := getUnpushedCommits(rootDir)

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
			subUnpushedCount := getUnpushedCommits(subDir)

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
		if sub.Branch != "" && sub.Branch != "HEAD" {
			if sub.UnpushedCommits > 0 || sub.HasUncommitted || hasNoUpstream(subDir) {
				if _, err := ExecGit(subDir, "push", "-u", "origin", sub.Branch); err != nil {
					return fmt.Errorf("ABORTING PUSH: Failed to push submodule '%s': %w", sub.Path, err)
				}
			}
		}
	}

	if status.MetaBranch != "" && status.MetaBranch != "HEAD" {
		if _, err := ExecGit(rootDir, "push", "-u", "origin", status.MetaBranch); err != nil {
			return fmt.Errorf("failed to push root meta-repo: %w", err)
		}
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

	// 2. Sync parent repo branch with origin
	if status.MetaBranch != "" && status.MetaBranch != "HEAD" {
		_, _ = ExecGit(rootDir, "pull", "--rebase", "origin", status.MetaBranch)
	}

	// 3. Sync submodules with origin
	for _, sub := range status.Submodules {
		subDir := rootDir + "/" + sub.Path
		_, _ = ExecGit(subDir, "fetch", "origin")
		targetBranch := sub.Branch
		if targetBranch == "" || targetBranch == "HEAD" {
			targetBranch = "main"
		}
		_, _ = ExecGit(subDir, "pull", "--rebase", "origin", targetBranch)
	}

	// 4. Align submodule pointers in parent index
	for _, sub := range status.Submodules {
		subDir := rootDir + "/" + sub.Path
		if commit, err := ExecGit(subDir, "rev-parse", "HEAD"); err == nil {
			commit = strings.TrimSpace(commit)
			_, _ = ExecGit(rootDir, "update-index", "--cacheinfo", "160000", commit, sub.Path)
		}
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
					// Update existing webhook config via PATCH to ensure secret and URL match
					listURL := fmt.Sprintf("https://api.github.com/repos/%s/hooks", name)
					if listReq, err := http.NewRequest(http.MethodGet, listURL, nil); err == nil {
						listReq.Header.Set("Authorization", "token "+token)
						listReq.Header.Set("Accept", "application/vnd.github.v3+json")
						if listResp, err := client.Do(listReq); err == nil {
							var hooks []struct {
								ID     int64 `json:"id"`
								Config struct {
									URL string `json:"url"`
								} `json:"config"`
							}
							if json.NewDecoder(listResp.Body).Decode(&hooks) == nil {
								for _, h := range hooks {
									if h.Config.URL == targetURL || strings.Contains(h.Config.URL, "metastac.kr") {
										patchURL := fmt.Sprintf("https://api.github.com/repos/%s/hooks/%d", name, h.ID)
										patchPayload, _ := json.Marshal(map[string]interface{}{
											"config": map[string]interface{}{
												"url":          targetURL,
												"content_type": "json",
												"secret":       secret,
												"insecure_ssl": "0",
											},
										})
										if patchReq, err := http.NewRequest(http.MethodPatch, patchURL, bytes.NewReader(patchPayload)); err == nil {
											patchReq.Header.Set("Authorization", "token "+token)
											patchReq.Header.Set("Accept", "application/vnd.github.v3+json")
											patchReq.Header.Set("Content-Type", "application/json")
											if patchResp, err := client.Do(patchReq); err == nil {
												patchResp.Body.Close()
											}
										}
									}
								}
							}
							listResp.Body.Close()
						}
					}
					fmt.Printf("  ℹ️ Webhook updated for repository '%s'. Continuing...\n", name)
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

func GetGitLabToken() string {
	if token := os.Getenv("GITLAB_TOKEN"); token != "" {
		return token
	}

	out, err := exec.Command("glab", "auth", "status", "-t").CombinedOutput()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Token") || strings.Contains(line, "token") {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					tok := strings.TrimSpace(parts[len(parts)-1])
					if tok != "" && !strings.Contains(tok, "*") {
						return tok
					}
				}
			}
		}
	}
	return ""
}

func RegisterGitLabWebhook(rootDir, targetURL, secret, token string) error {
	repoName, err := GetMetaRepoName(rootDir)
	if err != nil {
		return fmt.Errorf("failed to get repository name: %w", err)
	}

	if token == "" {
		token = GetGitLabToken()
	}

	registerOne := func(name string) error {
		// 1. Try glab CLI API first if available (uses OS keyring authentication natively)
		if _, err := exec.LookPath("glab"); err == nil {
			glCmd := exec.Command("glab", "api", fmt.Sprintf("projects/%s/hooks", url.PathEscape(name)),
				"-X", "POST",
				"-F", fmt.Sprintf("url=%s", targetURL),
				"-F", fmt.Sprintf("token=%s", secret),
				"-F", "merge_requests_events=true",
				"-F", "push_events=true",
			)
			if glOut, glErr := glCmd.CombinedOutput(); glErr == nil {
				return nil
			} else {
				log.Printf("[gitlab-webhook] glab api attempt note for %s: %s", name, strings.TrimSpace(string(glOut)))
			}
		}

		// 2. Direct HTTP REST API request with PRIVATE-TOKEN
		payload := map[string]interface{}{
			"url":                     targetURL,
			"token":                   secret,
			"merge_requests_events":   true,
			"push_events":             true,
			"enable_ssl_verification": true,
		}

		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		apiURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/hooks", url.PathEscape(name))
		req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(jsonBytes))
		if err != nil {
			return err
		}

		if token != "" {
			req.Header.Set("PRIVATE-TOKEN", token)
		}
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make HTTP request to GitLab: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return fmt.Errorf("GitLab API HTTP %d (%s). Set GITLAB_TOKEN or run 'glab auth login'", resp.StatusCode, string(bodyBytes))
			}
			return fmt.Errorf("GitLab API returned status %d: %s", resp.StatusCode, string(bodyBytes))
		}
		return nil
	}

	fmt.Printf("Registering GitLab webhook for parent repository: %s...\n", repoName)
	if err := registerOne(repoName); err != nil {
		fmt.Printf("  ℹ️ Webhook registration note: %v\n", err)
	} else {
		fmt.Printf("  ✅ Parent repository GitLab webhook registered.\n")
	}

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

		fmt.Printf("Registering GitLab webhook for submodule: %s...\n", subRepo)
		if err := registerOne(subRepo); err != nil {
			fmt.Printf("  ⚠️ Submodule webhook registration note: %v\n", err)
		} else {
			fmt.Printf("  ✅ Submodule GitLab webhook registered.\n")
		}
	}

	return nil
}

func RegisterVCSWebhook(rootDir, targetURL, secret, token string) error {
	remoteURL, _ := ExecGit(rootDir, "config", "--get", "remote.origin.url")
	vcsType := DetectVCSProvider(rootDir, remoteURL)

	if vcsType == "gitlab" {
		if targetURL == "" || strings.HasSuffix(targetURL, "/webhooks/github") {
			targetURL = "https://api.metastac.kr/webhooks/gitlab"
		}
		return RegisterGitLabWebhook(rootDir, targetURL, secret, token)
	}

	if targetURL == "" {
		targetURL = "https://api.metastac.kr/webhooks/github"
	}
	return RegisterGitHubWebhook(rootDir, targetURL, secret, token)
}

type PRResult struct {
	RepoPath   string `json:"repo_path"`
	RepoName   string `json:"repo_name"`
	HeadBranch string `json:"head_branch"`
	BaseBranch string `json:"base_branch"`
	URL        string `json:"url"`
	Created    bool   `json:"created"`
	OpenedWeb  bool   `json:"opened_web"`
	Error      string `json:"error,omitempty"`
}

type CreatePROptions struct {
	BaseBranch  string
	Title       string
	Body        string
	MergeMethod string
	Draft       bool
	ForceWeb    bool
	Interactive bool
}

func OpenInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Run()
}

func CheckCLITools(vcsType string) (installed bool, toolName string, installInstructions string) {
	if vcsType == "gitlab" {
		if _, err := exec.LookPath("glab"); err == nil {
			return true, "glab", ""
		}
		return false, "glab", "install via 'brew install glab' or visit https://gitlab.com/gitlab-org/cli"
	}
	if _, err := exec.LookPath("gh"); err == nil {
		return true, "gh", ""
	}
	return false, "gh", "install via 'brew install gh' or visit https://cli.github.com"
}

func CreatePRs(rootDir string, opts CreatePROptions) ([]PRResult, error) {
	status, err := GetLocalStatus(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get local status: %w", err)
	}

	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	type targetRepo struct {
		path   string
		branch string
		isMeta bool
	}

	var targets []targetRepo

	// Check submodules first
	for _, sub := range status.Submodules {
		if sub.Branch != "" && sub.Branch != "HEAD" && sub.Branch != baseBranch {
			targets = append(targets, targetRepo{
				path:   sub.Path,
				branch: sub.Branch,
				isMeta: false,
			})
		}
	}

	// Check meta repo
	if status.MetaBranch != "" && status.MetaBranch != "HEAD" && status.MetaBranch != baseBranch {
		targets = append(targets, targetRepo{
			path:   ".",
			branch: status.MetaBranch,
			isMeta: true,
		})
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("no submodules or meta-repo have non-base feature branches (base: %s)", baseBranch)
	}

	if !opts.ForceWeb && opts.Interactive {
		missingTools := make(map[string]string)
		for _, target := range targets {
			dir := rootDir
			if !target.isMeta {
				dir = rootDir + "/" + target.path
			}
			remoteURL, _ := ExecGit(dir, "remote", "get-url", "origin")
			vcsType := DetectVCSProvider(dir, remoteURL)

			hasToken := false
			if vcsType == "gitlab" && os.Getenv("GITLAB_TOKEN") != "" {
				hasToken = true
			}
			if vcsType == "github" && GetGHToken() != "" {
				hasToken = true
			}

			if !hasToken {
				if installed, tool, instructions := CheckCLITools(vcsType); !installed {
					missingTools[tool] = instructions
				}
			}
		}

		if len(missingTools) > 0 {
			for tool, instructions := range missingTools {
				fmt.Printf("⚠️ CLI tool '%s' is not installed (%s).\n", tool, instructions)
			}
			fmt.Println("\nWould you like to:")
			fmt.Println("  [1] Open compare / PR pages in web browser to create PRs manually")
			fmt.Println("  [2] Quit to install missing CLI tool(s) first")
			fmt.Print("Enter choice [1/2] (default 1): ")

			var input string
			_, _ = fmt.Scanln(&input)
			input = strings.TrimSpace(input)
			if input == "2" {
				return nil, fmt.Errorf("aborted PR creation to install missing CLI tool(s)")
			}
		}
	}

	token := GetGHToken()

	var results []PRResult

	for _, target := range targets {
		dir := rootDir
		if !target.isMeta {
			dir = rootDir + "/" + target.path
		}

		repoName, err := GetMetaRepoName(dir)
		if err != nil {
			repoName = target.path
		}

		title := opts.Title
		if title == "" {
			lastCommitMsg, _ := ExecGit(dir, "log", "-1", "--pretty=%s")
			if strings.TrimSpace(lastCommitMsg) != "" {
				title = strings.TrimSpace(lastCommitMsg)
			} else {
				title = fmt.Sprintf("PR for %s", target.branch)
			}
		}

		remoteURL, _ := ExecGit(dir, "remote", "get-url", "origin")
		vcsType := DetectVCSProvider(dir, remoteURL)

		if vcsType == "unknown" {
			errMsg := fmt.Sprintf("unsupported VCS provider for remote origin '%s' (MetaStackr currently supports GitHub and GitLab)", remoteURL)
			if strings.TrimSpace(remoteURL) == "" {
				errMsg = "no remote origin URL found to detect VCS provider (MetaStackr currently supports GitHub and GitLab)"
			}
			res := PRResult{
				RepoPath:   target.path,
				RepoName:   repoName,
				HeadBranch: target.branch,
				BaseBranch: baseBranch,
				Error:      errMsg,
			}
			results = append(results, res)
			continue
		}

		compareURL := fmt.Sprintf("https://github.com/%s/compare/%s...%s?expand=1", repoName, baseBranch, target.branch)
		if vcsType == "gitlab" {
			compareURL = fmt.Sprintf("https://gitlab.com/%s/-/merge_requests/new?merge_request[source_branch]=%s&merge_request[target_branch]=%s", repoName, target.branch, baseBranch)
		}

		res := PRResult{
			RepoPath:   target.path,
			RepoName:   repoName,
			HeadBranch: target.branch,
			BaseBranch: baseBranch,
			URL:        compareURL,
		}

		body := opts.Body

		created := false
		if !opts.ForceWeb {
			if vcsType == "gitlab" {
				// 1. Try glab CLI
				glArgs := []string{"mr", "create", "--repo", repoName, "--target-branch", baseBranch, "--source-branch", target.branch, "--title", title, "--description", body, "--yes"}
				glCmd := exec.Command("glab", glArgs...)
				glCmd.Dir = dir
				var glOut bytes.Buffer
				glCmd.Stdout = &glOut

				if err := glCmd.Run(); err == nil {
					mrURL := strings.TrimSpace(glOut.String())
					if mrURL != "" {
						res.URL = mrURL
						res.Created = true
						created = true
					}
				}

				// 2. Fallback: GitLab REST API with GITLAB_TOKEN
				if !created && os.Getenv("GITLAB_TOKEN") != "" {
					glToken := os.Getenv("GITLAB_TOKEN")
					apiURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/merge_requests", url.PathEscape(repoName))
					mrPayload := map[string]interface{}{
						"source_branch": target.branch,
						"target_branch": baseBranch,
						"title":         title,
						"description":   body,
					}
					jsonBytes, _ := json.Marshal(mrPayload)
					req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonBytes))
					if err == nil {
						req.Header.Set("PRIVATE-TOKEN", glToken)
						req.Header.Set("Content-Type", "application/json")
						client := &http.Client{Timeout: 10 * time.Second}
						if resp, err := client.Do(req); err == nil {
							if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
								var mrRes struct {
									WebURL string `json:"web_url"`
								}
								if err := json.NewDecoder(resp.Body).Decode(&mrRes); err == nil && mrRes.WebURL != "" {
									res.URL = mrRes.WebURL
									res.Created = true
									created = true
								}
							}
							resp.Body.Close()
						}
					}
				}
			} else {
				// 1. Try gh CLI
				ghArgs := []string{"pr", "create", "--repo", repoName, "--base", baseBranch, "--head", target.branch, "--title", title, "--body", body}
				if opts.Draft {
					ghArgs = append(ghArgs, "--draft")
				}

				ghCmd := exec.Command("gh", ghArgs...)
				ghCmd.Dir = dir
				var ghOut, ghErrOut bytes.Buffer
				ghCmd.Stdout = &ghOut
				ghCmd.Stderr = &ghErrOut

				if err := ghCmd.Run(); err == nil {
					prURL := strings.TrimSpace(ghOut.String())
					if prURL != "" {
						res.URL = prURL
						res.Created = true
						created = true
					}
				}
			}

			// 2. Try GitHub REST API if gh CLI failed / not found
			if !created && token != "" {
				apiURL := fmt.Sprintf("https://api.github.com/repos/%s/pulls", repoName)
				payload := map[string]interface{}{
					"title": title,
					"head":  target.branch,
					"base":  baseBranch,
					"body":  body,
					"draft": opts.Draft,
				}
				jsonBytes, _ := json.Marshal(payload)

				req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(jsonBytes))
				if err == nil {
					req.Header.Set("Authorization", "token "+token)
					req.Header.Set("Accept", "application/vnd.github.v3+json")
					req.Header.Set("Content-Type", "application/json")

					client := &http.Client{}
					resp, err := client.Do(req)
					if err == nil {
						defer resp.Body.Close()
						if resp.StatusCode == http.StatusCreated {
							var apiResp struct {
								HTMLURL string `json:"html_url"`
							}
							bodyBytes, _ := io.ReadAll(resp.Body)
							_ = json.Unmarshal(bodyBytes, &apiResp)
							if apiResp.HTMLURL != "" {
								res.URL = apiResp.HTMLURL
								res.Created = true
								created = true
							}
						} else if resp.StatusCode == http.StatusUnprocessableEntity {
							res.Error = "PR already exists"
						}
					}
				}
			}
		}

		// 3. Fallback to opening compare URL in browser if not created
		if !created && res.Error == "" {
			if installed, tool, instructions := CheckCLITools(vcsType); !installed {
				fmt.Fprintf(os.Stderr, "  ℹ️ '%s' CLI tool is not installed (%s). Opening browser fallback...\n", tool, instructions)
			}
			_ = OpenInBrowser(compareURL)
			res.OpenedWeb = true
		}

		results = append(results, res)
	}

	return results, nil
}

type DirectPRStatus struct {
	PRNumber int    `json:"pr_number"`
	Status   string `json:"status"`
	URL      string `json:"url"`
}

func GetDirectPRStatus(dir, branch string) *DirectPRStatus {
	if branch == "" || branch == "main" || branch == "master" || branch == "HEAD" {
		return nil
	}

	repoName, err := GetMetaRepoName(dir)
	if err != nil {
		return nil
	}

	// 1. Try gh CLI
	cmd := exec.Command("gh", "pr", "view", branch, "--repo", repoName, "--json", "number,state,url")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		var ghPR struct {
			Number int    `json:"number"`
			State  string `json:"state"`
			URL    string `json:"url"`
		}
		if err := json.Unmarshal(out.Bytes(), &ghPR); err == nil && ghPR.Number > 0 {
			return &DirectPRStatus{
				PRNumber: ghPR.Number,
				Status:   ghPR.State,
				URL:      ghPR.URL,
			}
		}
	}

	// 2. Try GitHub REST API
	token := GetGHToken()
	if token != "" {
		parts := strings.Split(repoName, "/")
		owner := parts[0]
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/pulls?head=%s:%s", repoName, owner, branch)
		req, err := http.NewRequest(http.MethodGet, apiURL, nil)
		if err == nil {
			req.Header.Set("Authorization", "token "+token)
			req.Header.Set("Accept", "application/vnd.github.v3+json")
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var apiPRs []struct {
						Number  int    `json:"number"`
						State   string `json:"state"`
						HTMLURL string `json:"html_url"`
					}
					bodyBytes, _ := io.ReadAll(resp.Body)
					if err := json.Unmarshal(bodyBytes, &apiPRs); err == nil && len(apiPRs) > 0 {
						return &DirectPRStatus{
							PRNumber: apiPRs[0].Number,
							Status:   strings.ToUpper(apiPRs[0].State),
							URL:      apiPRs[0].HTMLURL,
						}
					}
				}
			}
		}
	}

	return nil
}
