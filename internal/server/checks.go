package server

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"metastackr/internal/db"
	"metastackr/internal/gitutils"
	"metastackr/internal/vcs"
)

type SubmodulePointerUpdate = vcs.SubmodulePointerUpdate

type cachedToken struct {
	Token     string
	ExpiresAt time.Time
}

type GitHubClient struct {
	token      string
	baseURL    string
	httpClient *http.Client

	appID      string
	privateKey *rsa.PrivateKey
	tokenCache map[int64]cachedToken
	cacheMu    sync.RWMutex
	repo       *db.Repository
}

func NewGitHubClient(token string) *GitHubClient {
	if token == "" {
		token = "dummy_token"
	}
	return &GitHubClient{
		token:      token,
		baseURL:    "https://api.github.com",
		httpClient: &http.Client{},
		tokenCache: make(map[int64]cachedToken),
	}
}

func (c *GitHubClient) SetRepository(repo *db.Repository) {
	c.repo = repo
}

func NewGitHubClientWithApp(appID string, privateKeyPEM string, defaultToken string) (*GitHubClient, error) {
	client := NewGitHubClient(defaultToken)
	if appID != "" && privateKeyPEM != "" {
		key, err := parsePrivateKey([]byte(privateKeyPEM))
		if err != nil {
			log.Printf("[warning] Failed to parse GITHUB_PRIVATE_KEY: %v", err)
		} else {
			client.appID = appID
			client.privateKey = key
			log.Printf("[info] Initialized GitHub App client for App ID %s", appID)
		}
	}
	return client, nil
}

func parsePrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	pemStr := strings.ReplaceAll(string(pemBytes), "\\n", "\n")
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from private key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}
	return nil, fmt.Errorf("unsupported or invalid RSA private key format")
}

func generateJWT(appID string, privateKey *rsa.PrivateKey) (string, error) {
	headerJSON := []byte(`{"alg":"RS256","typ":"JWT"}`)
	now := time.Now().Unix()
	claimsJSON, err := json.Marshal(map[string]interface{}{
		"iss": appID,
		"iat": now - 60,
		"exp": now + 600,
	})
	if err != nil {
		return "", err
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := headerB64 + "." + claimsB64

	hashed := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", err
	}

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return signingInput + "." + sigB64, nil
}

// HasAppAccessToRepo checks whether the GitHub App has been granted access to a specific repository.
func (c *GitHubClient) HasAppAccessToRepo(ctx context.Context, repoFullName string) (bool, int64) {
	if c == nil || c.privateKey == nil || c.appID == "" {
		return false, 0
	}

	jwtStr, err := generateJWT(c.appID, c.privateKey)
	if err != nil {
		return false, 0
	}

	url := fmt.Sprintf("%s/repos/%s/installation", c.baseURL, repoFullName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, 0
	}

	req.Header.Set("Authorization", "Bearer "+jwtStr)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, 0
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var result struct {
			ID int64 `json:"id"`
		}
		if json.NewDecoder(resp.Body).Decode(&result) == nil && result.ID > 0 {
			return true, result.ID
		}
	}

	return false, 0
}

// HasAppInstalledOnAccount checks whether the GitHub App is installed on the user or org account.
func (c *GitHubClient) HasAppInstalledOnAccount(ctx context.Context, owner string) bool {
	if c == nil || c.privateKey == nil || c.appID == "" || owner == "" {
		return false
	}

	jwtStr, err := generateJWT(c.appID, c.privateKey)
	if err != nil {
		return false
	}

	for _, endpoint := range []string{"users", "orgs"} {
		url := fmt.Sprintf("%s/%s/%s/installation", c.baseURL, endpoint, owner)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+jwtStr)
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		resp, err := c.httpClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
	}

	return false
}

// GetInstallationToken exchanges a GitHub App JWT for a short-lived installation access token.
func (c *GitHubClient) GetInstallationToken(ctx context.Context, installationID int64) (string, error) {
	if installationID == 0 || c.privateKey == nil {
		return c.token, nil
	}

	c.cacheMu.RLock()
	if cached, exists := c.tokenCache[installationID]; exists {
		if time.Now().Add(2 * time.Minute).Before(cached.ExpiresAt) {
			c.cacheMu.RUnlock()
			return cached.Token, nil
		}
	}
	c.cacheMu.RUnlock()

	// Check DB cache for cross-container persistence
	if c.repo != nil {
		if dbToken, err := c.repo.GetCachedInstallationToken(ctx, installationID); err == nil && dbToken != "" {
			c.cacheMu.Lock()
			c.tokenCache[installationID] = cachedToken{
				Token:     dbToken,
				ExpiresAt: time.Now().Add(50 * time.Minute),
			}
			c.cacheMu.Unlock()
			return dbToken, nil
		}
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	// Double check after acquiring write lock
	if cached, exists := c.tokenCache[installationID]; exists {
		if time.Now().Add(2 * time.Minute).Before(cached.ExpiresAt) {
			return cached.Token, nil
		}
	}

	jwtStr, err := generateJWT(c.appID, c.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate GitHub App JWT: %w", err)
	}

	log.Printf("[auth] Requesting installation token for installation %d (appID: %s, hasKey: %v)", installationID, c.appID, c.privateKey != nil)
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", c.baseURL, installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwtStr)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[auth] HTTP client error requesting token: %v", err)
		return "", fmt.Errorf("failed to request installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[auth] Installation token error HTTP %d: %s", resp.StatusCode, string(body))
		fallbackToken := c.token
		if fallbackToken == "" {
			fallbackToken = gitutils.GetGHToken()
		}
		if fallbackToken != "" && (resp.StatusCode == 403 || resp.StatusCode == 429) {
			log.Printf("[auth] GitHub App rate limit exceeded (HTTP %d). Falling back to PAT token.", resp.StatusCode)
			return fallbackToken, nil
		}
		return "", fmt.Errorf("installation token API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	c.tokenCache[installationID] = cachedToken{
		Token:     tokenResp.Token,
		ExpiresAt: tokenResp.ExpiresAt,
	}

	if c.repo != nil {
		_ = c.repo.SaveInstallationToken(ctx, installationID, tokenResp.Token, tokenResp.ExpiresAt)
	}

	return tokenResp.Token, nil
}

