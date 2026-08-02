package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

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
	mux.HandleFunc("POST /webhooks/github", s.handleGitHubWebhook)
	mux.HandleFunc("GET /api/v1/prs/status", s.handlePRStatusQuery)
	mux.HandleFunc("POST /api/v1/prs/retry-merge", s.handleRetryMerge)
	mux.HandleFunc("POST /api/v1/repos/track", s.handleTrackRepo)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
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

	if !signatureVerified && !VerifySignature(s.webhookSecret, payload, sigHeader) {
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

func (s *Server) processNormalizedEvent(ctx context.Context, evt *NormalizedEvent) error {
	if s.repo == nil {
		return nil
	}

	// 1. Check if the repo is tracked as a Meta Repo
	tracked, err := s.repo.GetTrackedRepoByFullName(ctx, evt.Repo)
	if err == nil && tracked != nil {
		metaPR, err := s.repo.GetMetaPRByRepoAndNumber(ctx, evt.Repo, evt.PRNumber)
		if err == nil && metaPR != nil {
			if evt.EventType == EventTypePRMerged {
				_ = s.repo.UpdateMetaPRStatusWithLock(ctx, metaPR.ID, "MERGED", metaPR.LockVersion)
			} else if evt.EventType == EventTypePROpened {
				s.autoSynthesizeChildPRs(ctx, metaPR, evt)
			}
			_ = s.gh.UpdateMetaCheckRun(ctx, evt.Repo, evt.MergedSHA, metaPR)
		}
		return nil
	}

	// 2. Otherwise check if it is a child PR
	child, err := s.repo.GetChildPRByRepoAndNumber(ctx, evt.Repo, evt.PRNumber)
	if err == nil && child != nil {
		if evt.Merged {
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
			_ = s.gh.UpdateMetaCheckRun(ctx, evt.Repo, evt.MergedSHA, parentMeta)
			s.evaluateMetaPRReadiness(ctx, parentMeta)
		}
	}

	return nil
}

func (s *Server) autoSynthesizeChildPRs(ctx context.Context, metaPR *db.MetaPR, evt *NormalizedEvent) {
	log.Printf("[synthesis] Auto-synthesizing child PRs for Meta PR %d", metaPR.PRNumber)
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
	}
}

func (s *Server) evaluateMetaPRReadiness(ctx context.Context, metaPR *db.MetaPR) {
	if len(metaPR.ChildPRs) == 0 {
		return
	}

	allReady := true
	for _, child := range metaPR.ChildPRs {
		if child.Status != "MERGED" {
			allReady = false
			break
		}
	}

	if allReady && metaPR.Status == "OPEN" {
		log.Printf("[server] All child PRs merged for Meta PR #%d! Updating status to READY", metaPR.PRNumber)
		_ = s.repo.UpdateMetaPRStatusWithLock(ctx, metaPR.ID, "MERGING", metaPR.LockVersion)

		if s.reconcileFn != nil {
			go func() {
				_ = s.reconcileFn(context.Background(), metaPR.ID)
			}()
		}
	}
}

type PRStatusResponse struct {
	MetaPR *db.MetaPR `json:"meta_pr"`
}

func (s *Server) handlePRStatusQuery(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")
	branch := r.URL.Query().Get("branch")

	if repo == "" || branch == "" {
		http.Error(w, "missing required parameters repo or branch", http.StatusBadRequest)
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

	tracked, err := s.repo.GetTrackedRepoByFullName(r.Context(), repo)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"meta_pr": nil,
			"message": "Repo not tracked",
		})
		return
	}

	query := `
		SELECT id, meta_repo_id, pr_number, branch_name, base_branch, status, lock_version, created_at, updated_at
		FROM meta_prs
		WHERE meta_repo_id = $1 AND branch_name = $2
	`
	metaPR := &db.MetaPR{}
	err = s.repo.DB().QueryRowContext(r.Context(), query, tracked.ID, branch).Scan(
		&metaPR.ID, &metaPR.MetaRepoID, &metaPR.PRNumber, &metaPR.BranchName, &metaPR.BaseBranch, &metaPR.Status, &metaPR.LockVersion, &metaPR.CreatedAt, &metaPR.UpdatedAt,
	)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"meta_pr": nil,
			"message": "No active Meta PR found",
		})
		return
	}

	children, _ := s.repo.GetChildPRsByMetaPRID(r.Context(), metaPR.ID)
	metaPR.ChildPRs = children

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

	if metaPR.Status != "FAILED_PARTIAL" && metaPR.Status != "FAILED" {
		http.Error(w, fmt.Sprintf("Meta PR is in status %s; retry only allowed for FAILED or FAILED_PARTIAL", metaPR.Status), http.StatusBadRequest)
		return
	}

	err = s.repo.UpdateMetaPRStatusWithLock(r.Context(), metaPR.ID, "MERGING", metaPR.LockVersion)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to update status for retry: %v", err), http.StatusConflict)
		return
	}

	if s.reconcileFn != nil {
		go func() {
			_ = s.reconcileFn(context.Background(), metaPR.ID)
		}()
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"repo_id": tracked.ID.String(),
	})
}

