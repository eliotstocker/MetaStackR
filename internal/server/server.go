package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/google/uuid"
	"metastackr/internal/db"
	"metastackr/internal/vcs"
)


type Server struct {
	repo          *db.Repository
	vcs           vcs.VCSProvider
	gh            *GitHubClient
	webhookSecret string
	reconcileFn   func(ctx context.Context, metaPRID uuid.UUID) error
}

func NewServer(repo *db.Repository, gh *GitHubClient, webhookSecret string, reconcileFn func(ctx context.Context, metaPRID uuid.UUID) error) *Server {
	return &Server{
		repo:          repo,
		gh:            gh,
		vcs:           gh,
		webhookSecret: webhookSecret,
		reconcileFn:   reconcileFn,
	}
}

func (s *Server) VCS() vcs.VCSProvider {
	if s.vcs != nil {
		return s.vcs
	}
	if s.gh != nil {
		return s.gh
	}
	return nil
}

func (s *Server) VCSForRepo(ctx context.Context, repoFullName string) vcs.VCSProvider {
	if s.repo != nil && repoFullName != "" {
		if tracked, err := s.repo.GetTrackedRepoByFullName(ctx, repoFullName); err == nil && tracked != nil {
			if tracked.VCSProvider == "gitlab" {
				token := tracked.VCSToken
				if token == "" && tracked.RepoOwner != "" {
					token, _ = s.repo.GetUserVCSToken(ctx, "gitlab", tracked.RepoOwner)
				}
				if token == "" {
					parts := strings.Split(repoFullName, "/")
					if len(parts) > 1 {
						token, _ = s.repo.GetUserVCSToken(ctx, "gitlab", parts[0])
					}
				}
				if token == "" {
					token = os.Getenv("GITLAB_TOKEN")
				}
				return NewGitLabClient("", token)
			}
		}
	}
	if strings.Contains(strings.ToLower(repoFullName), "gitlab") {
		token := ""
		parts := strings.Split(repoFullName, "/")
		if len(parts) > 0 && s.repo != nil {
			token, _ = s.repo.GetUserVCSToken(ctx, "gitlab", parts[0])
		}
		if token == "" {
			token = os.Getenv("GITLAB_TOKEN")
		}
		return NewGitLabClient("", token)
	}
	return s.VCS()
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
	handleWithCORS("POST /webhooks/gitlab", s.handleGitLabWebhook)
	handleWithCORS("OPTIONS /webhooks/gitlab", func(w http.ResponseWriter, r *http.Request) {})
	handleWithCORS("GET /oauth/gitlab/callback", s.handleGitLabOAuthCallback)
	handleWithCORS("OPTIONS /oauth/gitlab/callback", func(w http.ResponseWriter, r *http.Request) {})
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

	if err := s.processNormalizedEvent(r.Context(), event); err != nil {
		log.Printf("[webhook] error processing event: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

func (s *Server) handleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	tokenHeader := r.Header.Get("X-Gitlab-Token")
	eventTypeHeader := r.Header.Get("X-Gitlab-Event")

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	if s.webhookSecret != "" && tokenHeader != "" && tokenHeader != s.webhookSecret {
		isValidToken := false
		if s.repo != nil {
			if repoUUID, err := uuid.Parse(tokenHeader); err == nil {
				if _, err := s.repo.GetTrackedRepoByID(r.Context(), repoUUID); err == nil {
					isValidToken = true
				}
			}
		}
		if !isValidToken {
			http.Error(w, "Invalid secret token", http.StatusUnauthorized)
			return
		}
	}

	event, err := ParseGitLabWebhook(eventTypeHeader, payload)
	if err != nil {
		log.Printf("[gitlab-webhook] unparseable event: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := s.processNormalizedEvent(r.Context(), event); err != nil {
		log.Printf("[gitlab-webhook] error processing event: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

			vcsClient := s.VCSForRepo(ctx, tracked.RepoFullName)
			if metaPR.HeadSHA == "" && metaPR.PRNumber > 0 {
				instID := resolveInstallationID(evt.InstallationID, tracked.InstallationID)
				if fetchedSHA, err := vcsClient.GetPRHeadSHA(ctx, tracked.RepoFullName, metaPR.PRNumber, instID); err == nil && fetchedSHA != "" {
					metaPR.HeadSHA = fetchedSHA
					_ = s.repo.UpdateMetaPRHeadSHA(ctx, metaPR.ID, fetchedSHA)
				}
			}

			if metaPR.HeadSHA != "" {
				instID := resolveInstallationID(evt.InstallationID, tracked.InstallationID)
				if err := vcsClient.UpdateMetaCheckRun(ctx, tracked.RepoFullName, metaPR.HeadSHA, metaPR, instID); err != nil {
					log.Printf("[checks] Error updating meta check run for %s: %v", tracked.RepoFullName, err)
				}
			}

			if metaPR.PRNumber > 0 {
				parentMetaRepoName := tracked.RepoFullName
				if metaPR.MetaRepoID != tracked.ID {
					if parentTracked, err := s.repo.GetTrackedRepoByID(ctx, metaPR.MetaRepoID); err == nil && parentTracked != nil {
						parentMetaRepoName = parentTracked.RepoFullName
					}
				}
				instID := resolveInstallationID(evt.InstallationID, tracked.InstallationID)
				parentVCS := s.VCSForRepo(ctx, parentMetaRepoName)
				if parentVCS != nil {
					_ = parentVCS.EnsureRootPRComment(ctx, parentMetaRepoName, metaPR.PRNumber, metaPR, instID)
				}
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
			initStatus := "OPEN"
			if evt.Merged || evt.EventType == EventTypePRMerged {
				initStatus = "MERGED"
			}
			child = &db.ChildPR{
				ID:            uuid.New(),
				MetaPRID:      metaPR.ID,
				SubmodulePath: evt.Repo,
				RepoFullName:  evt.Repo,
				PRNumber:      evt.PRNumber,
				HeadSHA:       evt.MergedSHA,
				Status:        initStatus,
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
				parentVCS := s.VCSForRepo(ctx, trackedMeta.RepoFullName)
				childVCS := s.VCSForRepo(ctx, child.RepoFullName)
				if parentMeta.HeadSHA == "" && parentMeta.PRNumber > 0 {
					if fetchedSHA, err := parentVCS.GetPRHeadSHA(ctx, trackedMeta.RepoFullName, parentMeta.PRNumber, instID); err == nil && fetchedSHA != "" {
						parentMeta.HeadSHA = fetchedSHA
						_ = s.repo.UpdateMetaPRHeadSHA(ctx, parentMeta.ID, fetchedSHA)
					}
				}
				if parentMeta.HeadSHA != "" {
					if err := parentVCS.UpdateMetaCheckRun(ctx, trackedMeta.RepoFullName, parentMeta.HeadSHA, parentMeta, instID); err != nil {
						log.Printf("[checks] Error updating meta check run for %s: %v", trackedMeta.RepoFullName, err)
					}
				}
				if childVCS != nil {
					_ = childVCS.EnsureChildPRComment(ctx, child.RepoFullName, child.PRNumber, trackedMeta.RepoFullName, parentMeta.PRNumber, parentMeta.BranchName, instID)
				}
				if parentVCS != nil {
					_ = parentVCS.EnsureRootPRComment(ctx, trackedMeta.RepoFullName, parentMeta.PRNumber, parentMeta, instID)
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

		childVCS := s.VCSForRepo(ctx, change.ChildRepo)
		if parentRepoName != "" && childVCS != nil {
			_ = childVCS.EnsureChildPRComment(ctx, change.ChildRepo, metaPR.PRNumber, parentRepoName, metaPR.PRNumber, metaPR.BranchName, evt.InstallationID)
		}
	}

	// Also discover child PRs across all tracked submodules matching the branch name
	if metaPR.BranchName != "" {
		instID := resolveInstallationID(evt.InstallationID, tracked.InstallationID)
		if allTracked, err := s.repo.ListTrackedRepos(ctx); err == nil {
			for idx, tr := range allTracked {
				if tr.RepoFullName != parentRepoName {
					trVCS := s.VCSForRepo(ctx, tr.RepoFullName)
					if trVCS != nil {
						cPRNum, cHeadSHA, cMerged, err := trVCS.GetOpenPRForBranch(ctx, tr.RepoFullName, metaPR.BranchName, instID)
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
							_ = trVCS.EnsureChildPRComment(ctx, tr.RepoFullName, cPRNum, parentRepoName, metaPR.PRNumber, metaPR.BranchName, instID)
						}
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
	parentVCS := s.VCSForRepo(ctx, trackedMeta.RepoFullName)

	// Rule 3: Require Root PR Approval (if enabled)
	if trackedMeta.RequireRootApproval && parentVCS != nil {
		approved, err := parentVCS.HasApprovedReview(ctx, trackedMeta.RepoFullName, metaPR.PRNumber, instID)
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
	if len(trackedMeta.RequiredChecks) > 0 && parentVCS != nil && metaPR.HeadSHA != "" {
		passing, missing, err := parentVCS.AreRequiredChecksPassing(ctx, trackedMeta.RepoFullName, metaPR.HeadSHA, trackedMeta.RequiredChecks, instID)
		if err != nil {
			log.Printf("[policy] Failed to verify required checks for %s#%d: %v", trackedMeta.RepoFullName, metaPR.PRNumber, err)
			return
		}
		if !passing {
			log.Printf("[policy] Meta PR %s#%d has missing/failed required checks (%v). Auto-merge paused.", trackedMeta.RepoFullName, metaPR.PRNumber, missing)
			return
		}
	}

	// Rule 5: Submodule Changes Only (if enabled, default true)
	if trackedMeta.SubmoduleChangesOnly && parentVCS != nil && metaPR.PRNumber > 0 {
		nonSubFiles, err := parentVCS.HasNonSubmoduleFilesChanged(ctx, trackedMeta.RepoFullName, metaPR.PRNumber, instID)
		if err != nil {
			log.Printf("[policy] Failed to verify PR changed files for %s#%d: %v", trackedMeta.RepoFullName, metaPR.PRNumber, err)
			return
		}
		if nonSubFiles {
			log.Printf("[policy] Meta PR %s#%d has modified files outside submodules. Auto-merge paused.", trackedMeta.RepoFullName, metaPR.PRNumber)
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
		if len(metaPR.ChildPRs) == 0 {
			evt := &NormalizedEvent{
				Repo:           tracked.RepoFullName,
				PRNumber:       metaPR.PRNumber,
				BranchName:     metaPR.BranchName,
				InstallationID: tracked.InstallationID,
			}
			s.autoSynthesizeChildPRs(r.Context(), metaPR, evt)
			if updatedChildren, err := s.repo.GetChildPRsByMetaPRID(r.Context(), metaPR.ID); err == nil && len(updatedChildren) > 0 {
				metaPR.ChildPRs = updatedChildren
			}
		} else {
			hasChanged := false
			for i, child := range metaPR.ChildPRs {
				if child.Status == "OPEN" && child.PRNumber > 0 {
					childVCS := s.VCSForRepo(r.Context(), child.RepoFullName)
					if childVCS != nil {
						instID := resolveInstallationID(0, tracked.InstallationID)
						_, _, merged, err := childVCS.GetOpenPRForBranch(r.Context(), child.RepoFullName, metaPR.BranchName, instID)
						if err == nil && merged {
							metaPR.ChildPRs[i].Status = "MERGED"
							_ = s.repo.UpdateChildPRStatus(r.Context(), child.ID, "MERGED", child.HeadSHA)
							hasChanged = true
						}
					}
				}
			}
		vcsClient := s.VCSForRepo(r.Context(), tracked.RepoFullName)
		if vcsClient != nil {
			instID := resolveInstallationID(0, tracked.InstallationID)
			if metaPR.HeadSHA != "" {
				_ = vcsClient.UpdateMetaCheckRun(r.Context(), tracked.RepoFullName, metaPR.HeadSHA, metaPR, instID)
			}
			if metaPR.PRNumber > 0 {
				_ = vcsClient.EnsureRootPRComment(r.Context(), tracked.RepoFullName, metaPR.PRNumber, metaPR, instID)
			}
		}
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
	VCSProvider   string `json:"vcs_provider"`
	VCSToken      string `json:"vcs_token"`
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

	vcsProv := req.VCSProvider
	if vcsProv == "" || vcsProv == "unknown" {
		if strings.Contains(strings.ToLower(req.FullName), "gitlab") {
			vcsProv = "gitlab"
		} else {
			vcsProv = "github"
		}
	}

	vcsToken := req.VCSToken
	if vcsToken == "" && s.repo != nil {
		if userTok, err := s.repo.GetUserVCSToken(r.Context(), vcsProv, owner); err == nil && userTok != "" {
			vcsToken = userTok
		}
	}

	tracked := &db.TrackedMetaRepo{
		RepoOwner:     owner,
		RepoName:      name,
		RepoFullName:  req.FullName,
		IsEnabled:      true,
		AllowCodePull:  req.AllowCodePull,
		VCSProvider:    vcsProv,
		VCSToken:       vcsToken,
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
	RepoFullName         string   `json:"repo"`
	RequireRootApproval  bool     `json:"require_root_approval"`
	AutoMergeEnabled     bool     `json:"auto_merge_enabled"`
	SubmoduleChangesOnly bool     `json:"submodule_changes_only"`
	VCSToken             string   `json:"vcs_token"`
	VCSProvider          string   `json:"vcs_provider"`
	RequiredChecks       []string `json:"required_checks"`
	DefaultMergeMethod   string   `json:"default_merge_method"`
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

	err := s.repo.UpdateTrackedRepoSettings(r.Context(), req.RepoFullName, req.RequireRootApproval, req.AutoMergeEnabled, req.RequiredChecks, req.DefaultMergeMethod, req.SubmoduleChangesOnly, req.VCSToken, req.VCSProvider)
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

func (s *Server) handleGitLabOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	errParam := r.URL.Query().Get("error")
	errDesc := r.URL.Query().Get("error_description")

	if errParam != "" {
		http.Error(w, fmt.Sprintf("GitLab OAuth error: %s - %s", errParam, errDesc), http.StatusBadRequest)
		return
	}

	if code == "" {
		http.Error(w, "Missing 'code' parameter in authorization callback", http.StatusBadRequest)
		return
	}

	clientID := os.Getenv("GITLAB_CLIENT_ID")
	clientSecret := os.Getenv("GITLAB_CLIENT_SECRET")
	redirectURI := os.Getenv("GITLAB_REDIRECT_URI")
	if redirectURI == "" {
		redirectURI = "https://api.metastac.kr/oauth/gitlab/callback"
	}

	if clientID == "" || clientSecret == "" {
		http.Error(w, "GitLab OAuth credentials (GITLAB_CLIENT_ID / GITLAB_CLIENT_SECRET) not configured on server", http.StatusInternalServerError)
		return
	}

	// 1. Exchange authorization code for token
	tokenURL := "https://gitlab.com/oauth/token"
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create token request: %v", err), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to contact GitLab OAuth token endpoint: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response from GitLab token endpoint", http.StatusInternalServerError)
		return
	}

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("GitLab OAuth token exchange failed (HTTP %d): %s", resp.StatusCode, string(bodyBytes)), http.StatusBadRequest)
		return
	}

	var tokenData struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		CreatedAt    int64  `json:"created_at"`
	}
	if err := json.Unmarshal(bodyBytes, &tokenData); err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse token response: %v", err), http.StatusInternalServerError)
		return
	}

	// 2. Fetch authenticated user profile from GitLab
	var glUser struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		Avatar   string `json:"avatar_url"`
	}
	userReq, userErr := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://gitlab.com/api/v4/user", nil)
	if userErr == nil {
		userReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
		userReq.Header.Set("Accept", "application/json")
		if userResp, err := http.DefaultClient.Do(userReq); err == nil {
			defer userResp.Body.Close()
			userBytes, _ := io.ReadAll(userResp.Body)
			_ = json.Unmarshal(userBytes, &glUser)
		}
	}

	// 3. Save OAuth Access Token to user_vcs_tokens database table for this user
	if s.repo != nil && tokenData.AccessToken != "" {
		username := glUser.Username
		if username == "" {
			username = "eliotstocker"
		}
		_ = s.repo.SaveUserVCSToken(r.Context(), "gitlab", username, tokenData.AccessToken, tokenData.RefreshToken)
	}

	// 4. Return JSON or HTML confirmation page
	if strings.Contains(r.Header.Get("Accept"), "application/json") || r.URL.Query().Get("json") == "true" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      true,
			"access_token": tokenData.AccessToken,
			"scope":        tokenData.Scope,
			"user":         glUser,
		})
		return
	}

	displayName := glUser.Name
	if displayName == "" {
		displayName = glUser.Username
	}
	if displayName == "" {
		displayName = "GitLab User"
	}

	usernameDisplay := glUser.Username
	if usernameDisplay == "" {
		usernameDisplay = "user"
	}

	avatarHTML := ""
	if glUser.Avatar != "" {
		avatarHTML = fmt.Sprintf(`<img src="%s" class="avatar" alt="Avatar">`, template.HTMLEscapeString(glUser.Avatar))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>GitLab Connected - MetaStackR</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background: #0d1117; color: #c9d1d9; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; }
    .card { background: #161b22; border: 1px solid #30363d; border-radius: 12px; padding: 40px; max-width: 500px; text-align: center; box-shadow: 0 10px 30px rgba(0,0,0,0.5); }
    h1 { color: #58a6ff; font-size: 24px; margin-top: 0; }
    .avatar { width: 64px; height: 64px; border-radius: 50%%; border: 2px solid #fc6d26; margin-bottom: 16px; }
    .token-box { background: #0d1117; border: 1px dashed #30363d; padding: 12px; border-radius: 6px; font-family: monospace; font-size: 13px; color: #79c0ff; word-break: break-all; margin: 20px 0; text-align: left; }
    .btn { display: inline-block; background: #238636; color: #fff; text-decoration: none; padding: 10px 20px; border-radius: 6px; font-weight: 600; margin-top: 16px; }
    .btn:hover { background: #2ea043; }
  </style>
</head>
<body>
  <div class="card">
    <div style="font-size: 48px; margin-bottom: 16px;">🦊 ✅</div>
    <h1>GitLab Connected to MetaStackR</h1>
    %s
    <p>Welcome, <strong>%s</strong> (@%s)! Your GitLab account has been authorized.</p>
    <p style="color: #8b949e; font-size: 14px;">Use the access token below to configure <code>git-meta</code> CLI or set it as your <code>VCS_TOKEN</code>:</p>
    <div class="token-box">
      <strong>Access Token:</strong><br>%s
    </div>
    <p style="font-size: 13px; color: #8b949e;">Run in terminal: <code>git meta config vcs-token %s</code></p>
    <a href="https://metastac.kr" class="btn">Return to MetaStackR</a>
  </div>
</body>
</html>`,
		avatarHTML,
		template.HTMLEscapeString(displayName),
		template.HTMLEscapeString(usernameDisplay),
		template.HTMLEscapeString(tokenData.AccessToken),
		template.HTMLEscapeString(tokenData.AccessToken),
	)
	_, _ = w.Write([]byte(html))
}


