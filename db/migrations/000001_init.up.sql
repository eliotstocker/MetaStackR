CREATE TYPE pr_status AS ENUM (
    'OPEN',
    'SYNCING',
    'READY',
    'MERGING',
    'MERGED',
    'FAILED_PARTIAL',
    'FAILED'
);

CREATE TYPE review_status AS ENUM ('PENDING', 'APPROVED', 'CHANGES_REQUESTED');
CREATE TYPE ci_status AS ENUM ('PENDING', 'SUCCESS', 'FAILURE');

CREATE TABLE meta_prs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    meta_repo VARCHAR(255) NOT NULL,
    pr_number INT NOT NULL,
    branch_name VARCHAR(255) NOT NULL,
    status pr_status NOT NULL DEFAULT 'OPEN',
    lock_version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(meta_repo, pr_number)
);

CREATE TABLE child_prs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    meta_pr_id UUID NOT NULL REFERENCES meta_prs(id) ON DELETE CASCADE,
    submodule_path VARCHAR(255) NOT NULL,
    child_repo VARCHAR(255) NOT NULL,
    pr_number INT NOT NULL,
    status pr_status NOT NULL DEFAULT 'OPEN',
    review_state review_status NOT NULL DEFAULT 'PENDING',
    ci_state ci_status NOT NULL DEFAULT 'PENDING',
    merged_sha VARCHAR(40),
    error_message TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(child_repo, pr_number)
);

CREATE TABLE child_pr_dependencies (
    parent_child_id UUID NOT NULL REFERENCES child_prs(id) ON DELETE CASCADE,
    depends_on_child_id UUID NOT NULL REFERENCES child_prs(id) ON DELETE CASCADE,
    PRIMARY KEY (parent_child_id, depends_on_child_id)
);

CREATE TABLE merge_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    meta_pr_id UUID NOT NULL REFERENCES meta_prs(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_meta_prs_reconcile ON meta_prs(status) WHERE status IN ('MERGING', 'SYNCING');
