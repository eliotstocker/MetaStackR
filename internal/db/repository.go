package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound               = errors.New("record not found")
	ErrOptimisticLockConflict = errors.New("optimistic lock conflict: record was modified by another process")
)

type Repository struct {
	db *DB
}

func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) DB() *DB {
	return r.db
}

func (r *Repository) CreateTrackedRepo(ctx context.Context, repo *TrackedMetaRepo) error {
	if repo.ID == uuid.Nil {
		repo.ID = uuid.New()
	}
	repo.CreatedAt = time.Now()

	query := `
		INSERT INTO tracked_meta_repos (id, repo_owner, repo_name, repo_full_name, installation_id, is_enabled, allow_code_pull, require_root_approval, auto_merge_enabled, required_checks, default_merge_method, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (repo_full_name) 
		DO UPDATE SET installation_id = EXCLUDED.installation_id, is_enabled = EXCLUDED.is_enabled, allow_code_pull = EXCLUDED.allow_code_pull
		RETURNING id, created_at
	`
	reqChecksJSON, _ := json.Marshal(repo.RequiredChecks)
	if len(repo.RequiredChecks) == 0 {
		reqChecksJSON = []byte("[]")
	}
	if repo.DefaultMergeMethod == "" {
		repo.DefaultMergeMethod = "merge"
	}
	err := r.db.QueryRowContext(
		ctx, query,
		repo.ID, repo.RepoOwner, repo.RepoName, repo.RepoFullName, repo.InstallationID, repo.IsEnabled, repo.AllowCodePull, repo.RequireRootApproval, repo.AutoMergeEnabled, string(reqChecksJSON), repo.DefaultMergeMethod, repo.CreatedAt,
	).Scan(&repo.ID, &repo.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create tracked repo: %w", err)
	}
	return nil
}

func (r *Repository) GetTrackedRepoByFullName(ctx context.Context, fullName string) (*TrackedMetaRepo, error) {
	query := `
		SELECT id, repo_owner, repo_name, repo_full_name, installation_id, is_enabled, allow_code_pull, require_root_approval, auto_merge_enabled, COALESCE(submodule_changes_only, true), required_checks, default_merge_method, created_at
		FROM tracked_meta_repos
		WHERE repo_full_name = $1
	`
	repo := &TrackedMetaRepo{}
	var reqChecksBytes []byte
	err := r.db.QueryRowContext(ctx, query, fullName).Scan(
		&repo.ID, &repo.RepoOwner, &repo.RepoName, &repo.RepoFullName, &repo.InstallationID, &repo.IsEnabled, &repo.AllowCodePull, &repo.RequireRootApproval, &repo.AutoMergeEnabled, &repo.SubmoduleChangesOnly, &reqChecksBytes, &repo.DefaultMergeMethod, &repo.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tracked repo: %w", err)
	}
	repo.RequiredChecks = parseJSONBStringArray(reqChecksBytes)
	return repo, nil
}

func (r *Repository) GetTrackedRepoByID(ctx context.Context, id uuid.UUID) (*TrackedMetaRepo, error) {
	query := `
		SELECT id, repo_owner, repo_name, repo_full_name, installation_id, is_enabled, allow_code_pull, require_root_approval, auto_merge_enabled, COALESCE(submodule_changes_only, true), required_checks, default_merge_method, created_at
		FROM tracked_meta_repos
		WHERE id = $1
	`
	repo := &TrackedMetaRepo{}
	var reqChecksBytes []byte
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&repo.ID, &repo.RepoOwner, &repo.RepoName, &repo.RepoFullName, &repo.InstallationID, &repo.IsEnabled, &repo.AllowCodePull, &repo.RequireRootApproval, &repo.AutoMergeEnabled, &repo.SubmoduleChangesOnly, &reqChecksBytes, &repo.DefaultMergeMethod, &repo.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tracked repo by id: %w", err)
	}
	repo.RequiredChecks = parseJSONBStringArray(reqChecksBytes)
	return repo, nil
}

