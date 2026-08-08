package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
	"metastackr/internal/db"
	"strings"
)

type Server struct {
	repo          *db.Repository
	gh            *GitHubClient
	webhookSecret string
	reconcileFn   func(ctx context.Context, metaPRID uuid.UUID) error
}

func NewServer(repo *db.Repository, gh *GitHubClient, webhookSecret string, reconcileFn func(ctx context.Context, metaPRID uuid.UUID) error) *Server {
	return &Server{
		repo:          repo,
		gh:            gh,
		webhookSecret: webhookSecret,
		reconcileFn:   reconcileFn,
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	handleWithCORS := func(pattern string, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			handler(w, r)
		})
	}

	handleWithCORS("POST /webhooks/github", s.handleGitHubWebhook)
	handleWithCORS("OPTIONS /webhooks/github", func(w http.ResponseWriter, r *http.Request) {})
	handleWithCORS("GET /api/v1/prs/status", s.handlePRStatusQuery)
	handleWithCORS("OPTIONS /api/v1/prs/status", func(w http.ResponseWriter, r *http.Request) {})
	handleWithCORS("POST /api/v1/prs/retry-merge", s.handleRetryMerge)
	handleWithCORS("OPTIONS /api/v1/prs/retry-merge", func(w http.ResponseWriter, r *http.Request) {})
	handleWithCORS("POST /api/v1/repos/track", s.handleTrackRepo)
	handleWithCORS("OPTIONS /api/v1/repos/track", func(w http.ResponseWriter, r *http.Request) {})
	handleWithCORS("GET /api/v1/repos/settings", s.handleGetRepoSettings)
	handleWithCORS("PUT /api/v1/repos/settings", s.handleUpdateRepoSettings)
	handleWithCORS("POST /api/v1/repos/settings", s.handleUpdateRepoSettings)
	handleWithCORS("OPTIONS /api/v1/repos/settings", func(w http.ResponseWriter, r *http.Request) {})
	handleWithCORS("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
}

