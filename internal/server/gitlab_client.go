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
			description = "Submodule cascade merge completed"
		case "FAILED_DRIFT", "FAILED_PARTIAL":
			state = "failed"
			description = "Submodule synchronization failed"
		default:
			allChildrenMerged := true
			for _, child := range metaPR.ChildPRs {
				if child.Status != "MERGED" {
					allChildrenMerged = false
					break
				}
			}
			if allChildrenMerged && len(metaPR.ChildPRs) > 0 {
				state = "success"
				description = "All submodule PRs merged cleanly"
			}
		}
	}

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/statuses/%s", c.baseURL, c.projectIDOrPath(repoFullName), headSHA)
	params := url.Values{}
	params.Set("state", state)
	params.Set("name", "meta-repo/sync")
	params.Set("description", description)

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

	body, _, _ := GenerateMarkdownTable(metaPR)
	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes", c.baseURL, c.projectIDOrPath(repoFullName), prNumber)
	payload := map[string]string{"body": body}
	jsonBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_ = c.EnsureRootPRDescriptionBody(ctx, repoFullName, prNumber, metaPR, installationID)
	return nil
}

func (c *GitLabClient) EnsureRootPRDescriptionBody(ctx context.Context, repoFullName string, prNumber int, metaPR *db.MetaPR, installationID int64) error {
	if repoFullName == "" || prNumber <= 0 {
		return nil
	}

	tableMarkdown, _, _ := GenerateMarkdownTable(metaPR)

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d", c.baseURL, c.projectIDOrPath(repoFullName), prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}

	var mr struct {
		Description string `json:"description"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&mr)

	currentDesc := mr.Description
	newDesc := currentDesc
	startIdx := strings.Index(currentDesc, "### 🔄 Submodule Synchronization")
	if startIdx != -1 {
		endIdx := strings.Index(currentDesc[startIdx:], "\n---\n")
		if endIdx != -1 {
			newDesc = currentDesc[:startIdx] + tableMarkdown + currentDesc[startIdx+endIdx+5:]
		} else {
			newDesc = currentDesc[:startIdx] + tableMarkdown
		}
	} else {
		if currentDesc != "" {
			newDesc = currentDesc + "\n\n" + tableMarkdown
		} else {
			newDesc = tableMarkdown
		}
	}

	if newDesc == currentDesc {
		return nil
	}

	putURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d", c.baseURL, c.projectIDOrPath(repoFullName), prNumber)
	payload := map[string]string{"description": newDesc}
	jsonBytes, _ := json.Marshal(payload)

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	putReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(putReq)

	putResp, err := c.httpClient.Do(putReq)
	if err == nil {
		putResp.Body.Close()
	}
	return nil
}

func (c *GitLabClient) EnsureChildPRComment(ctx context.Context, childRepo string, prNumber int, parentMetaRepo string, parentPRNumber int, branchName string, installationID int64) error {
	if childRepo == "" || prNumber <= 0 {
		return nil
	}

	body := fmt.Sprintf("🔗 **MetaStackr Child PR**: Managed by Parent Meta PR [%s#%d](%s/%s/-/merge_requests/%d)", parentMetaRepo, parentPRNumber, c.baseURL, parentMetaRepo, parentPRNumber)
	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes", c.baseURL, c.projectIDOrPath(childRepo), prNumber)
	payload := map[string]string{"body": body}
	jsonBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *GitLabClient) UpdateSubmodulePointersOnBranch(ctx context.Context, repoFullName string, branchName string, updates []vcs.SubmodulePointerUpdate, installationID int64) error {
	if len(updates) == 0 {
		return nil
	}

	type commitAction struct {
		Action   string `json:"action"`
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}

	var actions []commitAction
	for _, up := range updates {
		shaToUse := up.NewCommitSHA
		subRepoName := up.SubmoduleRepo
		if subRepoName == "" {
			subRepoName = up.SubmodulePath
		}
		if mainSHA, err := c.GetBranchHeadSHA(ctx, subRepoName, "main", installationID); err == nil && mainSHA != "" {
			shaToUse = mainSHA
		}
		if shaToUse != "" && up.SubmodulePath != "" {
			actions = append(actions, commitAction{
				Action:   "update",
				FilePath: up.SubmodulePath,
				Content:  shaToUse,
			})
		}
	}

	if len(actions) == 0 {
		return nil
	}

	commitPayload := map[string]interface{}{
		"branch":         branchName,
		"commit_message": "chore(meta): align submodule pointers to HEAD",
		"actions":        actions,
	}
	jsonBytes, _ := json.Marshal(commitPayload)

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/commits", c.baseURL, c.projectIDOrPath(repoFullName))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to commit submodule pointer update: HTTP %d %s", resp.StatusCode, string(body))
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

	for _, ch := range changesPayload.Changes {
		fn := strings.TrimSpace(ch.NewPath)
		if fn == "" || fn == ".gitmodules" {
			continue
		}
		if !strings.HasPrefix(fn, "services/") && !strings.HasPrefix(fn, "submodules/") {
			return true, nil
		}
	}

	return false, nil
}