func (r *Repository) UpdateTrackedRepoInstallationID(ctx context.Context, id uuid.UUID, installationID string) error {
	query := `UPDATE tracked_meta_repos SET installation_id = $1 WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, installationID, id)
	return err
}

func (r *Repository) UpdateTrackedRepoSettings(ctx context.Context, repoFullName string, requireRootApproval bool, autoMergeEnabled bool, requiredChecks []string, defaultMergeMethod string, submoduleChangesOnly bool) error {
	reqChecksJSON, _ := json.Marshal(requiredChecks)
	if len(requiredChecks) == 0 {
		reqChecksJSON = []byte("[]")
	}
	if defaultMergeMethod == "" {
		defaultMergeMethod = "merge"
	}

	query := `
		UPDATE tracked_meta_repos 
		SET require_root_approval = $1, auto_merge_enabled = $2, required_checks = $3, default_merge_method = $4, submodule_changes_only = $5
		WHERE repo_full_name = $6
	`
	_, err := r.db.ExecContext(ctx, query, requireRootApproval, autoMergeEnabled, string(reqChecksJSON), defaultMergeMethod, submoduleChangesOnly, repoFullName)
	return err
}

func parseJSONBStringArray(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var res []string
	if err := json.Unmarshal(raw, &res); err != nil {
		return []string{}
	}
	return res
}

func (r *Repository) ListTrackedRepos(ctx context.Context) ([]*TrackedMetaRepo, error) {
	query := `
		SELECT id, repo_owner, repo_name, repo_full_name, installation_id, is_enabled, allow_code_pull, created_at
		FROM tracked_meta_repos
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list tracked repos: %w", err)
	}
	defer rows.Close()

	var repos []*TrackedMetaRepo
	for rows.Next() {
		repo := &TrackedMetaRepo{}
		if err := rows.Scan(&repo.ID, &repo.RepoOwner, &repo.RepoName, &repo.RepoFullName, &repo.InstallationID, &repo.IsEnabled, &repo.AllowCodePull, &repo.CreatedAt); err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

func (r *Repository) CreateMetaPR(ctx context.Context, pr *MetaPR) error {
	if pr.ID == uuid.Nil {
		pr.ID = uuid.New()
	}
	pr.CreatedAt = time.Now()
	pr.UpdatedAt = time.Now()
	if pr.Status == "" {
		pr.Status = "OPEN"
	}
	if pr.LockVersion == 0 {
		pr.LockVersion = 1
	}
	if pr.MergeMethod == "" {
		pr.MergeMethod = "merge"
	}

	query := `
		INSERT INTO meta_prs (id, meta_repo_id, pr_number, branch_name, base_branch, merge_method, head_sha, status, lock_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (meta_repo_id, pr_number) 
		DO UPDATE SET branch_name = EXCLUDED.branch_name, merge_method = EXCLUDED.merge_method, head_sha = CASE WHEN EXCLUDED.head_sha <> '' THEN EXCLUDED.head_sha ELSE meta_prs.head_sha END, status = EXCLUDED.status, updated_at = NOW()
		RETURNING id, head_sha, status, lock_version, created_at, updated_at
	`
	err := r.db.QueryRowContext(
		ctx, query,
		pr.ID, pr.MetaRepoID, pr.PRNumber, pr.BranchName, pr.BaseBranch, pr.MergeMethod, pr.HeadSHA, pr.Status, pr.LockVersion, pr.CreatedAt, pr.UpdatedAt,
	).Scan(&pr.ID, &pr.HeadSHA, &pr.Status, &pr.LockVersion, &pr.CreatedAt, &pr.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create meta_pr: %w", err)
	}
	return nil
}

func (r *Repository) GetMetaPRByRepoAndNumber(ctx context.Context, repoFullName string, prNumber int) (*MetaPR, error) {
	tracked, err := r.GetTrackedRepoByFullName(ctx, repoFullName)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, meta_repo_id, pr_number, branch_name, base_branch, merge_method, head_sha, status, lock_version, created_at, updated_at
		FROM meta_prs
		WHERE meta_repo_id = $1 AND pr_number = $2
	`
	pr := &MetaPR{}
	err = r.db.QueryRowContext(ctx, query, tracked.ID, prNumber).Scan(
		&pr.ID, &pr.MetaRepoID, &pr.PRNumber, &pr.BranchName, &pr.BaseBranch, &pr.MergeMethod, &pr.HeadSHA, &pr.Status, &pr.LockVersion, &pr.CreatedAt, &pr.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get meta_pr: %w", err)
	}

	children, err := r.GetChildPRsByMetaPRID(ctx, pr.ID)
	if err != nil {
		return nil, err
	}
	pr.ChildPRs = children

	return pr, nil
}

func (r *Repository) GetMetaPRByRepoAndBranch(ctx context.Context, repoFullName, branchName string) (*MetaPR, error) {
	tracked, err := r.GetTrackedRepoByFullName(ctx, repoFullName)
	if err != nil {
		return r.GetMetaPRByAnyBranch(ctx, branchName)
	}

	query := `
		SELECT id, meta_repo_id, pr_number, branch_name, base_branch, merge_method, status, lock_version, created_at, updated_at
		FROM meta_prs
		WHERE meta_repo_id = $1 AND branch_name = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	pr := &MetaPR{}
	err = r.db.QueryRowContext(ctx, query, tracked.ID, branchName).Scan(
		&pr.ID, &pr.MetaRepoID, &pr.PRNumber, &pr.BranchName, &pr.BaseBranch, &pr.MergeMethod, &pr.Status, &pr.LockVersion, &pr.CreatedAt, &pr.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return r.GetMetaPRByAnyBranch(ctx, branchName)
		}
		return nil, fmt.Errorf("failed to get meta_pr by branch: %w", err)
	}

	children, err := r.GetChildPRsByMetaPRID(ctx, pr.ID)
	if err != nil {
		return nil, err
	}
	pr.ChildPRs = children

	return pr, nil
}

