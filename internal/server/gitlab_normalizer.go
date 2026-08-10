package server

import (
	"encoding/json"
	"fmt"
	"strings"

	"metastackr/internal/db"
)

type GitLabMergeRequestEvent struct {
	ObjectKind       string `json:"object_kind"`
	EventTypeCode    string `json:"event_type"`
	ObjectAttributes struct {
		ID              int64  `json:"id"`
		IID             int    `json:"iid"`
		TargetBranch    string `json:"target_branch"`
		SourceBranch    string `json:"source_branch"`
		Action          string `json:"action"`
		State           string `json:"state"`
		LastCommit      struct {
			ID string `json:"id"`
		} `json:"last_commit"`
	} `json:"object_attributes"`
	Project struct {
		ID                int64  `json:"id"`
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
}

func ParseGitLabWebhook(eventType string, payload []byte) (*NormalizedEvent, error) {
	if !strings.EqualFold(eventType, "Merge Request Hook") && !strings.EqualFold(eventType, "System Hook") {
		return nil, fmt.Errorf("unsupported GitLab event type: %s", eventType)
	}

	var raw GitLabMergeRequestEvent
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GitLab webhook payload: %w", err)
	}

	if raw.ObjectKind != "merge_request" && raw.ObjectAttributes.IID == 0 {
		return nil, fmt.Errorf("non-MR event in GitLab webhook payload")
	}

	action := strings.ToLower(raw.ObjectAttributes.Action)
	var eType EventType
	switch action {
	case "open", "reopen":
		eType = EventTypePROpened
	case "update":
		eType = EventTypePRUpdated
	case "close":
		eType = EventTypePRClosed
	case "merge":
		eType = EventTypePRMerged
	default:
		eType = EventTypePRUpdated
	}

	reviewStatus := db.ReviewStatusPending
	if action == "approved" {
		reviewStatus = db.ReviewStatusApproved
	}

	return &NormalizedEvent{
		EventType:      eType,
		Repo:           raw.Project.PathWithNamespace,
		PRNumber:       raw.ObjectAttributes.IID,
		BranchName:     raw.ObjectAttributes.SourceBranch,
		Action:         action,
		ReviewState:    reviewStatus,
		CIState:        db.CIStatusPending,
		MergedSHA:      raw.ObjectAttributes.LastCommit.ID,
		Merged:         action == "merge" || raw.ObjectAttributes.State == "merged",
	}, nil
}