type CheckRunOutput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Text    string `json:"text"`
}

type CheckRunPayload struct {
	Name       string         `json:"name"`
	HeadSHA    string         `json:"head_sha"`
	Status     string         `json:"status"`               // "queued", "in_progress", "completed"
	Conclusion *string        `json:"conclusion,omitempty"` // "success", "failure", "neutral", "cancelled", etc.
	Output     CheckRunOutput `json:"output"`
}

// GenerateMarkdownTable produces a markdown summary matrix of all child PR states.
func GenerateMarkdownTable(metaPR *db.MetaPR) (title string, summary string, text string) {
	isGitLab := strings.Contains(strings.ToLower(metaPR.MetaRepoFullName), "gitlab")

	title = fmt.Sprintf("Meta-Repo Sync Status: %s", metaPR.Status)
	if isGitLab {
		summary = fmt.Sprintf("Meta MR !%d (%s) - Submodule Merge Request Matrix", metaPR.PRNumber, metaPR.BranchName)
	} else {
		summary = fmt.Sprintf("Meta PR #%d (%s) - Submodule PR Matrix", metaPR.PRNumber, metaPR.BranchName)
	}

	var sb strings.Builder
	if len(metaPR.ChildPRs) == 0 {
		if isGitLab {
			sb.WriteString("### 🔄 Submodule Synchronization\n\nNo submodule Merge Requests affected\n")
		} else {
			sb.WriteString("### 🔄 Submodule Synchronization\n\nNo submodule Pull Requests affected\n")
		}
	} else {
		if isGitLab {
			sb.WriteString("### 🔄 Submodule Merge Request Matrix\n\n")
			sb.WriteString("| Submodule Path | Child Repo | MR ! | Head SHA | Status |\n")
		} else {
			sb.WriteString("### 🔄 Submodule Synchronization Matrix\n\n")
			sb.WriteString("| Submodule Path | Child Repo | PR # | Head SHA | Status |\n")
		}
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
		for _, child := range metaPR.ChildPRs {
			childIsGitLab := isGitLab || strings.Contains(strings.ToLower(child.RepoFullName), "gitlab")
			prLink := fmt.Sprintf("#%d", child.PRNumber)
			if child.PRNumber > 0 {
				if childIsGitLab {
					prLink = fmt.Sprintf("[!%d](https://gitlab.com/%s/-/merge_requests/%d)", child.PRNumber, child.RepoFullName, child.PRNumber)
				} else {
					prLink = fmt.Sprintf("[%d](https://github.com/%s/pull/%d)", child.PRNumber, child.RepoFullName, child.PRNumber)
				}
			}

			shaStr := child.HeadSHA
			if len(shaStr) > 7 {
				shaStr = shaStr[:7]
			}

			sb.WriteString(fmt.Sprintf(
				"| `%s` | `%s` | %s | `%s` | **%s** |\n",
				child.SubmodulePath, child.RepoFullName, prLink, shaStr, child.Status,
			))
		}
	}

	sb.WriteString("\n\n---\n*Updated automatically by `metastackr` orchestration engine.*")
	return title, summary, sb.String()
}

// GetInstallationTokenForRepo resolves installation ID for a repo if zero, then fetches access token.
func (c *GitHubClient) GetInstallationTokenForRepo(ctx context.Context, repoFullName string, installationID int64) (string, error) {
	if c.privateKey == nil {
		return c.token, nil
	}

	// 1. Query GitHub API for the specific installation ID of target repoFullName
	jwtStr, err := generateJWT(c.appID, c.privateKey)
	if err == nil {
		url := fmt.Sprintf("%s/repos/%s/installation", c.baseURL, repoFullName)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+jwtStr)
			req.Header.Set("Accept", "application/vnd.github+json")
			if resp, err := c.httpClient.Do(req); err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var instResp struct {
						ID int64 `json:"id"`
					}
					if json.NewDecoder(resp.Body).Decode(&instResp) == nil && instResp.ID > 0 {
						return c.GetInstallationToken(ctx, instResp.ID)
					}
				}
			}
		}
	}

	// 2. Fallback to provided installationID if repo-specific lookup failed
	if installationID > 0 {
		return c.GetInstallationToken(ctx, installationID)
	}

	return c.token, nil
}