func (r *Repository) GetMetaPRByAnyBranch(ctx context.Context, branchName string) (*MetaPR, error) {
	query := `
		SELECT id, meta_repo_id, pr_number, branch_name, base_branch, merge_method, head_sha, status, lock_version, created_at, updated_at
		FROM meta_prs
		WHERE branch_name = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	pr := &MetaPR{}
	err := r.db.QueryRowContext(ctx, query, branchName).Scan(
		&pr.ID, &pr.MetaRepoID, &pr.PRNumber, &pr.BranchName, &pr.BaseBranch, &pr.MergeMethod, &pr.HeadSHA, &pr.Status, &pr.LockVersion, &pr.CreatedAt, &pr.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get meta_pr by any branch: %w", err)
	}

	children, err := r.GetChildPRsByMetaPRID(ctx, pr.ID)
	if err != nil {
		return nil, err
	}
	pr.ChildPRs = children

	return pr, nil
}

func (r *Repository) GetMetaPRByHeadSHA(ctx context.Context, headSHA string) (*MetaPR, error) {
	if headSHA == "" {
		return nil, ErrNotFound
	}
	query := `
		SELECT id, meta_repo_id, pr_number, branch_name, base_branch, merge_method, head_sha, status, lock_version, created_at, updated_at
		FROM meta_prs
		WHERE head_sha = $1
		ORDER BY updated_at DESC
		LIMIT 1
	`
	pr := &MetaPR{}
	err := r.db.QueryRowContext(ctx, query, headSHA).Scan(
		&pr.ID, &pr.MetaRepoID, &pr.PRNumber, &pr.BranchName, &pr.BaseBranch, &pr.MergeMethod, &pr.HeadSHA, &pr.Status, &pr.LockVersion, &pr.CreatedAt, &pr.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get meta_pr by head_sha: %w", err)
	}

	children, err := r.GetChildPRsByMetaPRID(ctx, pr.ID)
	if err != nil {
		return nil, err
	}
	pr.ChildPRs = children

	return pr, nil
}

func (r *Repository) GetMetaPRByID(ctx context.Context, id uuid.UUID) (*MetaPR, error) {
	query := `
		SELECT id, meta_repo_id, pr_number, branch_name, base_branch, merge_method, head_sha, status, lock_version, created_at, updated_at
		FROM meta_prs
		WHERE id = $1
	`
	pr := &MetaPR{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&pr.ID, &pr.MetaRepoID, &pr.PRNumber, &pr.BranchName, &pr.BaseBranch, &pr.MergeMethod, &pr.HeadSHA, &pr.Status, &pr.LockVersion, &pr.CreatedAt, &pr.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get meta_pr by id: %w", err)
	}

	children, err := r.GetChildPRsByMetaPRID(ctx, pr.ID)
	if err != nil {
		return nil, err
	}
	pr.ChildPRs = children

	return pr, nil
}

func (r *Repository) GetOpenMetaPRsByRepoID(ctx context.Context, metaRepoID uuid.UUID) ([]*MetaPR, error) {
	query := `
		SELECT id, meta_repo_id, pr_number, branch_name, base_branch, merge_method, head_sha, status, lock_version, created_at, updated_at
		FROM meta_prs
		WHERE meta_repo_id = $1 AND status = 'OPEN'
	`
	rows, err := r.db.QueryContext(ctx, query, metaRepoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get open meta_prs: %w", err)
	}
	defer rows.Close()

	var result []*MetaPR
	for rows.Next() {
		pr := &MetaPR{}
		if err := rows.Scan(&pr.ID, &pr.MetaRepoID, &pr.PRNumber, &pr.BranchName, &pr.BaseBranch, &pr.MergeMethod, &pr.HeadSHA, &pr.Status, &pr.LockVersion, &pr.CreatedAt, &pr.UpdatedAt); err != nil {
			return nil, err
		}
		children, _ := r.GetChildPRsByMetaPRID(ctx, pr.ID)
		pr.ChildPRs = children
		result = append(result, pr)
	}
	return result, nil
}

func (r *Repository) UpdateMetaPRStatusWithLock(ctx context.Context, id uuid.UUID, status string, expectedLockVersion int) error {
	query := `
		UPDATE meta_prs
		SET status = $1, lock_version = lock_version + 1, updated_at = NOW()
		WHERE id = $2 AND lock_version = $3
	`
	res, err := r.db.ExecContext(ctx, query, status, id, expectedLockVersion)
	if err != nil {
		return fmt.Errorf("failed to update meta_pr status: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrOptimisticLockConflict
	}
	return nil
}

func (r *Repository) UpdateMetaPRHeadSHA(ctx context.Context, id uuid.UUID, headSHA string) error {
	query := `
		UPDATE meta_prs
		SET head_sha = $1, updated_at = NOW()
		WHERE id = $2
	`
	_, err := r.db.ExecContext(ctx, query, headSHA, id)
	return err
}

func (r *Repository) UpsertChildPR(ctx context.Context, child *ChildPR) error {
	if child.ID == uuid.Nil {
		child.ID = uuid.New()
	}
	child.CreatedAt = time.Now()
	if child.Status == "" {
		child.Status = "OPEN"
	}

	query := `
		INSERT INTO child_prs (id, meta_pr_id, submodule_path, repo_full_name, pr_number, head_sha, status, depth_level, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (meta_pr_id, submodule_path)
		DO UPDATE SET 
			status = CASE WHEN child_prs.status = 'MERGED' THEN 'MERGED' ELSE EXCLUDED.status END,
			head_sha = EXCLUDED.head_sha,
			depth_level = EXCLUDED.depth_level
		RETURNING id, created_at
	`
	err := r.db.QueryRowContext(
		ctx, query,
		child.ID, child.MetaPRID, child.SubmodulePath, child.RepoFullName, child.PRNumber,
		child.HeadSHA, child.Status, child.DepthLevel, child.CreatedAt,
	).Scan(&child.ID, &child.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert child_pr: %w", err)
	}
	return nil
}

func (r *Repository) GetChildPRsByMetaPRID(ctx context.Context, metaPRID uuid.UUID) ([]ChildPR, error) {
	query := `
		SELECT id, meta_pr_id, submodule_path, repo_full_name, pr_number, head_sha, status, depth_level, created_at
		FROM child_prs
		WHERE meta_pr_id = $1
		ORDER BY depth_level ASC, submodule_path ASC
	`
	rows, err := r.db.QueryContext(ctx, query, metaPRID)
	if err != nil {
		return nil, fmt.Errorf("failed to query child_prs: %w", err)
	}
	defer rows.Close()

	var results []ChildPR
	for rows.Next() {
		var c ChildPR
		if err := rows.Scan(
			&c.ID, &c.MetaPRID, &c.SubmodulePath, &c.RepoFullName, &c.PRNumber,
			&c.HeadSHA, &c.Status, &c.DepthLevel, &c.CreatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, c)
	}

	for i := range results {
		deps, err := r.GetChildPRDependenciesForChild(ctx, results[i].ID)
		if err != nil {
			return nil, err
		}
		results[i].DependsOnIDs = deps
	}

	return results, nil
}

func (r *Repository) UpdateChildPRStatus(ctx context.Context, childID uuid.UUID, status string, headSHA string) error {
	query := `
		UPDATE child_prs
		SET status = $1, head_sha = $2
		WHERE id = $3
	`
	_, err := r.db.ExecContext(ctx, query, status, headSHA, childID)
	if err != nil {
		return fmt.Errorf("failed to update child_pr status: %w", err)
	}
	return nil
}

func (r *Repository) AddChildPRDependency(ctx context.Context, parentChildPRID, dependentChildPRID uuid.UUID) error {
	query := `
		INSERT INTO child_pr_dependencies (parent_child_pr_id, dependent_child_pr_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	_, err := r.db.ExecContext(ctx, query, parentChildPRID, dependentChildPRID)
	if err != nil {
		return fmt.Errorf("failed to add child_pr dependency: %w", err)
	}
	return nil
}

func (r *Repository) GetChildPRDependenciesForChild(ctx context.Context, childID uuid.UUID) ([]uuid.UUID, error) {
	query := `
		SELECT dependent_child_pr_id
		FROM child_pr_dependencies
		WHERE parent_child_pr_id = $1
	`
	rows, err := r.db.QueryContext(ctx, query, childID)
	if err != nil {
		return nil, fmt.Errorf("failed to query dependencies: %w", err)
	}
	defer rows.Close()

	var deps []uuid.UUID
	for rows.Next() {
		var d uuid.UUID
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, nil
}

func (r *Repository) GetChildPRDependencies(ctx context.Context, metaPRID uuid.UUID) ([]ChildPRDependency, error) {
	query := `
		SELECT d.parent_child_pr_id, d.dependent_child_pr_id
		FROM child_pr_dependencies d
		JOIN child_prs c ON d.parent_child_pr_id = c.id
		WHERE c.meta_pr_id = $1
	`
	rows, err := r.db.QueryContext(ctx, query, metaPRID)
	if err != nil {
		return nil, fmt.Errorf("failed to query dependencies for meta_pr: %w", err)
	}
	defer rows.Close()

	var deps []ChildPRDependency
	for rows.Next() {
		var dep ChildPRDependency
		if err := rows.Scan(&dep.ParentChildPRID, &dep.DependentChildPRID); err != nil {
			return nil, err
		}
		deps = append(deps, dep)
	}
	return deps, nil
}

func (r *Repository) CreateMergeAuditLog(ctx context.Context, metaPRID uuid.UUID, eventType string, payload interface{}) error {
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal audit log payload: %w", err)
	}

	query := `
		INSERT INTO merge_audit_logs (meta_pr_id, event_type, payload, created_at)
		VALUES ($1, $2, $3, NOW())
	`
	_, err = r.db.ExecContext(ctx, query, metaPRID, eventType, jsonBytes)
	if err != nil {
		return fmt.Errorf("failed to insert merge_audit_log: %w", err)
	}
	return nil
}

func (r *Repository) GetPendingReconcileMetaPRs(ctx context.Context) ([]MetaPR, error) {
	query := `
		SELECT id, meta_repo_id, pr_number, branch_name, base_branch, merge_method, head_sha, status, lock_version, created_at, updated_at
		FROM meta_prs
		WHERE status IN ('SYNCING', 'MERGING')
		ORDER BY updated_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending reconcile meta_prs: %w", err)
	}
	defer rows.Close()

	var results []MetaPR
	for rows.Next() {
		var pr MetaPR
		if err := rows.Scan(
			&pr.ID, &pr.MetaRepoID, &pr.PRNumber, &pr.BranchName, &pr.BaseBranch, &pr.MergeMethod, &pr.HeadSHA, &pr.Status, &pr.LockVersion, &pr.CreatedAt, &pr.UpdatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, pr)
	}

	for i := range results {
		children, err := r.GetChildPRsByMetaPRID(ctx, results[i].ID)
		if err != nil {
			return nil, err
		}
		results[i].ChildPRs = children
	}

	return results, nil
}

func (r *Repository) GetChildPRByRepoAndNumber(ctx context.Context, childRepo string, prNumber int) (*ChildPR, error) {
	query := `
		SELECT id, meta_pr_id, submodule_path, repo_full_name, pr_number, head_sha, status, depth_level, created_at
		FROM child_prs
		WHERE (repo_full_name = $1 OR submodule_path = $1) AND pr_number = $2
	`
	child := &ChildPR{}
	err := r.db.QueryRowContext(ctx, query, childRepo, prNumber).Scan(
		&child.ID, &child.MetaPRID, &child.SubmodulePath, &child.RepoFullName, &child.PRNumber,
		&child.HeadSHA, &child.Status, &child.DepthLevel, &child.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return child, nil
}

func (r *Repository) GetCachedInstallationToken(ctx context.Context, installationID int64) (string, error) {
	query := `
		SELECT token
		FROM app_installation_tokens
		WHERE installation_id = $1 AND expires_at > NOW() + INTERVAL '2 minutes'
	`
	var token string
	err := r.db.QueryRowContext(ctx, query, installationID).Scan(&token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("failed to query cached installation token: %w", err)
	}
	return token, nil
}

func (r *Repository) SaveInstallationToken(ctx context.Context, installationID int64, token string, expiresAt time.Time) error {
	query := `
		INSERT INTO app_installation_tokens (installation_id, token, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (installation_id) DO UPDATE SET token = EXCLUDED.token, expires_at = EXCLUDED.expires_at
	`
	_, err := r.db.ExecContext(ctx, query, installationID, token, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to save installation token: %w", err)
	}
	return nil
}