func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	sigHeader := r.Header.Get("X-Hub-Signature-256")
	eventTypeHeader := r.Header.Get("X-GitHub-Event")

	payload, err := ReadBody(r)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	signatureVerified := false

	// 1. Primary check: GitHub App webhook secret
	if s.webhookSecret != "" && VerifySignature(s.webhookSecret, payload, sigHeader) {
		signatureVerified = true
	}

	// 2. Fallback check: repository-level webhook secret (tracked repo ID)
	if !signatureVerified {
		var repoPayload struct {
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
		}
		if err := json.Unmarshal(payload, &repoPayload); err == nil && repoPayload.Repository.FullName != "" {
			tracked, err := s.repo.GetTrackedRepoByFullName(r.Context(), repoPayload.Repository.FullName)
			if err == nil && tracked != nil {
				if VerifySignature(tracked.ID.String(), payload, sigHeader) {
					signatureVerified = true
				}
			}
		}
	}

	if !signatureVerified && s.repo != nil {
		if allRepos, err := s.repo.ListTrackedRepos(r.Context()); err == nil {
			for _, tr := range allRepos {
				if VerifySignature(tr.ID.String(), payload, sigHeader) {
					signatureVerified = true
					break
				}
			}
		}
	}

	if !signatureVerified {
		log.Printf("[webhook] 401 Unauthorized: signature verification failed for repo payload (sigHeader: '%s')", sigHeader)
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	event, err := ParseGitHubWebhook(eventTypeHeader, payload)
	if err != nil {
		log.Printf("[webhook] unsupported or unparseable event: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("[webhook] received %s for repo %s PR #%d (branch: %s)", event.EventType, event.Repo, event.PRNumber, event.BranchName)

	ctx := r.Context()

	if err := s.processNormalizedEvent(ctx, event); err != nil {
		log.Printf("[webhook] error processing event: %v", err)
		http.Error(w, "Error processing event", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

func resolveInstallationID(evtID int64, dbInstID string) int64 {
	if evtID > 0 {
		return evtID
	}
	if dbInstID != "" {
		var parsed int64
		_, _ = fmt.Sscanf(dbInstID, "%d", &parsed)
		return parsed
	}
	return 0
}

func (s *Server) processNormalizedEvent(ctx context.Context, evt *NormalizedEvent) error {
	if s.repo == nil || evt == nil {
		return nil
	}

	// Prevent rate-limit infinite loop: ignore check_run / check_suite status webhooks triggered by MetaStackr's own check runs
	if evt.EventType == EventTypeCheckStatus {
		return nil
	}

	// 1. Check if the repo is tracked as a Meta Repo
	tracked, err := s.repo.GetTrackedRepoByFullName(ctx, evt.Repo)
	if err == nil && tracked != nil {
		if evt.InstallationID > 0 && tracked.InstallationID == "" {
			tracked.InstallationID = fmt.Sprintf("%d", evt.InstallationID)
			_ = s.repo.UpdateTrackedRepoInstallationID(ctx, tracked.ID, tracked.InstallationID)
		}
		metaPR, err := s.repo.GetMetaPRByRepoAndNumber(ctx, evt.Repo, evt.PRNumber)
		if (err != nil || metaPR == nil) && evt.BranchName != "" {
			metaPR, _ = s.repo.GetMetaPRByRepoAndBranch(ctx, evt.Repo, evt.BranchName)
		}
		if (err != nil || metaPR == nil) && evt.MergedSHA != "" {
			metaPR, _ = s.repo.GetMetaPRByHeadSHA(ctx, evt.MergedSHA)
		}
		if (err != nil || metaPR == nil) && evt.PRNumber > 0 {
			if !strings.Contains(strings.ToLower(evt.Repo), "sub-") && !strings.Contains(strings.ToLower(evt.Repo), "submodule") {
				metaPR = &db.MetaPR{
					ID:          uuid.New(),
					MetaRepoID:  tracked.ID,
					PRNumber:    evt.PRNumber,
					BranchName:  evt.BranchName,
					BaseBranch:  "main",
					HeadSHA:     evt.MergedSHA,
					Status:      "OPEN",
					LockVersion: 1,
				}
				_ = s.repo.CreateMetaPR(ctx, metaPR)
			}
		}

		if metaPR != nil && metaPR.MetaRepoID == tracked.ID {
			if evt.MergedSHA != "" && metaPR.HeadSHA != evt.MergedSHA {
				metaPR.HeadSHA = evt.MergedSHA
				_ = s.repo.UpdateMetaPRHeadSHA(ctx, metaPR.ID, evt.MergedSHA)
			}

			if evt.EventType == EventTypePRMerged {
				_ = s.repo.UpdateMetaPRStatusWithLock(ctx, metaPR.ID, "MERGED", metaPR.LockVersion)
			} else {
				s.autoSynthesizeChildPRs(ctx, metaPR, evt)
				if updatedChildren, err := s.repo.GetChildPRsByMetaPRID(ctx, metaPR.ID); err == nil {
					metaPR.ChildPRs = updatedChildren
				}
			}

			if metaPR.HeadSHA == "" && metaPR.PRNumber > 0 {
				instID := resolveInstallationID(evt.InstallationID, tracked.InstallationID)
				if fetchedSHA, err := s.gh.GetPRHeadSHA(ctx, tracked.RepoFullName, metaPR.PRNumber, instID); err == nil && fetchedSHA != "" {
					metaPR.HeadSHA = fetchedSHA
					_ = s.repo.UpdateMetaPRHeadSHA(ctx, metaPR.ID, fetchedSHA)
				}
			}

			if metaPR.HeadSHA != "" {
				instID := resolveInstallationID(evt.InstallationID, tracked.InstallationID)
				if err := s.gh.UpdateMetaCheckRun(ctx, tracked.RepoFullName, metaPR.HeadSHA, metaPR, instID); err != nil {
					log.Printf("[checks] Error updating meta check run for %s: %v", tracked.RepoFullName, err)
				}
			}

			if s.gh != nil && metaPR.PRNumber > 0 {
				parentMetaRepoName := tracked.RepoFullName
				if metaPR.MetaRepoID != tracked.ID {
					if parentTracked, err := s.repo.GetTrackedRepoByID(ctx, metaPR.MetaRepoID); err == nil && parentTracked != nil {
						parentMetaRepoName = parentTracked.RepoFullName
					}
				}
				instID := resolveInstallationID(evt.InstallationID, tracked.InstallationID)
				_ = s.gh.EnsureRootPRComment(ctx, parentMetaRepoName, metaPR.PRNumber, metaPR, instID)
			}
			return nil
		}
	}

	// 2. Otherwise check if it is a child PR
	child, err := s.repo.GetChildPRByRepoAndNumber(ctx, evt.Repo, evt.PRNumber)
	if (err != nil || child == nil) && evt.BranchName != "" {
		// Attempt fallback by matching branch name across tracked Meta PRs
		metaPR, _ := s.repo.GetMetaPRByAnyBranch(ctx, evt.BranchName)
		if metaPR != nil {
			child = &db.ChildPR{
				ID:            uuid.New(),
				MetaPRID:      metaPR.ID,
				SubmodulePath: evt.Repo,
				RepoFullName:  evt.Repo,
				PRNumber:      evt.PRNumber,
				HeadSHA:       evt.MergedSHA,
				Status:        "OPEN",
			}
			_ = s.repo.UpsertChildPR(ctx, child)
		}
	}
	
	if child != nil {
		if evt.Merged || evt.EventType == EventTypePRMerged {
			child.Status = "MERGED"
		}
		if evt.MergedSHA != "" {
			child.HeadSHA = evt.MergedSHA
		}

		err = s.repo.UpdateChildPRStatus(ctx, child.ID, child.Status, child.HeadSHA)
		if err != nil {
			return err
		}

		parentMeta, err := s.repo.GetMetaPRByID(ctx, child.MetaPRID)
		if err == nil && parentMeta != nil {
			if updatedChildren, err := s.repo.GetChildPRsByMetaPRID(ctx, parentMeta.ID); err == nil {
				parentMeta.ChildPRs = updatedChildren
			}
			trackedMeta, err := s.repo.GetTrackedRepoByID(ctx, parentMeta.MetaRepoID)
			if err == nil && trackedMeta != nil {
				instID := resolveInstallationID(evt.InstallationID, trackedMeta.InstallationID)
				if parentMeta.HeadSHA == "" && parentMeta.PRNumber > 0 {
					if fetchedSHA, err := s.gh.GetPRHeadSHA(ctx, trackedMeta.RepoFullName, parentMeta.PRNumber, instID); err == nil && fetchedSHA != "" {
						parentMeta.HeadSHA = fetchedSHA
						_ = s.repo.UpdateMetaPRHeadSHA(ctx, parentMeta.ID, fetchedSHA)
					}
				}
				if parentMeta.HeadSHA != "" {
					if err := s.gh.UpdateMetaCheckRun(ctx, trackedMeta.RepoFullName, parentMeta.HeadSHA, parentMeta, instID); err != nil {
						log.Printf("[checks] Error updating meta check run for %s: %v", trackedMeta.RepoFullName, err)
					}
				}
				if s.gh != nil {
					_ = s.gh.EnsureChildPRComment(ctx, child.RepoFullName, child.PRNumber, trackedMeta.RepoFullName, parentMeta.PRNumber, parentMeta.BranchName, instID)
					_ = s.gh.EnsureRootPRComment(ctx, trackedMeta.RepoFullName, parentMeta.PRNumber, parentMeta, instID)
				}
				s.evaluateMetaPRReadiness(ctx, parentMeta)
			}
		}
	}

	return nil
}

func (s *Server) autoSynthesizeChildPRs(ctx context.Context, metaPR *db.MetaPR, evt *NormalizedEvent) {
	log.Printf("[synthesis] Auto-synthesizing child PRs for Meta PR %d", metaPR.PRNumber)
	tracked, _ := s.repo.GetTrackedRepoByID(ctx, metaPR.MetaRepoID)
	parentRepoName := ""
	if tracked != nil {
		parentRepoName = tracked.RepoFullName
	}

	for idx, change := range evt.ChangedFiles {
		childPR := &db.ChildPR{
			ID:            uuid.New(),
			MetaPRID:      metaPR.ID,
			SubmodulePath: change.SubmodulePath,
			RepoFullName:  change.ChildRepo,
			PRNumber:      metaPR.PRNumber,
			HeadSHA:       change.NewSHA,
			Status:        "OPEN",
			DepthLevel:    idx,
		}
		_ = s.repo.UpsertChildPR(ctx, childPR)

		if parentRepoName != "" && s.gh != nil {
			_ = s.gh.EnsureChildPRComment(ctx, change.ChildRepo, metaPR.PRNumber, parentRepoName, metaPR.PRNumber, metaPR.BranchName, evt.InstallationID)
		}
	}

	// Also discover child PRs across all tracked submodules matching the branch name
	if s.gh != nil && metaPR.BranchName != "" {
		instID := resolveInstallationID(evt.InstallationID, tracked.InstallationID)
		if allTracked, err := s.repo.ListTrackedRepos(ctx); err == nil {
			for idx, tr := range allTracked {
				if tr.RepoFullName != parentRepoName {
					cPRNum, cHeadSHA, cMerged, err := s.gh.GetOpenPRForBranch(ctx, tr.RepoFullName, metaPR.BranchName, instID)
					if err == nil && cPRNum > 0 {
						status := "OPEN"
						if cMerged {
							status = "MERGED"
						}
						childPR := &db.ChildPR{
							ID:            uuid.New(),
							MetaPRID:      metaPR.ID,
							SubmodulePath: tr.RepoFullName,
							RepoFullName:  tr.RepoFullName,
							PRNumber:      cPRNum,
							HeadSHA:       cHeadSHA,
							Status:        status,
							DepthLevel:    idx,
						}
						_ = s.repo.UpsertChildPR(ctx, childPR)
						_ = s.gh.EnsureChildPRComment(ctx, tr.RepoFullName, cPRNum, parentRepoName, metaPR.PRNumber, metaPR.BranchName, instID)
					}
				}
			}
		}
	}
}

func (s *Server) evaluateMetaPRReadiness(ctx context.Context, metaPR *db.MetaPR) {
	if len(metaPR.ChildPRs) == 0 {
		return
	}

	trackedMeta, err := s.repo.GetTrackedRepoByID(ctx, metaPR.MetaRepoID)
	if err != nil || trackedMeta == nil {
		return
	}

	// Rule 1: Master auto merge toggle
	if !trackedMeta.AutoMergeEnabled {
		log.Printf("[policy] Auto-merge is disabled for %s", trackedMeta.RepoFullName)
		return
	}

	// Rule 2: All child submodule PRs must be MERGED
	for _, child := range metaPR.ChildPRs {
		if child.Status != "MERGED" {
			return
		}
	}

	instID := resolveInstallationID(0, trackedMeta.InstallationID)

	// Rule 3: Require Root PR Approval (if enabled)
	if trackedMeta.RequireRootApproval && s.gh != nil {
		approved, err := s.gh.HasApprovedReview(ctx, trackedMeta.RepoFullName, metaPR.PRNumber, instID)
		if err != nil {
			log.Printf("[policy] Failed to verify root PR approval for %s#%d: %v", trackedMeta.RepoFullName, metaPR.PRNumber, err)
			return
		}
		if !approved {
			log.Printf("[policy] Root Meta PR %s#%d requires approval before auto-merge. Auto-merge paused.", trackedMeta.RepoFullName, metaPR.PRNumber)
			return
		}
	}

	// Rule 4: Required Status Checks (if specified)
	if len(trackedMeta.RequiredChecks) > 0 && s.gh != nil && metaPR.HeadSHA != "" {
		passing, missing, err := s.gh.AreRequiredChecksPassing(ctx, trackedMeta.RepoFullName, metaPR.HeadSHA, trackedMeta.RequiredChecks, instID)
		if err != nil {
			log.Printf("[policy] Failed to verify required checks for %s#%d: %v", trackedMeta.RepoFullName, metaPR.PRNumber, err)
			return
		}
		if !passing {
			log.Printf("[policy] Meta PR %s#%d has missing/failed required checks (%v). Auto-merge paused.", trackedMeta.RepoFullName, metaPR.PRNumber, missing)
			return
		}
	}

	if metaPR.Status == "OPEN" {
		log.Printf("[policy] All auto-merge rules passed for Meta PR #%d! Triggering cascade merge.", metaPR.PRNumber)
		_ = s.repo.UpdateMetaPRStatusWithLock(ctx, metaPR.ID, "MERGING", metaPR.LockVersion)

		if s.reconcileFn != nil {
			_ = s.reconcileFn(ctx, metaPR.ID)
		}
	}
}

type PRStatusResponse struct {
	MetaPR *db.MetaPR `json:"meta_pr"`
}

func (s *Server) handlePRStatusQuery(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")
	branch := r.URL.Query().Get("branch")
	prStr := r.URL.Query().Get("pr")

	if repo == "" {
		http.Error(w, "missing required parameter: repo", http.StatusBadRequest)
		return
	}

	if s.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"meta_pr": nil,
			"message": "Database repo not initialized",
		})
		return
	}

	var metaPR *db.MetaPR
	var err error

	if prStr != "" {
		var prNum int
		if _, parseErr := fmt.Sscanf(prStr, "%d", &prNum); parseErr == nil && prNum > 0 {
			metaPR, err = s.repo.GetMetaPRByRepoAndNumber(r.Context(), repo, prNum)
			if (metaPR == nil || err != nil) {
				childPR, childErr := s.repo.GetChildPRByRepoAndNumber(r.Context(), repo, prNum)
				if childErr == nil && childPR != nil {
					metaPR, err = s.repo.GetMetaPRByID(r.Context(), childPR.MetaPRID)
				}
			}
		}
	}

	if (metaPR == nil || err != nil) && branch != "" {
		metaPR, err = s.repo.GetMetaPRByRepoAndBranch(r.Context(), repo, branch)
	}

	if err != nil || metaPR == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"meta_pr": nil,
			"message": "No active Meta PR found",
		})
		return
	}

	if tracked, trackedErr := s.repo.GetTrackedRepoByID(r.Context(), metaPR.MetaRepoID); trackedErr == nil && tracked != nil {
		metaPR.MetaRepoFullName = tracked.RepoFullName
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(PRStatusResponse{MetaPR: metaPR})
}

