package db

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ReviewStatus string

const (
	ReviewStatusPending          ReviewStatus = "PENDING"
	ReviewStatusApproved         ReviewStatus = "APPROVED"
	ReviewStatusChangesRequested ReviewStatus = "CHANGES_REQUESTED"
)

type CIStatus string

const (
	CIStatusPending CIStatus = "PENDING"
	CIStatusSuccess CIStatus = "SUCCESS"
	CIStatusFailure CIStatus = "FAILURE"
)

type TrackedMetaRepo struct {
	ID             uuid.UUID `json:"id"`
	RepoOwner      string    `json:"repo_owner"`
	RepoName       string    `json:"repo_name"`
	RepoFullName   string    `json:"repo_full_name"`
	InstallationID string    `json:"installation_id"`
	IsEnabled           bool      `json:"is_enabled"`
	AllowCodePull       bool      `json:"allow_code_pull"`
	RequireRootApproval bool      `json:"require_root_approval"`
	AutoMergeEnabled    bool      `json:"auto_merge_enabled"`
	RequiredChecks      []string  `json:"required_checks"`
	DefaultMergeMethod  string    `json:"default_merge_method"`
	CreatedAt           time.Time `json:"created_at"`
}

type MetaPR struct {
	ID          uuid.UUID `json:"id"`
	MetaRepoID  uuid.UUID `json:"meta_repo_id"`
	PRNumber    int       `json:"pr_number"`
	BranchName  string    `json:"branch_name"`
	BaseBranch  string    `json:"base_branch"`
	MergeMethod string    `json:"merge_method"` // merge, squash, rebase
	HeadSHA     string    `json:"head_sha"`
	Status      string    `json:"status"` // OPEN, MERGING, MERGED, FAILED_DRIFT, FAILED_PARTIAL
	LockVersion int       `json:"lock_version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	MetaRepoFullName string `json:"meta_repo_full_name,omitempty"`

	ChildPRs []ChildPR `json:"child_prs,omitempty"`
}

type ChildPR struct {
	ID            uuid.UUID `json:"id"`
	MetaPRID      uuid.UUID `json:"meta_pr_id"`
	SubmodulePath string    `json:"submodule_path"`
	RepoFullName  string    `json:"repo_full_name"`
	PRNumber      int       `json:"pr_number"`
	HeadSHA       string    `json:"head_sha"`
	Status        string    `json:"status"` // OPEN, MERGED, FAILED
	DepthLevel    int       `json:"depth_level"`
	CreatedAt     time.Time `json:"created_at"`

	DependsOnIDs []uuid.UUID `json:"depends_on_ids,omitempty"`
}

type ChildPRDependency struct {
	ParentChildPRID    uuid.UUID `json:"parent_child_pr_id"`
	DependentChildPRID uuid.UUID `json:"dependent_child_pr_id"`
}

type MergeAuditLog struct {
	ID        int64           `json:"id"`
	MetaPRID  uuid.UUID       `json:"meta_pr_id"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}
