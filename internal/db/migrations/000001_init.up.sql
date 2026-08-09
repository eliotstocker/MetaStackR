CREATE TABLE IF NOT EXISTS tracked_meta_repos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_owner VARCHAR(255) NOT NULL,
    repo_name VARCHAR(255) NOT NULL,
    repo_full_name VARCHAR(255) NOT NULL UNIQUE,
    installation_id VARCHAR(255) NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    allow_code_pull BOOLEAN NOT NULL DEFAULT FALSE,
    require_root_approval BOOLEAN NOT NULL DEFAULT TRUE,
    auto_merge_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    required_checks JSONB NOT NULL DEFAULT '[]'::jsonb,
    default_merge_method VARCHAR(50) NOT NULL DEFAULT 'merge',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS meta_prs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    meta_repo_id UUID NOT NULL REFERENCES tracked_meta_repos(id) ON DELETE CASCADE,
    pr_number INT NOT NULL,
    branch_name VARCHAR(255) NOT NULL,
    base_branch VARCHAR(255) NOT NULL DEFAULT 'main',
    merge_method VARCHAR(50) NOT NULL DEFAULT 'merge',
    status VARCHAR(50) NOT NULL DEFAULT 'OPEN', -- OPEN, MERGING, MERGED, FAILED_DRIFT, FAILED_PARTIAL
    lock_version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(meta_repo_id, pr_number)
);

CREATE TABLE IF NOT EXISTS child_prs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    meta_pr_id UUID NOT NULL REFERENCES meta_prs(id) ON DELETE CASCADE,
    submodule_path VARCHAR(255) NOT NULL,
    repo_full_name VARCHAR(255) NOT NULL,
    pr_number INT NOT NULL,
    head_sha VARCHAR(40) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'OPEN', -- OPEN, MERGED, FAILED
    depth_level INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(meta_pr_id, submodule_path)
);

CREATE TABLE IF NOT EXISTS child_pr_dependencies (
    parent_child_pr_id UUID NOT NULL REFERENCES child_prs(id) ON DELETE CASCADE,
    dependent_child_pr_id UUID NOT NULL REFERENCES child_prs(id) ON DELETE CASCADE,
    PRIMARY KEY (parent_child_pr_id, dependent_child_pr_id)
);

CREATE TABLE IF NOT EXISTS merge_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    meta_pr_id UUID NOT NULL REFERENCES meta_prs(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_meta_prs_reconcile ON meta_prs(status) WHERE status IN ('MERGING', 'SYNCING');

CREATE TABLE IF NOT EXISTS app_installation_tokens (
    installation_id BIGINT PRIMARY KEY,
    token TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
