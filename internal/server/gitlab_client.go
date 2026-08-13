package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"metastackr/internal/db"
	"metastackr/internal/vcs"
)

type GitLabClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	repo       *db.Repository
}

func NewGitLabClient(baseURL string, token string) *GitLabClient {
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	if token == "" {
		token = os.Getenv("GITLAB_TOKEN")
	}
	return &GitLabClient{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *GitLabClient) SetRepository(repo *db.Repository) {
	c.repo = repo
}

func (c *GitLabClient) projectIDOrPath(repoFullName string) string {
	return url.PathEscape(repoFullName)
}

func (c *GitLabClient) setAuthHeader(req *http.Request) {
	if c.token == "" {
		return
	}
	if strings.HasPrefix(c.token, "glpat-") {
		req.Header.Set("PRIVATE-TOKEN", c.token)
	} else {
		tokenVal := c.token
		if !strings.HasPrefix(tokenVal, "Bearer ") {
			tokenVal = "Bearer " + tokenVal
		}
		req.Header.Set("Authorization", tokenVal)
	}
}

func RefreshGitLabToken(ctx context.Context, clientID, clientSecret, refreshToken string) (string, string, error) {
	if clientID == "" || clientSecret == "" || refreshToken == "" {
		return "", "", fmt.Errorf("missing clientID, clientSecret, or refreshToken for OAuth token refresh")
	}
	tokenURL := "https://gitlab.com/oauth/token"
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("refresh_token", refreshToken)
	form.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("token refresh failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", err
	}
	return data.AccessToken, data.RefreshToken, nil
}

func (c *GitLabClient) GetPRHeadSHA(ctx context.Context, repoFullName string, prNumber int, installationID int64) (string, error) {
	if repoFullName == "" || prNumber <= 0 {
		return "", fmt.Errorf("invalid repository or MR number")
	}
	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d", c.baseURL, c.projectIDOrPath(repoFullName), prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitLab API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var mr struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return "", err
	}
	return mr.SHA, nil
}

func (c *GitLabClient) GetBranchHeadSHA(ctx context.Context, repoFullName string, branchName string, installationID int64) (string, error) {
	if repoFullName == "" || branchName == "" {
		return "", fmt.Errorf("invalid repository or branch name")
	}
	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches/%s", c.baseURL, c.projectIDOrPath(repoFullName), url.PathEscape(branchName))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitLab API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var b struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		return "", err
	}
	return b.Commit.ID, nil
}

