package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"metastackr/internal/db"
)

var (
	ErrInvalidSignature = errors.New("invalid webhook signature")
	ErrUnsupportedEvent = errors.New("unsupported webhook event type")
)

type EventType string

const (
	EventTypePROpened    EventType = "PR_OPENED"
	EventTypePRUpdated   EventType = "PR_UPDATED"
	EventTypePRClosed    EventType = "PR_CLOSED"
	EventTypePRMerged    EventType = "PR_MERGED"
	EventTypeReview      EventType = "REVIEW_SUBMITTED"
	EventTypeCheckStatus EventType = "CHECK_STATUS_CHANGED"
)

type NormalizedEvent struct {
	EventType    EventType
	Repo         string
	PRNumber     int
	BranchName   string
	Action       string
	ReviewState  db.ReviewStatus
	CIState      db.CIStatus
	Merged       bool
	MergedSHA    string
	ChangedFiles []SubmoduleChange
	RawPayload   json.RawMessage
}

type SubmoduleChange struct {
	SubmodulePath string
	ChildRepo     string
	NewSHA        string
}

// VerifySignature validates the X-Hub-Signature-256 header using the provided secret.
func VerifySignature(secret string, payload []byte, signatureHeader string) bool {
	if secret == "" {
		return true // If no secret configured, bypass
	}
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}
	actualSig := strings.TrimPrefix(signatureHeader, "sha256=")
	decodedSig, err := hex.DecodeString(actualSig)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSig := mac.Sum(nil)

	return hmac.Equal(decodedSig, expectedSig)
}

// ParseGitHubWebhook parses GitHub event headers and body into a NormalizedEvent.
func ParseGitHubWebhook(eventTypeHeader string, payload []byte) (*NormalizedEvent, error) {
	switch eventTypeHeader {
	case "pull_request":
		return parsePullRequestEvent(payload)
	case "pull_request_review":
		return parsePullRequestReviewEvent(payload)
	case "check_run":
		return parseCheckRunEvent(payload)
	case "workflow_run":
		return parseWorkflowRunEvent(payload)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedEvent, eventTypeHeader)
	}
}

type ghPRPayload struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Number int  `json:"number"`
		Merged bool `json:"merged"`
		Head   struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func parsePullRequestEvent(payload []byte) (*NormalizedEvent, error) {
	var p ghPRPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, err
	}

	evtType := EventTypePRUpdated
	if p.Action == "opened" || p.Action == "reopened" {
		evtType = EventTypePROpened
	} else if p.Action == "closed" {
		if p.PullRequest.Merged {
			evtType = EventTypePRMerged
		} else {
			evtType = EventTypePRClosed
		}
	}

	return &NormalizedEvent{
		EventType:  evtType,
		Repo:       p.Repository.FullName,
		PRNumber:   p.Number,
		BranchName: p.PullRequest.Head.Ref,
		Action:     p.Action,
		Merged:     p.PullRequest.Merged,
		MergedSHA:  p.PullRequest.Head.SHA,
		RawPayload: json.RawMessage(payload),
	}, nil
}

type ghReviewPayload struct {
	Action string `json:"action"`
	Review struct {
		State string `json:"state"`
	} `json:"review"`
	PullRequest struct {
		Number int `json:"number"`
		Head   struct {
			Ref string `json:"ref"`
		} `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func parsePullRequestReviewEvent(payload []byte) (*NormalizedEvent, error) {
	var p ghReviewPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, err
	}

	var reviewState db.ReviewStatus
	switch strings.ToLower(p.Review.State) {
	case "approved":
		reviewState = db.ReviewStatusApproved
	case "changes_requested":
		reviewState = db.ReviewStatusChangesRequested
	default:
		reviewState = db.ReviewStatusPending
	}

	return &NormalizedEvent{
		EventType:   EventTypeReview,
		Repo:        p.Repository.FullName,
		PRNumber:    p.PullRequest.Number,
		BranchName:  p.PullRequest.Head.Ref,
		Action:      p.Action,
		ReviewState: reviewState,
		RawPayload:  json.RawMessage(payload),
	}, nil
}

type ghCheckRunPayload struct {
	Action   string `json:"action"`
	CheckRun struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		Name       string `json:"name"`
		CheckSuite struct {
			PullRequests []struct {
				Number int `json:"number"`
				Head   struct {
					Ref string `json:"ref"`
				} `json:"head"`
			} `json:"pull_requests"`
		} `json:"check_suite"`
	} `json:"check_run"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func parseCheckRunEvent(payload []byte) (*NormalizedEvent, error) {
	var p ghCheckRunPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, err
	}

	var ciState db.CIStatus
	switch strings.ToLower(p.CheckRun.Conclusion) {
	case "success":
		ciState = db.CIStatusSuccess
	case "failure", "timed_out", "action_required", "cancelled":
		ciState = db.CIStatusFailure
	default:
		ciState = db.CIStatusPending
	}

	prNum := 0
	branch := ""
	if len(p.CheckRun.CheckSuite.PullRequests) > 0 {
		prNum = p.CheckRun.CheckSuite.PullRequests[0].Number
		branch = p.CheckRun.CheckSuite.PullRequests[0].Head.Ref
	}

	return &NormalizedEvent{
		EventType:  EventTypeCheckStatus,
		Repo:       p.Repository.FullName,
		PRNumber:   prNum,
		BranchName: branch,
		Action:     p.Action,
		CIState:    ciState,
		RawPayload: json.RawMessage(payload),
	}, nil
}

type ghWorkflowRunPayload struct {
	Action      string `json:"action"`
	WorkflowRun struct {
		Status       string `json:"status"`
		Conclusion   string `json:"conclusion"`
		HeadBranch   string `json:"head_branch"`
		PullRequests []struct {
			Number int `json:"number"`
		} `json:"pull_requests"`
	} `json:"workflow_run"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func parseWorkflowRunEvent(payload []byte) (*NormalizedEvent, error) {
	var p ghWorkflowRunPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, err
	}

	var ciState db.CIStatus
	switch strings.ToLower(p.WorkflowRun.Conclusion) {
	case "success":
		ciState = db.CIStatusSuccess
	case "failure":
		ciState = db.CIStatusFailure
	default:
		ciState = db.CIStatusPending
	}

	prNum := 0
	if len(p.WorkflowRun.PullRequests) > 0 {
		prNum = p.WorkflowRun.PullRequests[0].Number
	}

	return &NormalizedEvent{
		EventType:  EventTypeCheckStatus,
		Repo:       p.Repository.FullName,
		PRNumber:   prNum,
		BranchName: p.WorkflowRun.HeadBranch,
		Action:     p.Action,
		CIState:    ciState,
		RawPayload: json.RawMessage(payload),
	}, nil
}

// ReadBody reads request body up to limit to prevent memory exhaustion.
func ReadBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
}
