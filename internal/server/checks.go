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
)

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
	block, _ := pem.Decode(pemBytes)
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

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", c.baseURL, installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwtStr)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
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
	title = fmt.Sprintf("Meta-Repo Sync Status: %s", metaPR.Status)
	summary = fmt.Sprintf("Meta PR #%d (%s) - Submodule PR Matrix", metaPR.PRNumber, metaPR.BranchName)

	var sb strings.Builder
	sb.WriteString("### 🔄 Submodule Synchronization Matrix\n\n")
	sb.WriteString("| Submodule Path | Child Repo | PR # | Head SHA | Status |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")

	if len(metaPR.ChildPRs) == 0 {
		sb.WriteString("| *No child PRs tracked* | - | - | - | - |\n")
	} else {
		for _, child := range metaPR.ChildPRs {
			prLink := fmt.Sprintf("#%d", child.PRNumber)
			if child.PRNumber > 0 {
				prLink = fmt.Sprintf("[%d](https://github.com/%s/pull/%d)", child.PRNumber, child.RepoFullName, child.PRNumber)
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
	if installationID > 0 {
		return c.GetInstallationToken(ctx, installationID)
	}

	if c.privateKey == nil {
		return c.token, nil
	}

	jwtStr, err := generateJWT(c.appID, c.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate GitHub App JWT: %w", err)
	}

	url := fmt.Sprintf("%s/repos/%s/installation", c.baseURL, repoFullName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwtStr)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to query repo installation: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("repo installation API returned HTTP %d for %s: %s", resp.StatusCode, repoFullName, string(body))
	}

	var instResp struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&instResp); err != nil {
		return "", err
	}

	return c.GetInstallationToken(ctx, instResp.ID)
}

// UpdateMetaCheckRun creates or updates the single GitHub Check Run named 'meta-repo/sync'.
func (c *GitHubClient) UpdateMetaCheckRun(ctx context.Context, metaRepo string, headSHA string, metaPR *db.MetaPR, installationID int64) error {
	token, err := c.GetInstallationTokenForRepo(ctx, metaRepo, installationID)
	if err != nil {
		return fmt.Errorf("failed to acquire token for check run: %w", err)
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

// EnsureChildPRComment posts a standard comment on a child PR referencing its parent Meta PR if not already posted.
func (c *GitHubClient) EnsureChildPRComment(ctx context.Context, childRepo string, prNumber int, parentMetaRepo string, parentPRNumber int, branchName string, installationID int64) error {
	if prNumber <= 0 || childRepo == "" || parentMetaRepo == "" || parentPRNumber <= 0 {
		return nil
	}

	token, err := c.GetInstallationTokenForRepo(ctx, childRepo, installationID)
	if err != nil {
		return fmt.Errorf("failed to acquire token for PR comment: %w", err)
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

// EnsureRootPRComment posts or updates a single sticky comment on the root Meta PR with the latest matrix table.
func (c *GitHubClient) EnsureRootPRComment(ctx context.Context, metaRepo string, prNumber int, metaPR *db.MetaPR, installationID int64) error {
	if prNumber <= 0 || metaRepo == "" || metaPR == nil {
		return nil
	}

	token, err := c.GetInstallationTokenForRepo(ctx, metaRepo, installationID)
	if err != nil {
		return fmt.Errorf("failed to acquire token for root PR comment: %w", err)
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
					if comment.Body == commentBody {
						return nil
					}
					patchURL := fmt.Sprintf("%s/repos/%s/issues/comments/%d", c.baseURL, metaRepo, comment.ID)
					patchBytes, _ := json.Marshal(map[string]string{"body": commentBody})
					patchReq, _ := http.NewRequestWithContext(ctx, http.MethodPatch, patchURL, bytes.NewReader(patchBytes))
					patchReq.Header.Set("Authorization", "Bearer "+token)
					patchReq.Header.Set("Accept", "application/vnd.github+json")
					patchReq.Header.Set("Content-Type", "application/json")
					if patchResp, err := c.httpClient.Do(patchReq); err == nil {
						patchResp.Body.Close()
						log.Printf("[comments] Updated root Meta PR comment on %s#%d", metaRepo, prNumber)
					}
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
		return fmt.Errorf("failed to post root PR comment: %w", err)
	}
	defer postResp.Body.Close()

	log.Printf("[comments] Posted root Meta PR comment on %s#%d", metaRepo, prNumber)
	return nil
}

