package vcs

import (
	"context"

	"metastackr/internal/db"
)

// SubmodulePointerUpdate represents a submodule pointer SHA update on a branch.
type SubmodulePointerUpdate struct {
	SubmodulePath string
	SubmoduleRepo string
	NewCommitSHA  string
}

// VCSProvider abstracts Version Control System (VCS) operations across GitHub, GitLab, etc.
type VCSProvider interface {
	GetPRHeadSHA(ctx context.Context, repo string, prNumber int, installationID int64) (string, error)
	GetBranchHeadSHA(ctx context.Context, repo string, branchName string, installationID int64) (string, error)
	UpdateMetaCheckRun(ctx context.Context, repo string, headSHA string, metaPR *db.MetaPR, installationID int64) error
	EnsureRootPRComment(ctx context.Context, repo string, prNumber int, metaPR *db.MetaPR, installationID int64) error
	EnsureChildPRComment(ctx context.Context, childRepo string, prNumber int, parentMetaRepo string, parentPRNumber int, branchName string, installationID int64) error
	UpdateSubmodulePointersOnBranch(ctx context.Context, repo string, branch string, updates []SubmodulePointerUpdate, installationID int64) error
	MergePullRequest(ctx context.Context, repo string, prNumber int, mergeMethod string, installationID int64) (string, error)
	HasApprovedReview(ctx context.Context, repo string, prNumber int, installationID int64) (bool, error)
	AreRequiredChecksPassing(ctx context.Context, repo string, headSHA string, required []string, installationID int64) (bool, []string, error)
	HasNonSubmoduleFilesChanged(ctx context.Context, repo string, prNumber int, installationID int64) (bool, error)
}
