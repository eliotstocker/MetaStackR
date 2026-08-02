package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"metastackr/internal/db"
)

type GitHubClient struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

func NewGitHubClient(token string) *GitHubClient {
	if token == "" {
		token = "dummy_token"
	}
	return &GitHubClient{
		token:      token,
		baseURL:    "https://api.github.com",
		httpClient: &http.Client{},
	}
}

type CheckRunOutput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Text    string `json:"text"`
}

type CheckRunPayload struct {
	Name       string          `json:"name"`
	HeadSHA    string          `json:"head_sha"`
	Status     string          `json:"status"` // "queued", "in_progress", "completed"
	Conclusion *string         `json:"conclusion,omitempty"` // "success", "failure", "neutral", "cancelled", etc.
	Output     CheckRunOutput  `json:"output"`
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

// UpdateMetaCheckRun creates or updates the single GitHub Check Run named 'meta-repo/sync'.
func (c *GitHubClient) UpdateMetaCheckRun(ctx context.Context, metaRepo string, headSHA string, metaPR *db.MetaPR) error {
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
		Name:    "meta-repo/sync",
		HeadSHA: headSHA,
		Status:  status,
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

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to post check run to GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("GitHub API returned HTTP %d for check run update", resp.StatusCode)
	}

	return nil
}