// UpdateMetaCheckRun creates or updates the single GitHub Check Run named 'meta-repo/sync'.
func (c *GitHubClient) UpdateMetaCheckRun(ctx context.Context, metaRepo string, headSHA string, metaPR *db.MetaPR, installationID int64) error {
	token, err := c.GetInstallationTokenForRepo(ctx, metaRepo, installationID)
	if err != nil || token == "" {
		if c.token != "" {
			token = c.token
		} else {
			token = gitutils.GetGHToken()
		}
	}

	title, summary, markdownText := GenerateMarkdownTable(metaPR)

	status := "in_progress"
	var conclusion *string

	switch metaPR.Status {
	case "MERGED":
		status = "completed"
		succ := "success"
		conclusion = &succ
	case "FAILED", "FAILED_PARTIAL", "FAILED_DRIFT":
		status = "completed"
		fail := "failure"
		conclusion = &fail
	}

	payload := CheckRunPayload{
		Name:       "meta-repo/sync",
		HeadSHA:    headSHA,
		Status:     status,
		Conclusion: conclusion,
		Output: CheckRunOutput{
			Title:   title,
			Summary: summary,
			Text:    markdownText,
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/repos/%s/check-runs", c.baseURL, metaRepo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to post check run to GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API returned HTTP %d for check run update on %s: %s", resp.StatusCode, metaRepo, string(body))
	}

	log.Printf("[checks] Successfully posted check run 'meta-repo/sync' (status: %s) for %s SHA %s", status, metaRepo, headSHA)
	return nil
}

// GetPRHeadSHA fetches the head commit SHA for a pull request from GitHub API.
func (c *GitHubClient) GetPRHeadSHA(ctx context.Context, repoFullName string, prNumber int, installationID int64) (string, error) {
	token, err := c.GetInstallationTokenForRepo(ctx, repoFullName, installationID)
	if err != nil {
		return "", fmt.Errorf("failed to acquire token: %w", err)
	}

	url := fmt.Sprintf("%s/repos/%s/pulls/%d", c.baseURL, repoFullName, prNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GitHub API returned HTTP %d for pull request %s#%d", resp.StatusCode, repoFullName, prNumber)
	}

	var prPayload struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prPayload); err != nil {
		return "", err
	}

	return prPayload.Head.SHA, nil
}

