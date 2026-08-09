package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"metastackr/internal/db"
)

func TestVerifySignature(t *testing.T) {
	secret := "my_webhook_secret"
	payload := []byte(`{"action":"opened"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !VerifySignature(secret, payload, validSig) {
		t.Error("expected valid signature to pass verification")
	}

	if VerifySignature(secret, payload, "sha256=invalid_signature_hash") {
		t.Error("expected invalid signature to fail verification")
	}
}

func TestParseGitHubWebhook_PullRequest(t *testing.T) {
	payload := []byte(`{
		"action": "opened",
		"number": 42,
		"pull_request": {
			"number": 42,
			"merged": false,
			"head": {
				"ref": "feature/multi-repo",
				"sha": "abc12345"
			}
		},
		"repository": {
			"full_name": "org/meta-repo"
		}
	}`)

	evt, err := ParseGitHubWebhook("pull_request", payload)
	if err != nil {
		t.Fatalf("unexpected error parsing PR webhook: %v", err)
	}

	if evt.EventType != EventTypePROpened {
		t.Errorf("got EventType %s, want %s", evt.EventType, EventTypePROpened)
	}
	if evt.PRNumber != 42 {
		t.Errorf("got PRNumber %d, want 42", evt.PRNumber)
	}
	if evt.Repo != "org/meta-repo" {
		t.Errorf("got Repo %s, want org/meta-repo", evt.Repo)
	}
	if evt.BranchName != "feature/multi-repo" {
		t.Errorf("got BranchName %s, want feature/multi-repo", evt.BranchName)
	}
}

func TestParseGitHubWebhook_PRReview(t *testing.T) {
	payload := []byte(`{
		"action": "submitted",
		"review": {
			"state": "approved"
		},
		"pull_request": {
			"number": 15,
			"head": {
				"ref": "feature/submodule-a"
			}
		},
		"repository": {
			"full_name": "org/sub-repo"
		}
	}`)

	evt, err := ParseGitHubWebhook("pull_request_review", payload)
	if err != nil {
		t.Fatalf("unexpected error parsing review webhook: %v", err)
	}

	if evt.EventType != EventTypeReview {
		t.Errorf("got EventType %s, want %s", evt.EventType, EventTypeReview)
	}
	if evt.ReviewState != db.ReviewStatusApproved {
		t.Errorf("got ReviewState %s, want APPROVED", evt.ReviewState)
	}
}