type RetryMergeRequest struct {
	MetaRepo string `json:"meta_repo"`
	PRNumber int    `json:"pr_number"`
}

func (s *Server) handleRetryMerge(w http.ResponseWriter, r *http.Request) {
	var req RetryMergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if s.repo == nil {
		http.Error(w, "database not initialized", http.StatusInternalServerError)
		return
	}

	metaPR, err := s.repo.GetMetaPRByRepoAndNumber(r.Context(), req.MetaRepo, req.PRNumber)
	if err != nil {
		http.Error(w, fmt.Sprintf("Meta PR not found: %v", err), http.StatusNotFound)
		return
	}

	if metaPR.Status != "FAILED_PARTIAL" && metaPR.Status != "FAILED" && metaPR.Status != "OPEN" {
		http.Error(w, fmt.Sprintf("Meta PR is in status %s; retry only allowed for OPEN, FAILED or FAILED_PARTIAL", metaPR.Status), http.StatusBadRequest)
		return
	}

	err = s.repo.UpdateMetaPRStatusWithLock(r.Context(), metaPR.ID, "MERGING", metaPR.LockVersion)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to update status for retry: %v", err), http.StatusConflict)
		return
	}

	if s.reconcileFn != nil {
		if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
			_ = s.reconcileFn(r.Context(), metaPR.ID)
		} else {
			go func() {
				_ = s.reconcileFn(context.Background(), metaPR.ID)
			}()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "retry_initiated",
		"meta_id": metaPR.ID.String(),
	})
}