// GetOpenPRForBranch checks if a repository has an open or merged PR matching the branch name.
func (c *GitHubClient) GetOpenPRForBranch(ctx context.Context, repoFullName string, branchName string, installationID int64) (int, string, bool, error) {
	if repoFullName == "" || branchName == "" {
		return 0, "", false, nil
	}

	token, err := c.GetInstallationTokenForRepo(ctx, repoFullName, installationID)
	if err != nil || token == "" {
		if c.token != "" {
			token = c.token
		} else {
			token = gitutils.GetGHToken()
		}
	}

	parts := strings.Split(repoFullName, "/")
	owner := parts[0]
	url := fmt.Sprintf("%s/repos/%s/pulls?head=%s:%s&state=all", c.baseURL, repoFullName, owner, branchName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, "", false, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden && token != gitutils.GetGHToken() && gitutils.GetGHToken() != "" {
		// Retry with personal token fallback if installation token is rate-limited
		fallbackReq, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		fallbackReq.Header.Set("Authorization", "Bearer "+gitutils.GetGHToken())
		fallbackReq.Header.Set("Accept", "application/vnd.github.v3+json")
		if fallbackResp, fallbackErr := c.httpClient.Do(fallbackReq); fallbackErr == nil {
			defer fallbackResp.Body.Close()
			if fallbackResp.StatusCode >= 200 && fallbackResp.StatusCode < 300 {
				resp = fallbackResp
			}
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, "", false, fmt.Errorf("GitHub API returned HTTP %d for pulls on %s", resp.StatusCode, repoFullName)
	}

	var prs []struct {
		Number int  `json:"number"`
		Merged bool `json:"merged"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		return 0, "", false, err
	}

	if len(prs) > 0 {
		return prs[0].Number, prs[0].Head.SHA, prs[0].Merged, nil
	}

	return 0, "", false, nil
}

// MergePullRequest merges a pull request on GitHub via REST API PUT /repos/{owner}/{repo}/pulls/{number}/merge.
func (c *GitHubClient) MergePullRequest(ctx context.Context, repoFullName string, prNumber int, mergeMethod string, installationID int64) (string, error) {
	if repoFullName == "" || prNumber <= 0 {
		return "", fmt.Errorf("invalid repository or PR number")
	}

	if mergeMethod == "" {
		mergeMethod = "merge"
	}

	token, err := c.GetInstallationTokenForRepo(ctx, repoFullName, installationID)
	if err != nil || token == "" {
		if c.token != "" {
			token = c.token
		} else {
			token = gitutils.GetGHToken()
		}
	}

	url := fmt.Sprintf("%s/repos/%s/pulls/%d/merge", c.baseURL, repoFullName, prNumber)
	payload := map[string]string{
		"merge_method": mergeMethod,
	}
	bodyBytes, _ := json.Marshal(payload)

	var resp *http.Response
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*500) * time.Millisecond)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		req.Header.Set("Content-Type", "application/json")

		resp, err = c.doRequestWithPATFallback(ctx, req, token, bodyBytes)
		if err != nil {
			return "", fmt.Errorf("failed to call GitHub merge API: %w", err)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			break
		}

		if (resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusMethodNotAllowed) && attempt < 4 {
			resp.Body.Close()
			continue
		}
		break
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API returned HTTP %d for merge on %s#%d: %s", resp.StatusCode, repoFullName, prNumber, string(body))
	}

	var result struct {
		SHA     string `json:"sha"`
		Merged  bool   `json:"merged"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	log.Printf("[merger] Successfully merged %s#%d (SHA: %s)", repoFullName, prNumber, result.SHA)
	return result.SHA, nil
}

// EnsureChildPRComment posts a standard comment on a child PR referencing its parent Meta PR if not already posted.
func (c *GitHubClient) EnsureChildPRComment(ctx context.Context, childRepo string, prNumber int, parentMetaRepo string, parentPRNumber int, branchName string, installationID int64) error {
	if prNumber <= 0 || childRepo == "" || parentMetaRepo == "" || parentPRNumber <= 0 {
		return nil
	}

	token, err := c.GetInstallationTokenForRepo(ctx, childRepo, installationID)
	if err != nil || token == "" {
		if c.token != "" {
			token = c.token
		} else {
			token = gitutils.GetGHToken()
		}
	}

	marker := "<!-- metastackr-child-pr-comment -->"
	parentURL := fmt.Sprintf("https://github.com/%s/pull/%d", parentMetaRepo, parentPRNumber)
	commentBody := fmt.Sprintf("%s\n### ⚡ MetaStackr Orchestration\nThis Pull Request is tracked as part of Parent Meta PR **[%s#%d](%s)** (`%s`).\n\n*Automated cascade merge will execute once all child submodule PRs are approved and merged.*", marker, parentMetaRepo, parentPRNumber, parentURL, branchName)

	listURL := fmt.Sprintf("%s/repos/%s/issues/%d/comments", c.baseURL, childRepo, prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to list issue comments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var comments []struct {
			ID   int64  `json:"id"`
			Body string `json:"body"`
		}
		if json.NewDecoder(resp.Body).Decode(&comments) == nil {
			for _, comment := range comments {
				if strings.Contains(comment.Body, marker) {
					return nil
				}
			}
		}
	}

	postPayload := map[string]string{"body": commentBody}
	bodyBytes, _ := json.Marshal(postPayload)

	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, listURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	postReq.Header.Set("Authorization", "Bearer "+token)
	postReq.Header.Set("Accept", "application/vnd.github+json")
	postReq.Header.Set("Content-Type", "application/json")

	postResp, err := c.httpClient.Do(postReq)
	if err != nil {
		return fmt.Errorf("failed to post comment: %w", err)
	}
	defer postResp.Body.Close()

	log.Printf("[comments] Posted MetaStackr linkage comment on %s#%d (Parent: %s#%d)", childRepo, prNumber, parentMetaRepo, parentPRNumber)
	return nil
}

// EnsureRootPRDescriptionBody embeds/updates the matrix table directly in the main PR description body (e.g. #issue-xxx),
// avoiding separate issue comments and cleaning up any legacy comments.
func (c *GitHubClient) EnsureRootPRDescriptionBody(ctx context.Context, metaRepo string, prNumber int, metaPR *db.MetaPR, installationID int64) error {
	if prNumber <= 0 || metaRepo == "" || metaPR == nil {
		return nil
	}

	token, err := c.GetInstallationTokenForRepo(ctx, metaRepo, installationID)
	if err != nil || token == "" {
		if c.token != "" {
			token = c.token
		} else {
			token = gitutils.GetGHToken()
		}
	}

	startMarker := "<!-- metastackr-matrix-start -->"
	endMarker := "<!-- metastackr-matrix-end -->"
	_, _, tableMarkdown := GenerateMarkdownTable(metaPR)
	matrixBlock := fmt.Sprintf("%s\n%s\n%s", startMarker, tableMarkdown, endMarker)

	// 1. Fetch current PR description body
	prURL := fmt.Sprintf("%s/repos/%s/pulls/%d", c.baseURL, metaRepo, prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, prURL, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch PR description: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden && token != gitutils.GetGHToken() && gitutils.GetGHToken() != "" {
		retryReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, prURL, nil)
		retryReq.Header.Set("Authorization", "Bearer "+gitutils.GetGHToken())
		retryReq.Header.Set("Accept", "application/vnd.github.v3+json")
		if retryResp, retryErr := c.httpClient.Do(retryReq); retryErr == nil {
			defer retryResp.Body.Close()
			if retryResp.StatusCode >= 200 && retryResp.StatusCode < 300 {
				resp = retryResp
				token = gitutils.GetGHToken()
			}
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GitHub API returned HTTP %d for pull request %s#%d", resp.StatusCode, metaRepo, prNumber)
	}

	var prPayload struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prPayload); err != nil {
		return err
	}

	currentBody := prPayload.Body
	var newBody string

	if strings.Contains(currentBody, startMarker) && strings.Contains(currentBody, endMarker) {
		startIndex := strings.Index(currentBody, startMarker)
		endIndex := strings.Index(currentBody, endMarker) + len(endMarker)
		newBody = currentBody[:startIndex] + matrixBlock + currentBody[endIndex:]
	} else {
		if strings.TrimSpace(currentBody) == "" {
			newBody = matrixBlock
		} else {
			newBody = fmt.Sprintf("%s\n\n---\n%s", currentBody, matrixBlock)
		}
	}

	if newBody != currentBody {
		patchPayload := map[string]string{"body": newBody}
		bodyBytes, _ := json.Marshal(patchPayload)

		patchReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, prURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return err
		}
		patchReq.Header.Set("Authorization", "Bearer "+token)
		patchReq.Header.Set("Accept", "application/vnd.github.v3+json")
		patchReq.Header.Set("Content-Type", "application/json")

		patchResp, err := c.httpClient.Do(patchReq)
		if err == nil {
			defer patchResp.Body.Close()
			if patchResp.StatusCode == http.StatusForbidden && token != gitutils.GetGHToken() && gitutils.GetGHToken() != "" {
				retryPatch, _ := http.NewRequestWithContext(ctx, http.MethodPatch, prURL, bytes.NewReader(bodyBytes))
				retryPatch.Header.Set("Authorization", "Bearer "+gitutils.GetGHToken())
				retryPatch.Header.Set("Accept", "application/vnd.github.v3+json")
				retryPatch.Header.Set("Content-Type", "application/json")
				if rResp, rErr := c.httpClient.Do(retryPatch); rErr == nil {
					rResp.Body.Close()
				}
			}
			log.Printf("[description] Merged matrix into PR description body for %s#%d", metaRepo, prNumber)
		}
	}

	// 2. Also clean up any separate root issue comments so there are no extra comments
	_ = c.CleanupRootPRComments(ctx, metaRepo, prNumber, installationID)

	return nil
}

// CleanupRootPRComments deletes any separate issue comments containing the root comment marker.
func (c *GitHubClient) CleanupRootPRComments(ctx context.Context, metaRepo string, prNumber int, installationID int64) error {
	token, err := c.GetInstallationTokenForRepo(ctx, metaRepo, installationID)
	if err != nil || token == "" {
		if c.token != "" {
			token = c.token
		} else {
			token = gitutils.GetGHToken()
		}
	}

	listURL := fmt.Sprintf("%s/repos/%s/issues/%d/comments", c.baseURL, metaRepo, prNumber)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden && token != gitutils.GetGHToken() && gitutils.GetGHToken() != "" {
		retryReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
		retryReq.Header.Set("Authorization", "Bearer "+gitutils.GetGHToken())
		retryReq.Header.Set("Accept", "application/vnd.github.v3+json")
		if retryResp, retryErr := c.httpClient.Do(retryReq); retryErr == nil {
			defer retryResp.Body.Close()
			if retryResp.StatusCode >= 200 && retryResp.StatusCode < 300 {
				resp = retryResp
				token = gitutils.GetGHToken()
			}
		}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var comments []struct {
			ID   int64  `json:"id"`
			Body string `json:"body"`
		}
		if json.NewDecoder(resp.Body).Decode(&comments) == nil {
			for _, comment := range comments {
				if strings.Contains(comment.Body, "<!-- metastackr-root-pr-comment -->") {
					delURL := fmt.Sprintf("%s/repos/%s/issues/comments/%d", c.baseURL, metaRepo, comment.ID)
					delReq, _ := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil)
					delReq.Header.Set("Authorization", "Bearer "+token)
					delReq.Header.Set("Accept", "application/vnd.github.v3+json")
					if delResp, err := c.httpClient.Do(delReq); err == nil {
						delResp.Body.Close()
					}
				}
			}
		}
	}
	return nil
}

// EnsureRootPRComment posts or updates a single sticky comment on the root Meta PR with the latest matrix table.
func (c *GitHubClient) EnsureRootPRComment(ctx context.Context, metaRepo string, prNumber int, metaPR *db.MetaPR, installationID int64) error {
	if prNumber <= 0 || metaRepo == "" || metaPR == nil {
		return nil
	}

	token, err := c.GetInstallationTokenForRepo(ctx, metaRepo, installationID)
	if err != nil || token == "" {
		if c.token != "" {
			token = c.token
		} else {
			token = gitutils.GetGHToken()
		}
	}

	marker := "<!-- metastackr-root-pr-comment -->"
	_, _, tableMarkdown := GenerateMarkdownTable(metaPR)
	commentBody := fmt.Sprintf("%s\n%s", marker, tableMarkdown)

	listURL := fmt.Sprintf("%s/repos/%s/issues/%d/comments", c.baseURL, metaRepo, prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to list issue comments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden && token != gitutils.GetGHToken() && gitutils.GetGHToken() != "" {
		retryReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
		retryReq.Header.Set("Authorization", "Bearer "+gitutils.GetGHToken())
		retryReq.Header.Set("Accept", "application/vnd.github.v3+json")
		if retryResp, retryErr := c.httpClient.Do(retryReq); retryErr == nil {
			defer retryResp.Body.Close()
			if retryResp.StatusCode >= 200 && retryResp.StatusCode < 300 {
				resp = retryResp
				token = gitutils.GetGHToken()
			}
		}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var comments []struct {
			ID   int64  `json:"id"`
			Body string `json:"body"`
		}
		if json.NewDecoder(resp.Body).Decode(&comments) == nil {
			var firstCommentID int64 = 0
			for _, comment := range comments {
				if strings.Contains(comment.Body, marker) {
					if firstCommentID == 0 {
						firstCommentID = comment.ID
						if comment.Body != commentBody {
							patchURL := fmt.Sprintf("%s/repos/%s/issues/comments/%d", c.baseURL, metaRepo, comment.ID)
							patchBytes, _ := json.Marshal(map[string]string{"body": commentBody})
							patchReq, _ := http.NewRequestWithContext(ctx, http.MethodPatch, patchURL, bytes.NewReader(patchBytes))
							patchReq.Header.Set("Authorization", "Bearer "+token)
							patchReq.Header.Set("Accept", "application/vnd.github.v3+json")
							patchReq.Header.Set("Content-Type", "application/json")
							if patchResp, err := c.httpClient.Do(patchReq); err == nil {
								defer patchResp.Body.Close()
								if patchResp.StatusCode == http.StatusForbidden && token != gitutils.GetGHToken() && gitutils.GetGHToken() != "" {
									retryPatch, _ := http.NewRequestWithContext(ctx, http.MethodPatch, patchURL, bytes.NewReader(patchBytes))
									retryPatch.Header.Set("Authorization", "Bearer "+gitutils.GetGHToken())
									retryPatch.Header.Set("Accept", "application/vnd.github.v3+json")
									retryPatch.Header.Set("Content-Type", "application/json")
									if rResp, rErr := c.httpClient.Do(retryPatch); rErr == nil {
										rResp.Body.Close()
									}
								}
								log.Printf("[comments] Updated root Meta PR comment on %s#%d", metaRepo, prNumber)
							}
						}
					} else {
						// Delete duplicate comment
						delURL := fmt.Sprintf("%s/repos/%s/issues/comments/%d", c.baseURL, metaRepo, comment.ID)
						delReq, _ := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil)
						delReq.Header.Set("Authorization", "Bearer "+token)
						delReq.Header.Set("Accept", "application/vnd.github.v3+json")
						if delResp, err := c.httpClient.Do(delReq); err == nil {
							delResp.Body.Close()
						}
					}
				}
			}
			if firstCommentID > 0 {
				return nil
			}
		}
	}

	postPayload := map[string]string{"body": commentBody}
	bodyBytes, _ := json.Marshal(postPayload)

	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, listURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	postReq.Header.Set("Authorization", "Bearer "+token)
	postReq.Header.Set("Accept", "application/vnd.github+json")
	postReq.Header.Set("Content-Type", "application/json")

	postResp, err := c.httpClient.Do(postReq)
	if err != nil {
		return fmt.Errorf("failed to post comment: %w", err)
	}
	defer postResp.Body.Close()

	if postResp.StatusCode == http.StatusForbidden && token != gitutils.GetGHToken() && gitutils.GetGHToken() != "" {
		// Retry post with personal token if installation token is rate limited
		retryReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, listURL, bytes.NewReader(bodyBytes))
		retryReq.Header.Set("Authorization", "Bearer "+gitutils.GetGHToken())
		retryReq.Header.Set("Accept", "application/vnd.github+json")
		retryReq.Header.Set("Content-Type", "application/json")
		if retryResp, retryErr := c.httpClient.Do(retryReq); retryErr == nil {
			defer retryResp.Body.Close()
			if retryResp.StatusCode >= 200 && retryResp.StatusCode < 300 {
				log.Printf("[comments] Posted root Meta PR comment on %s#%d (via PAT fallback)", metaRepo, prNumber)
				return nil
			}
		}
	}

	if postResp.StatusCode >= 400 {
		body, _ := io.ReadAll(postResp.Body)
		return fmt.Errorf("failed to post root comment on %s#%d: HTTP %d: %s", metaRepo, prNumber, postResp.StatusCode, string(body))
	}

	log.Printf("[comments] Posted root Meta PR comment on %s#%d", metaRepo, prNumber)
	return nil
}

