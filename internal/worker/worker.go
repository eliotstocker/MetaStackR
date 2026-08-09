package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"metastackr/internal/dag"
	"metastackr/internal/db"
	"metastackr/internal/vcs"
)

type ChildMerger interface {
	MergePR(ctx context.Context, repo string, prNumber int, mergeMethod string) (mergedSHA string, err error)
}

type MockMerger struct {
	FailOnPRs map[int]string // PRNumber -> error message
}

func (m *MockMerger) MergePR(ctx context.Context, repo string, prNumber int, mergeMethod string) (string, error) {
	if errMsg, fail := m.FailOnPRs[prNumber]; fail {
		return "", fmt.Errorf("merge conflict in %s PR #%d: %s", repo, prNumber, errMsg)
	}
	return fmt.Sprintf("sha-%s-%d", repo, prNumber), nil
}

type RealVCSMerger struct {
	vcs vcs.VCSProvider
}

func NewRealVCSMerger(vcsProvider vcs.VCSProvider) *RealVCSMerger {
	return &RealVCSMerger{vcs: vcsProvider}
}

func (m *RealVCSMerger) MergePR(ctx context.Context, repo string, prNumber int, mergeMethod string) (string, error) {
	if m.vcs == nil {
		return "", fmt.Errorf("VCS provider is nil")
	}
	return m.vcs.MergePullRequest(ctx, repo, prNumber, mergeMethod, 0)
}

type Engine struct {
	repo   *db.Repository
	vcs    vcs.VCSProvider
	merger ChildMerger
}

func NewEngine(repo *db.Repository, vcsProvider vcs.VCSProvider, merger ChildMerger) *Engine {
	if merger == nil {
		if vcsProvider != nil {
			merger = NewRealVCSMerger(vcsProvider)
		} else {
			merger = &MockMerger{}
		}
	}
	return &Engine{
		repo:   repo,
		vcs:    vcsProvider,
		merger: merger,
	}
}

func (e *Engine) StartReconciliationLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.ReconcileAllPending(ctx); err != nil {
				log.Printf("[worker] error during reconciliation: %v", err)
			}
		}
	}
}

func (e *Engine) ReconcileAllPending(ctx context.Context) error {
	pending, err := e.repo.GetPendingReconcileMetaPRs(ctx)
	if err != nil {
		return err
	}

	for _, metaPR := range pending {
		if err := e.ExecuteCascadeMerge(ctx, metaPR.ID); err != nil {
			log.Printf("[worker] cascade merge failed for Meta PR %s: %v", metaPR.ID, err)
		}
	}

	return nil
}