type trackRepoRequest struct {
	FullName      string `json:"full_name"`
	AllowCodePull bool   `json:"allow_code_pull"`
}

func (s *Server) handleTrackRepo(w http.ResponseWriter, r *http.Request) {
	var req trackRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.FullName == "" || !strings.Contains(req.FullName, "/") {
		http.Error(w, "invalid repository name; must be 'owner/repo'", http.StatusBadRequest)
		return
	}

	parts := strings.Split(req.FullName, "/")
	owner, name := parts[0], parts[1]

	tracked := &db.TrackedMetaRepo{
		RepoOwner:     owner,
		RepoName:      name,
		RepoFullName:  req.FullName,
		IsEnabled:      true,
		AllowCodePull:  req.AllowCodePull,
	}

	if err := s.repo.CreateTrackedRepo(r.Context(), tracked); err != nil {
		http.Error(w, fmt.Sprintf("database error: %v", err), http.StatusInternalServerError)
		return
	}

	appInstalled := false
	accountInstalled := false
	if s.gh != nil {
		ok, instID := s.gh.HasAppAccessToRepo(r.Context(), req.FullName)
		if ok {
			appInstalled = true
			accountInstalled = true
			if instID > 0 {
				tracked.InstallationID = fmt.Sprintf("%d", instID)
				_ = s.repo.CreateTrackedRepo(r.Context(), tracked)
			}
		} else {
			accountInstalled = s.gh.HasAppInstalledOnAccount(r.Context(), owner)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":                  true,
		"repo_id":                  tracked.ID.String(),
		"github_app_installed":     appInstalled,
		"app_installed_on_account": accountInstalled,
	})
}