// HasApprovedReview checks if a pull request has at least one 'APPROVED' review on GitHub.
func (c *GitHubClient) HasApprovedReview(ctx context.Context, repoFullName string, prNumber int, installationID int64) (bool, error) {
	token, err := c.GetInstallationTokenForRepo(ctx, repoFullName, installationID)
	if err != nil || token == "" {
		if c.token != "" {
			token = c.token
		} else {
			token = gitutils.GetGHToken()
		}
	}

	url := fmt.Sprintf("%s/repos/%s/pulls/%d/reviews", c.baseURL, repoFullName, prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to fetch PR reviews: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("GitHub API returned HTTP %d for reviews on %s#%d: %s", resp.StatusCode, repoFullName, prNumber, string(body))
	}

	var reviews []struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reviews); err != nil {
		return false, err
	}

	for _, rev := range reviews {
		if strings.ToUpper(rev.State) == "APPROVED" {
			return true, nil
		}
	}

	return false, nil
}

// AreRequiredChecksPassing checks if all required status check contexts have passed on a commit SHA.
func (c *GitHubClient) AreRequiredChecksPassing(ctx context.Context, repoFullName string, headSHA string, requiredChecks []string, installationID int64) (bool, []string, error) {
	if len(requiredChecks) == 0 {
		return true, nil, nil
	}

	token, err := c.GetInstallationTokenForRepo(ctx, repoFullName, installationID)
	if err != nil || token == "" {
		if c.token != "" {
			token = c.token
		} else {
			token = gitutils.GetGHToken()
		}
	}

	// 1. Fetch Check Runs
	url := fmt.Sprintf("%s/repos/%s/commits/%s/check-runs", c.baseURL, repoFullName, headSHA)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, requiredChecks, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	passedChecks := make(map[string]bool)

	if resp, err := c.httpClient.Do(req); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var checkRunsResp struct {
				CheckRuns []struct {
					Name       string  `json:"name"`
					Status     string  `json:"status"`
					Conclusion *string `json:"conclusion"`
				} `json:"check_runs"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&checkRunsResp); err == nil {
				for _, run := range checkRunsResp.CheckRuns {
					if run.Status == "completed" && run.Conclusion != nil && *run.Conclusion == "success" {
						passedChecks[strings.ToLower(run.Name)] = true
					}
				}
			}
		}
	}

	// 2. Fetch Commit Statuses (legacy status API)
	statusURL := fmt.Sprintf("%s/repos/%s/commits/%s/status", c.baseURL, repoFullName, headSHA)
	if reqStatus, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil); err == nil {
		if token != "" {
			reqStatus.Header.Set("Authorization", "Bearer "+token)
		}
		reqStatus.Header.Set("Accept", "application/vnd.github+json")
		if respStatus, err := c.httpClient.Do(reqStatus); err == nil {
			defer respStatus.Body.Close()
			if respStatus.StatusCode >= 200 && respStatus.StatusCode < 300 {
				var statusResp struct {
					Statuses []struct {
						Context string `json:"context"`
						State   string `json:"state"`
					} `json:"statuses"`
				}
				if err := json.NewDecoder(respStatus.Body).Decode(&statusResp); err == nil {
					for _, st := range statusResp.Statuses {
						if st.State == "success" {
							passedChecks[strings.ToLower(st.Context)] = true
						}
					}
				}
			}
		}
	}

	var pendingOrFailed []string
	for _, reqCheck := range requiredChecks {
		if !passedChecks[strings.ToLower(reqCheck)] {
			pendingOrFailed = append(pendingOrFailed, reqCheck)
		}
	}

	if len(pendingOrFailed) > 0 {
		return false, pendingOrFailed, nil
	}

	return true, nil, nil
}

// HasNonSubmoduleFilesChanged queries GitHub API to check if any files modified in the pull request lie outside submodule directories or .gitmodules.
func (c *GitHubClient) HasNonSubmoduleFilesChanged(ctx context.Context, repoFullName string, prNumber int, installationID int64) (bool, error) {
	if repoFullName == "" || prNumber <= 0 {
		return false, nil
	}

	token, err := c.GetInstallationTokenForRepo(ctx, repoFullName, installationID)
	if err != nil || token == "" {
		if c.token != "" {
			token = c.token
		} else {
			token = gitutils.GetGHToken()
		}
	}

	url := fmt.Sprintf("%s/repos/%s/pulls/%d/files", c.baseURL, repoFullName, prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.doRequestWithPATFallback(ctx, req, token, nil)
	if err != nil {
		return false, fmt.Errorf("failed to fetch pull request files: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("GitHub API returned HTTP %d for pull files: %s", resp.StatusCode, string(body))
	}

	var files []struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return false, err
	}

	for _, f := range files {
		fn := strings.TrimSpace(f.Filename)
		if fn == "" || fn == ".gitmodules" {
			continue
		}
		// If file path is not within submodules directory (services/) and not a known submodule path
		if !strings.HasPrefix(fn, "services/") && !strings.HasPrefix(fn, "submodules/") {
			return true, nil
		}
	}

	return false, nil
}

func (c *GitHubClient) GetBranchHeadSHA(ctx context.Context, repoFullName string, branchName string, installationID int64) (string, error) {
	token, err := c.GetInstallationTokenForRepo(ctx, repoFullName, installationID)
	if err != nil || token == "" {
		if c.token != "" {
			token = c.token
		} else {
			token = gitutils.GetGHToken()
		}
	}

	url := fmt.Sprintf("%s/repos/%s/git/ref/heads/%s", c.baseURL, repoFullName, branchName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d for ref %s", resp.StatusCode, branchName)
	}

	var refResp struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&refResp); err != nil {
		return "", err
	}

	return refResp.Object.SHA, nil
}

func (c *GitHubClient) doRequestWithPATFallback(ctx context.Context, req *http.Request, token string, bodyBytes []byte) (*http.Response, error) {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	pat := gitutils.GetGHToken()
	if (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) && pat != "" && token != pat {
		resp.Body.Close()
		var reader io.Reader
		if len(bodyBytes) > 0 {
			reader = bytes.NewReader(bodyBytes)
		}
		retryReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL.String(), reader)
		if err == nil {
			for k, v := range req.Header {
				retryReq.Header[k] = v
			}
			retryReq.Header.Set("Authorization", "Bearer "+pat)
			if rResp, rErr := c.httpClient.Do(retryReq); rErr == nil {
				return rResp, nil
			}
		}
	}
	return resp, nil
}

func ParseGitmodules(content string) map[string]string {
	res := make(map[string]string)
	lines := strings.Split(content, "\n")
	var currentPath string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "path =") {
			currentPath = strings.TrimSpace(strings.TrimPrefix(line, "path ="))
		} else if strings.HasPrefix(line, "url =") && currentPath != "" {
			rawURL := strings.TrimSpace(strings.TrimPrefix(line, "url ="))
			rawURL = strings.TrimSuffix(rawURL, ".git")
			parts := strings.Split(rawURL, "/")
			if len(parts) >= 2 {
				repoFullName := fmt.Sprintf("%s/%s", parts[len(parts)-2], parts[len(parts)-1])
				if idx := strings.Index(repoFullName, ":"); idx != -1 {
					repoFullName = repoFullName[idx+1:]
				}
				res[strings.ToLower(repoFullName)] = currentPath
				res[strings.ToLower(currentPath)] = currentPath
			}
			currentPath = ""
		}
	}
	return res
}

func (c *GitHubClient) UpdateSubmodulePointersOnBranch(ctx context.Context, repoFullName string, branchName string, updates []SubmodulePointerUpdate, instID int64) error {
	if len(updates) == 0 {
		return nil
	}

	token, err := c.GetInstallationTokenForRepo(ctx, repoFullName, instID)
	if err != nil || token == "" {
		token = c.token
	}

	// 0. Fetch and parse .gitmodules from parent repo to resolve relative submodule paths
	gitmodulesURL := fmt.Sprintf("%s/repos/%s/contents/.gitmodules?ref=%s", c.baseURL, repoFullName, branchName)
	reqGitmodules, _ := http.NewRequestWithContext(ctx, http.MethodGet, gitmodulesURL, nil)
	reqGitmodules.Header.Set("Accept", "application/vnd.github.v3.raw")
	pathMap := make(map[string]string)
	if respGitmodules, err := c.doRequestWithPATFallback(ctx, reqGitmodules, token, nil); err == nil {
		if respGitmodules.StatusCode == http.StatusOK {
			if bodyBytes, err := io.ReadAll(respGitmodules.Body); err == nil {
				pathMap = ParseGitmodules(string(bodyBytes))
			}
		}
		respGitmodules.Body.Close()
	}

	// 1. Get branch HEAD commit
	refURL := fmt.Sprintf("%s/repos/%s/git/ref/heads/%s", c.baseURL, repoFullName, branchName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, refURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.doRequestWithPATFallback(ctx, req, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to fetch branch ref %s: %s", branchName, string(body))
	}

	var refResp struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&refResp); err != nil {
		return err
	}
	baseCommitSHA := refResp.Object.SHA

	// 2. Get base commit to extract tree SHA
	commitURL := fmt.Sprintf("%s/repos/%s/git/commits/%s", c.baseURL, repoFullName, baseCommitSHA)
	reqCommit, _ := http.NewRequestWithContext(ctx, http.MethodGet, commitURL, nil)
	reqCommit.Header.Set("Accept", "application/vnd.github+json")
	respCommit, err := c.doRequestWithPATFallback(ctx, reqCommit, token, nil)
	if err != nil {
		return err
	}
	defer respCommit.Body.Close()

	var commitObj struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(respCommit.Body).Decode(&commitObj); err != nil {
		return err
	}
	baseTreeSHA := commitObj.Tree.SHA

	// 3. Post new tree with updated submodule commit pointers
	type treeItem struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
		Type string `json:"type"`
		SHA  string `json:"sha"`
	}
	var treeItems []treeItem
	for _, up := range updates {
		shaToUse := up.NewCommitSHA
		subRepoName := up.SubmoduleRepo
		if subRepoName == "" {
			subRepoName = up.SubmodulePath
		}
		if mainSHA, err := c.GetBranchHeadSHA(ctx, subRepoName, "main", instID); err == nil && mainSHA != "" {
			shaToUse = mainSHA
		}
		treePath := up.SubmodulePath
		if mappedPath, ok := pathMap[strings.ToLower(up.SubmodulePath)]; ok {
			treePath = mappedPath
		}
		if shaToUse != "" && treePath != "" {
			treeItems = append(treeItems, treeItem{
				Path: treePath,
				Mode: "160000",
				Type: "commit",
				SHA:  shaToUse,
			})
		}
	}

	treeReqBody, _ := json.Marshal(map[string]interface{}{
		"base_tree": baseTreeSHA,
		"tree":      treeItems,
	})

	postTreeURL := fmt.Sprintf("%s/repos/%s/git/trees", c.baseURL, repoFullName)
	reqPostTree, _ := http.NewRequestWithContext(ctx, http.MethodPost, postTreeURL, bytes.NewReader(treeReqBody))
	reqPostTree.Header.Set("Accept", "application/vnd.github+json")
	reqPostTree.Header.Set("Content-Type", "application/json")

	respPostTree, err := c.doRequestWithPATFallback(ctx, reqPostTree, token, treeReqBody)
	if err != nil {
		return err
	}
	defer respPostTree.Body.Close()

	var postTreeResp struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(respPostTree.Body).Decode(&postTreeResp); err != nil {
		return err
	}
	newTreeSHA := postTreeResp.SHA

	// 4. Create new git commit with updated tree
	newCommitBody, _ := json.Marshal(map[string]interface{}{
		"message": "chore: sync submodule commit pointers [metastackr]",
		"tree":    newTreeSHA,
		"parents": []string{baseCommitSHA},
	})

	postCommitURL := fmt.Sprintf("%s/repos/%s/git/commits", c.baseURL, repoFullName)
	reqPostCommit, _ := http.NewRequestWithContext(ctx, http.MethodPost, postCommitURL, bytes.NewReader(newCommitBody))
	reqPostCommit.Header.Set("Accept", "application/vnd.github+json")
	reqPostCommit.Header.Set("Content-Type", "application/json")

	respPostCommit, err := c.doRequestWithPATFallback(ctx, reqPostCommit, token, newCommitBody)
	if err != nil {
		return err
	}
	defer respPostCommit.Body.Close()

	var postCommitResp struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(respPostCommit.Body).Decode(&postCommitResp); err != nil {
		return err
	}
	newCommitSHA := postCommitResp.SHA

	// 5. Update branch ref to point to new commit
	updateRefBody, _ := json.Marshal(map[string]interface{}{
		"sha":   newCommitSHA,
		"force": false,
	})

	patchRefURL := fmt.Sprintf("%s/repos/%s/git/refs/heads/%s", c.baseURL, repoFullName, branchName)
	reqPatchRef, _ := http.NewRequestWithContext(ctx, http.MethodPatch, patchRefURL, bytes.NewReader(updateRefBody))
	reqPatchRef.Header.Set("Accept", "application/vnd.github+json")
	reqPatchRef.Header.Set("Content-Type", "application/json")

	respPatchRef, err := c.doRequestWithPATFallback(ctx, reqPatchRef, token, updateRefBody)
	if err != nil {
		return err
	}
	defer respPatchRef.Body.Close()

	if respPatchRef.StatusCode < 200 || respPatchRef.StatusCode >= 300 {
		body, _ := io.ReadAll(respPatchRef.Body)
		return fmt.Errorf("failed to update branch ref %s: %s", branchName, string(body))
	}

	return nil
}