func (c *GitLabClient) GetOpenPRForBranch(ctx context.Context, repoFullName string, branchName string, installationID int64) (int, string, bool, error) {
	if repoFullName == "" || branchName == "" {
		return 0, "", false, nil
	}

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests?source_branch=%s&state=all", c.baseURL, c.projectIDOrPath(repoFullName), url.QueryEscape(branchName))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, "", false, err
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return 0, "", false, fmt.Errorf("GitLab API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var mrs []struct {
		IID         int    `json:"iid"`
		State       string `json:"state"`
		SHA         string `json:"sha"`
		DiffHeadSHA string `json:"diff_head_sha"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&mrs); err != nil {
		return 0, "", false, err
	}

	if len(mrs) > 0 {
		sha := mrs[0].SHA
		if sha == "" {
			sha = mrs[0].DiffHeadSHA
		}
		merged := strings.EqualFold(mrs[0].State, "merged")
		return mrs[0].IID, sha, merged, nil
	}

	return 0, "", false, nil
}

func (c *GitLabClient) UpdateMetaCheckRun(ctx context.Context, repoFullName string, headSHA string, metaPR *db.MetaPR, installationID int64) error {
	if repoFullName == "" || headSHA == "" {
		return nil
	}

	state := "pending"
	description := "Evaluating submodule dependencies..."
	if metaPR != nil {
		switch metaPR.Status {
		case "MERGED":
			state = "success"
			description = "All submodule MRs merged cleanly"
		case "FAILED_DRIFT", "FAILED_PARTIAL":
			state = "failed"
			description = "Submodule synchronization failed"
		default:
			mergedCount := 0
			openCount := 0
			for _, child := range metaPR.ChildPRs {
				if child.Status == "MERGED" {
					mergedCount++
				} else {
					openCount++
				}
			}
			if mergedCount == len(metaPR.ChildPRs) && len(metaPR.ChildPRs) > 0 {
				state = "success"
				description = fmt.Sprintf("All %d submodule MR(s) merged cleanly", len(metaPR.ChildPRs))
			} else {
				state = "pending"
				description = fmt.Sprintf("Submodules: %d/%d merged, %d open", mergedCount, len(metaPR.ChildPRs), openCount)
			}
		}
	}

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/statuses/%s", c.baseURL, c.projectIDOrPath(repoFullName), headSHA)
	params := url.Values{}
	params.Set("state", state)
	params.Set("name", "meta-repo/sync")
	params.Set("description", description)
	if metaPR != nil && metaPR.PRNumber > 0 {
		params.Set("target_url", fmt.Sprintf("https://gitlab.com/%s/-/merge_requests/%d", repoFullName, metaPR.PRNumber))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *GitLabClient) EnsureRootPRComment(ctx context.Context, repoFullName string, prNumber int, metaPR *db.MetaPR, installationID int64) error {
	if repoFullName == "" || prNumber <= 0 {
		return nil
	}

	marker := "<!-- metastackr-root-pr-comment -->"
	_, _, tableMarkdown := GenerateMarkdownTable(metaPR)
	body := fmt.Sprintf("%s\n%s", marker, tableMarkdown)

	// 1. Fetch existing notes on the MR in chronological order (earliest first)
	listURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes?sort=asc&order_by=created_at&per_page=100", c.baseURL, c.projectIDOrPath(repoFullName), prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return err
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var existingNoteID int64
	var existingBody string
	var duplicateNoteIDs []int64

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var notes []struct {
			ID   int64  `json:"id"`
			Body string `json:"body"`
		}
		if json.NewDecoder(resp.Body).Decode(&notes) == nil {
			for _, n := range notes {
				if strings.Contains(n.Body, marker) || strings.Contains(n.Body, "MetaStackr") || strings.Contains(n.Body, "metastackr") || strings.Contains(n.Body, "PR Status Matrix") || strings.Contains(n.Body, "Submodule Synchronization") || strings.Contains(n.Body, "Submodule Merge Request Matrix") || strings.Contains(n.Body, "Meta-Repo Sync Status") {
					if existingNoteID == 0 {
						// Keep the initial comment to overwrite
						existingNoteID = n.ID
						existingBody = n.Body
					} else {
						// Collect any subsequent duplicate comments for cleanup
						duplicateNoteIDs = append(duplicateNoteIDs, n.ID)
					}
				}
			}
		}
	}

	// 2. Delete any redundant duplicate notes so only the single initial comment remains
	for _, dupID := range duplicateNoteIDs {
		delURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes/%d", c.baseURL, c.projectIDOrPath(repoFullName), prNumber, dupID)
		if delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil); err == nil {
			c.setAuthHeader(delReq)
			if dResp, err := c.httpClient.Do(delReq); err == nil {
				dResp.Body.Close()
			}
		}
	}

	// 3. If existing note matches body exactly, do nothing
	if existingNoteID > 0 && strings.TrimSpace(existingBody) == strings.TrimSpace(body) {
		return nil
	}

	payload := map[string]string{"body": body}
	jsonBytes, _ := json.Marshal(payload)

	var updateReq *http.Request
	if existingNoteID > 0 {
		// Overwrite the initial note in-place via PUT
		putURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes/%d", c.baseURL, c.projectIDOrPath(repoFullName), prNumber, existingNoteID)
		updateReq, _ = http.NewRequestWithContext(ctx, http.MethodPut, putURL, bytes.NewReader(jsonBytes))
	} else {
		// Create the initial note via POST
		postURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes", c.baseURL, c.projectIDOrPath(repoFullName), prNumber)
		updateReq, _ = http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(jsonBytes))
	}

	if updateReq != nil {
		updateReq.Header.Set("Content-Type", "application/json")
		c.setAuthHeader(updateReq)
		if uResp, err := c.httpClient.Do(updateReq); err == nil {
			uResp.Body.Close()
		}
	}

	return nil
}

func (c *GitLabClient) EnsureRootPRDescriptionBody(ctx context.Context, repoFullName string, prNumber int, metaPR *db.MetaPR, installationID int64) error {
	return nil
}

func (c *GitLabClient) EnsureChildPRComment(ctx context.Context, childRepo string, prNumber int, parentMetaRepo string, parentPRNumber int, branchName string, installationID int64) error {
	if childRepo == "" || prNumber <= 0 {
		return nil
	}

	marker := "<!-- metastackr-child-pr-comment -->"
	body := fmt.Sprintf("%s\n🔗 **MetaStackr Child MR**: Managed by Parent Meta MR [%s!%d](https://gitlab.com/%s/-/merge_requests/%d)", marker, parentMetaRepo, parentPRNumber, parentMetaRepo, parentPRNumber)

	// Fetch notes to check for initial comment vs duplicates
	listURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes?sort=asc&order_by=created_at&per_page=100", c.baseURL, c.projectIDOrPath(childRepo), prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err == nil {
		c.setAuthHeader(req)
		if resp, err := c.httpClient.Do(req); err == nil {
			defer resp.Body.Close()
			var notes []struct {
				ID   int64  `json:"id"`
				Body string `json:"body"`
			}
			var existingNoteID int64
			var existingBody string
			var duplicateNoteIDs []int64

			if json.NewDecoder(resp.Body).Decode(&notes) == nil {
				for _, n := range notes {
					if strings.Contains(n.Body, marker) || strings.Contains(n.Body, "MetaStackr Child PR") || strings.Contains(n.Body, "MetaStackr Child MR") {
						if existingNoteID == 0 {
							existingNoteID = n.ID
							existingBody = n.Body
						} else {
							duplicateNoteIDs = append(duplicateNoteIDs, n.ID)
						}
					}
				}
			}

			// Clean up duplicate notes
			for _, dupID := range duplicateNoteIDs {
				delURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes/%d", c.baseURL, c.projectIDOrPath(childRepo), prNumber, dupID)
				if delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil); err == nil {
					c.setAuthHeader(delReq)
					if dResp, err := c.httpClient.Do(delReq); err == nil {
						dResp.Body.Close()
					}
				}
			}

			if existingNoteID > 0 {
				if strings.TrimSpace(existingBody) == strings.TrimSpace(body) {
					return nil
				}
				// Overwrite initial note in-place
				putURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes/%d", c.baseURL, c.projectIDOrPath(childRepo), prNumber, existingNoteID)
				payload := map[string]string{"body": body}
				jsonBytes, _ := json.Marshal(payload)
				if putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, bytes.NewReader(jsonBytes)); err == nil {
					putReq.Header.Set("Content-Type", "application/json")
					c.setAuthHeader(putReq)
					if pResp, err := c.httpClient.Do(putReq); err == nil {
						pResp.Body.Close()
					}
				}
				return nil
			}
		}
	}

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes", c.baseURL, c.projectIDOrPath(childRepo), prNumber)
	payload := map[string]string{"body": body}
	jsonBytes, _ := json.Marshal(payload)

	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	postReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(postReq)

	resp, err := c.httpClient.Do(postReq)
	if err == nil {
		resp.Body.Close()
	}
	return nil
}

func (c *GitLabClient) UpdateSubmodulePointersOnBranch(ctx context.Context, repoFullName string, branchName string, updates []vcs.SubmodulePointerUpdate, installationID int64) error {
	if len(updates) == 0 {
		return nil
	}

	// 0. Fetch and parse .gitmodules from parent repo to resolve relative submodule paths
	gitmodulesURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/.gitmodules/raw?ref=%s", c.baseURL, c.projectIDOrPath(repoFullName), url.QueryEscape(branchName))
	pathMap := make(map[string]string)
	reqGitmodules, err := http.NewRequestWithContext(ctx, http.MethodGet, gitmodulesURL, nil)
	if err == nil {
		c.setAuthHeader(reqGitmodules)
		if respGitmodules, err := c.httpClient.Do(reqGitmodules); err == nil {
			if respGitmodules.StatusCode == http.StatusOK {
				if bodyBytes, err := io.ReadAll(respGitmodules.Body); err == nil {
					pathMap = ParseGitmodules(string(bodyBytes))
				}
			}
			respGitmodules.Body.Close()
		}
	}

	for _, up := range updates {
		shaToUse := up.NewCommitSHA
		subRepoName := up.SubmoduleRepo
		if subRepoName == "" {
			subRepoName = up.SubmodulePath
		}
		if mainSHA, err := c.GetBranchHeadSHA(ctx, subRepoName, "main", installationID); err == nil && mainSHA != "" {
			shaToUse = mainSHA
		}

		filePath := up.SubmodulePath
		if realPath, ok := pathMap[strings.ToLower(up.SubmoduleRepo)]; ok && realPath != "" {
			filePath = realPath
		} else if realPath, ok := pathMap[strings.ToLower(up.SubmodulePath)]; ok && realPath != "" {
			filePath = realPath
		}

		if shaToUse == "" || filePath == "" {
			continue
		}

		payload := map[string]interface{}{
			"branch":         branchName,
			"commit_sha":     shaToUse,
			"commit_message": fmt.Sprintf("chore(meta): align %s submodule pointer to HEAD", filePath),
		}
		jsonBytes, _ := json.Marshal(payload)

		reqURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/submodules/%s", c.baseURL, c.projectIDOrPath(repoFullName), url.QueryEscape(filePath))
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(jsonBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		c.setAuthHeader(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("failed to update submodule pointer for %s: HTTP %d %s", filePath, resp.StatusCode, string(body))
		}
		resp.Body.Close()
	}

	return nil
}

func (c *GitLabClient) MergePullRequest(ctx context.Context, repoFullName string, prNumber int, mergeMethod string, installationID int64) (string, error) {
	if repoFullName == "" || prNumber <= 0 {
		return "", fmt.Errorf("invalid repository or MR number")
	}

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/merge", c.baseURL, c.projectIDOrPath(repoFullName), prNumber)
	payload := map[string]interface{}{
		"should_remove_source_branch": false,
	}
	if mergeMethod == "squash" {
		payload["squash"] = true
	}
	jsonBytes, _ := json.Marshal(payload)

	var resp *http.Response
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*500) * time.Millisecond)
		}
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(jsonBytes))
		if reqErr != nil {
			return "", reqErr
		}
		req.Header.Set("Content-Type", "application/json")
		c.setAuthHeader(req)

		resp, err = c.httpClient.Do(req)
		if err != nil {
			return "", err
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			break
		}
		if (resp.StatusCode == http.StatusConflict || resp.StatusCode == 405 || resp.StatusCode == 422) && attempt < 4 {
			resp.Body.Close()
			continue
		}
		break
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitLab API returned HTTP %d for merge: %s", resp.StatusCode, string(body))
	}

	var res struct {
		MergeCommitSHA string `json:"merge_commit_sha"`
		SquashCommitSHA string `json:"squash_commit_sha"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	if res.MergeCommitSHA != "" {
		return res.MergeCommitSHA, nil
	}
	return res.SquashCommitSHA, nil
}

func (c *GitLabClient) HasApprovedReview(ctx context.Context, repoFullName string, prNumber int, installationID int64) (bool, error) {
	if repoFullName == "" || prNumber <= 0 {
		return true, nil
	}

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/approvals", c.baseURL, c.projectIDOrPath(repoFullName), prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, err
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, nil
	}

	var app struct {
		Approved bool `json:"approved"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return false, err
	}
	return app.Approved, nil
}

func (c *GitLabClient) AreRequiredChecksPassing(ctx context.Context, repoFullName string, headSHA string, requiredChecks []string, installationID int64) (bool, []string, error) {
	if repoFullName == "" || headSHA == "" || len(requiredChecks) == 0 {
		return true, nil, nil
	}

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/commits/%s/statuses", c.baseURL, c.projectIDOrPath(repoFullName), headSHA)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, requiredChecks, err
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, requiredChecks, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, requiredChecks, nil
	}

	var statuses []struct {
		Name  string `json:"name"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return false, requiredChecks, err
	}

	passedMap := make(map[string]bool)
	for _, s := range statuses {
		if strings.ToLower(s.Status) == "success" {
			passedMap[strings.ToLower(s.Name)] = true
		}
	}

	var missing []string
	for _, reqName := range requiredChecks {
		if !passedMap[strings.ToLower(reqName)] {
			missing = append(missing, reqName)
		}
	}

	return len(missing) == 0, missing, nil
}