type RepoSettingsRequest struct {
	RepoFullName        string   `json:"repo"`
	RequireRootApproval bool     `json:"require_root_approval"`
	AutoMergeEnabled    bool     `json:"auto_merge_enabled"`
	RequiredChecks      []string `json:"required_checks"`
	DefaultMergeMethod  string   `json:"default_merge_method"`
}

func (s *Server) handleGetRepoSettings(w http.ResponseWriter, r *http.Request) {
	repoFullName := r.URL.Query().Get("repo")
	if repoFullName == "" {
		http.Error(w, "missing required query param: repo", http.StatusBadRequest)
		return
	}

	if s.repo == nil {
		http.Error(w, "database repo not initialized", http.StatusInternalServerError)
		return
	}

	tracked, err := s.repo.GetTrackedRepoByFullName(r.Context(), repoFullName)
	if err != nil || tracked == nil {
		http.Error(w, "repository settings not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tracked)
}

func (s *Server) handleUpdateRepoSettings(w http.ResponseWriter, r *http.Request) {
	var req RepoSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.RepoFullName == "" {
		http.Error(w, "missing field: repo", http.StatusBadRequest)
		return
	}

	if s.repo == nil {
		http.Error(w, "database repo not initialized", http.StatusInternalServerError)
		return
	}

	err := s.repo.UpdateTrackedRepoSettings(r.Context(), req.RepoFullName, req.RequireRootApproval, req.AutoMergeEnabled, req.RequiredChecks, req.DefaultMergeMethod)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to update settings: %v", err), http.StatusInternalServerError)
		return
	}

	tracked, _ := s.repo.GetTrackedRepoByFullName(r.Context(), req.RepoFullName)

	// Re-evaluate open Meta PRs for this repository when settings are updated
	if tracked != nil && tracked.AutoMergeEnabled {
		if metaPRs, err := s.repo.GetOpenMetaPRsByRepoID(r.Context(), tracked.ID); err == nil {
			for _, mPR := range metaPRs {
				s.evaluateMetaPRReadiness(r.Context(), mPR)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"settings": tracked,
	})
}