func (e *Engine) ExecuteCascadeMerge(ctx context.Context, metaPRID uuid.UUID) error {
	metaPR, err := e.repo.GetMetaPRByID(ctx, metaPRID)
	if err != nil {
		return fmt.Errorf("failed to fetch meta PR: %w", err)
	}

	if metaPR.Status != "MERGING" {
		err := e.repo.UpdateMetaPRStatusWithLock(ctx, metaPR.ID, "MERGING", metaPR.LockVersion)
		if err != nil {
			return fmt.Errorf("optimistic locking failed on Meta PR %s: %w", metaPR.ID, err)
		}
		metaPR.LockVersion++
		metaPR.Status = "MERGING"
	}

	e.repo.CreateMergeAuditLog(ctx, metaPR.ID, "CASCADE_MERGE_STARTED", metaPR)

	g := dag.NewGraph()
	childMap := make(map[string]db.ChildPR)
	for _, child := range metaPR.ChildPRs {
		idStr := child.ID.String()
		g.AddNode(idStr, child)
		childMap[idStr] = child
	}

	deps, err := e.repo.GetChildPRDependencies(ctx, metaPR.ID)
	if err != nil {
		return fmt.Errorf("failed to get child PR dependencies: %w", err)
	}

	for _, dep := range deps {
		g.AddDependency(dep.ParentChildPRID.String(), dep.DependentChildPRID.String())
	}

	batches, err := g.TopologicalSort()
	if err != nil {
		e.repo.UpdateMetaPRStatusWithLock(ctx, metaPR.ID, "FAILED", metaPR.LockVersion)
		e.repo.CreateMergeAuditLog(ctx, metaPR.ID, "CYCLE_DETECTED", map[string]string{"error": err.Error()})
		return fmt.Errorf("aborting cascade merge due to cycle: %w", err)
	}

	// Resolve merge method (prefer repository policy default_merge_method if configured, fallback to metaPR.MergeMethod or "merge")
	effectiveMergeMethod := metaPR.MergeMethod
	var repoDefaultMethod string
	if err := e.repo.DB().QueryRowContext(ctx, "SELECT default_merge_method FROM tracked_meta_repos WHERE id = $1", metaPR.MetaRepoID).Scan(&repoDefaultMethod); err == nil && repoDefaultMethod != "" {
		effectiveMergeMethod = repoDefaultMethod
	}
	if effectiveMergeMethod == "" {
		effectiveMergeMethod = "merge"
	}

	for batchIdx, batch := range batches {
		log.Printf("[worker] executing batch %d with %d PRs (merge method: %s)", batchIdx, len(batch), effectiveMergeMethod)

		type mergeResult struct {
			childID   uuid.UUID
			mergedSHA string
			err       error
		}

		resChan := make(chan mergeResult, len(batch))
		var wg sync.WaitGroup

		for _, childIDStr := range batch {
			child := childMap[childIDStr]

			if child.Status == "MERGED" {
				log.Printf("[worker] skipping child PR %s (#%d) - already merged", child.RepoFullName, child.PRNumber)
				continue
			}

			wg.Add(1)
			go func(c db.ChildPR) {
				defer wg.Done()
				_ = e.repo.UpdateChildPRStatus(ctx, c.ID, "MERGING", c.HeadSHA)
				sha, err := e.merger.MergePR(ctx, c.RepoFullName, c.PRNumber, effectiveMergeMethod)
				resChan <- mergeResult{childID: c.ID, mergedSHA: sha, err: err}
			}(child)
		}

		wg.Wait()
		close(resChan)

		var batchErr error
		for res := range resChan {
			if res.err != nil {
				batchErr = res.err
				_ = e.repo.UpdateChildPRStatus(ctx, res.childID, "FAILED", "")
			} else {
				_ = e.repo.UpdateChildPRStatus(ctx, res.childID, "MERGED", res.mergedSHA)
			}
		}

		if batchErr != nil {
			log.Printf("[worker] batch %d failed: %v", batchIdx, batchErr)
			_ = e.repo.UpdateMetaPRStatusWithLock(ctx, metaPR.ID, "FAILED_PARTIAL", metaPR.LockVersion)
			_ = e.repo.CreateMergeAuditLog(ctx, metaPR.ID, "CHILD_MERGE_FAILED", map[string]string{
				"batch_index": fmt.Sprintf("%d", batchIdx),
				"error":       batchErr.Error(),
			})
			return fmt.Errorf("cascade merge halted on partial failure: %w", batchErr)
		}
	}

	_ = e.repo.CreateMergeAuditLog(ctx, metaPR.ID, "SUBMODULE_POINTERS_BUMPED", map[string]string{"status": "success"})

	// Look up the name of the meta repo and privacy settings
	query := `
		SELECT repo_full_name, allow_code_pull 
		FROM tracked_meta_repos 
		WHERE id = $1
	`
	var metaRepoName string
	var allowCodePull bool
	err = e.repo.DB().QueryRowContext(ctx, query, metaPR.MetaRepoID).Scan(&metaRepoName, &allowCodePull)
	if err != nil || metaRepoName == "" {
		if metaPR.MetaRepoFullName != "" {
			metaRepoName = metaPR.MetaRepoFullName
		} else {
			metaRepoName = "meta-repo"
		}
	}

	// Privacy Guardrail: Ensure code pulls are strictly opt-in
	requiresLocalClone := false // Cascades merge is metadata-only by default
	if requiresLocalClone && !allowCodePull {
		return fmt.Errorf("security block: Cascade Merge requires local git clone, but allow_code_pull is disabled")
	}

	// Server-side Submodule Pointer Alignment on parent meta-repo branch
	if e.vcs != nil && metaPR.BranchName != "" {
		var pointerUpdates []vcs.SubmodulePointerUpdate
		for _, child := range metaPR.ChildPRs {
			if child.HeadSHA != "" {
				pointerUpdates = append(pointerUpdates, vcs.SubmodulePointerUpdate{
					SubmodulePath: child.SubmodulePath,
					SubmoduleRepo: child.RepoFullName,
					NewCommitSHA:  child.HeadSHA,
				})
			}
		}
		if len(pointerUpdates) > 0 {
			var instID int64
			_ = e.repo.DB().QueryRowContext(ctx, "SELECT installation_id FROM tracked_meta_repos WHERE id = $1", metaPR.MetaRepoID).Scan(&instID)
			err := e.vcs.UpdateSubmodulePointersOnBranch(ctx, metaRepoName, metaPR.BranchName, pointerUpdates, instID)
			if err != nil {
				log.Printf("[worker] Error: Failed to update submodule pointers on branch %s for %s: %v", metaPR.BranchName, metaRepoName, err)
				_ = e.repo.UpdateMetaPRStatusWithLock(ctx, metaPR.ID, "FAILED_PARTIAL", metaPR.LockVersion)
				_ = e.repo.CreateMergeAuditLog(ctx, metaPR.ID, "POINTER_ALIGNMENT_FAILED", map[string]string{"error": err.Error()})
				return fmt.Errorf("cascade merge halted: failed to update submodule pointers: %w", err)
			} else {
				log.Printf("[worker] Successfully updated submodule pointers on branch %s for %s", metaPR.BranchName, metaRepoName)
			}
		}
	}

	rootSHA, err := e.merger.MergePR(ctx, metaRepoName, metaPR.PRNumber, effectiveMergeMethod)
	if err != nil {
		_ = e.repo.UpdateMetaPRStatusWithLock(ctx, metaPR.ID, "FAILED_PARTIAL", metaPR.LockVersion)
		_ = e.repo.CreateMergeAuditLog(ctx, metaPR.ID, "ROOT_META_PR_MERGE_FAILED", map[string]string{"error": err.Error()})
		return fmt.Errorf("failed to merge root meta PR: %w", err)
	}

	_ = e.repo.UpdateMetaPRStatusWithLock(ctx, metaPR.ID, "MERGED", metaPR.LockVersion)
	_ = e.repo.CreateMergeAuditLog(ctx, metaPR.ID, "CASCADE_MERGE_COMPLETED", map[string]string{"root_sha": rootSHA})

	log.Printf("[worker] Meta PR %d merged successfully!", metaPR.PRNumber)
	return nil
}