func (c *GitLabClient) HasNonSubmoduleFilesChanged(ctx context.Context, repoFullName string, prNumber int, installationID int64) (bool, error) {
	if repoFullName == "" || prNumber <= 0 {
		return false, nil
	}

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/changes", c.baseURL, c.projectIDOrPath(repoFullName), prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, err
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, nil
	}

	var changesPayload struct {
		Changes []struct {
			OldPath string `json:"old_path"`
			NewPath string `json:"new_path"`
		} `json:"changes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&changesPayload); err != nil {
		return false, err
	}

	// 2. Fetch .gitmodules to dynamically resolve all submodule paths
	pathMap := make(map[string]string)
	gitmodulesURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/.gitmodules/raw", c.baseURL, c.projectIDOrPath(repoFullName))
	reqGitmodules, err := http.NewRequestWithContext(ctx, http.MethodGet, gitmodulesURL, nil)
	if err == nil {
		c.setAuthHeader(reqGitmodules)
		if respGitmodules, err := c.httpClient.Do(reqGitmodules); err == nil {
			if respGitmodules.StatusCode == http.StatusOK {
				if bodyBytes, err := io.ReadAll(respGitmodules.Body); err == nil {
					pathMap = ParseGitmodules(string(bodyBytes))
				}
			}
			respGitmodules.Body.Close()
		}
	}

	for _, ch := range changesPayload.Changes {
		fn := strings.TrimSpace(ch.NewPath)
		if !IsSubmodulePath(fn, pathMap) {
			return true, nil
		}
	}

	return false, nil
}
