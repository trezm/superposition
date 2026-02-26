package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/peterje/superposition/internal/git"
	"github.com/peterje/superposition/internal/models"
	ptymgr "github.com/peterje/superposition/internal/pty"
)

type SessionsHandler struct {
	db      *sql.DB
	manager ptymgr.SessionManager
}

func NewSessionsHandler(db *sql.DB, manager ptymgr.SessionManager) *SessionsHandler {
	return &SessionsHandler{db: db, manager: manager}
}

func (h *SessionsHandler) HandleList(w http.ResponseWriter, _ *http.Request) {
	rows, err := h.db.Query(`SELECT s.id, s.repo_id, s.worktree_path, s.branch, s.cli_type, s.status, s.pid, s.created_at,
		s.source_branch, s.base_commit, r.owner, r.name FROM sessions s JOIN repositories r ON s.repo_id = r.id ORDER BY s.created_at DESC`)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type sessionWithRepo struct {
		models.Session
		RepoOwner string `json:"repo_owner"`
		RepoName  string `json:"repo_name"`
	}

	sessions := []sessionWithRepo{}
	for rows.Next() {
		var s sessionWithRepo
		if err := rows.Scan(&s.ID, &s.RepoID, &s.WorktreePath, &s.Branch, &s.CLIType, &s.Status, &s.PID, &s.CreatedAt,
			&s.SourceBranch, &s.BaseCommit, &s.RepoOwner, &s.RepoName); err != nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sessions = append(sessions, s)
	}
	WriteJSON(w, http.StatusOK, sessions)
}

func (h *SessionsHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RepoID       int64  `json:"repo_id"`
		SourceBranch string `json:"source_branch"`
		NewBranch    string `json:"new_branch"`
		CLIType      string `json:"cli_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if body.CLIType != "claude" && body.CLIType != "codex" && body.CLIType != "gemini" {
		WriteError(w, http.StatusBadRequest, "cli_type must be 'claude', 'codex', or 'gemini'")
		return
	}
	if body.SourceBranch == "" {
		WriteError(w, http.StatusBadRequest, "source_branch is required")
		return
	}
	if body.NewBranch == "" {
		WriteError(w, http.StatusBadRequest, "new_branch is required")
		return
	}

	// Get repo info
	var repo models.Repository
	err := h.db.QueryRow(`SELECT id, local_path, clone_status FROM repositories WHERE id = ?`, body.RepoID).
		Scan(&repo.ID, &repo.LocalPath, &repo.CloneStatus)
	if err == sql.ErrNoRows {
		WriteError(w, http.StatusNotFound, "repository not found")
		return
	}
	if repo.CloneStatus != "ready" {
		WriteError(w, http.StatusBadRequest, "repository not ready")
		return
	}

	// Create worktree
	sessionID := uuid.New().String()[:8]
	wtDir, err := git.WorktreesDir()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	worktreePath := filepath.Join(wtDir, sessionID)

	if err := git.AddWorktree(repo.LocalPath, worktreePath, body.NewBranch, body.SourceBranch); err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("create worktree: %v", err))
		return
	}

	// Resolve the base commit SHA for diff support
	baseCommit, _ := git.ResolveCommit(worktreePath, "HEAD")

	// Resolve CLI command (may include args from settings override)
	command := resolveCommand(h.db, body.CLIType)

	// Start PTY
	sess, pid, err := h.manager.Start(sessionID, command, worktreePath)
	if err != nil {
		git.RemoveWorktree(repo.LocalPath, worktreePath)
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("start session: %v", err))
		return
	}

	now := time.Now()
	h.db.Exec(`INSERT INTO sessions (id, repo_id, worktree_path, branch, cli_type, status, pid, created_at, source_branch, base_commit)
		VALUES (?, ?, ?, ?, ?, 'running', ?, ?, ?, ?)`,
		sessionID, body.RepoID, worktreePath, body.NewBranch, body.CLIType, pid, now, body.SourceBranch, baseCommit)

	// Monitor for process exit and update DB
	go func() {
		<-sess.Done()
		h.db.Exec(`UPDATE sessions SET status = 'stopped' WHERE id = ?`, sessionID)
		log.Printf("Session %s stopped", sessionID)
	}()

	WriteJSON(w, http.StatusCreated, models.Session{
		ID:           sessionID,
		RepoID:       body.RepoID,
		WorktreePath: worktreePath,
		Branch:       body.NewBranch,
		CLIType:      body.CLIType,
		Status:       "running",
		PID:          &pid,
		CreatedAt:    now,
		SourceBranch: body.SourceBranch,
		BaseCommit:   baseCommit,
	})
}

func (h *SessionsHandler) HandleReplay(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := h.manager.Get(id)
	if sess == nil {
		WriteError(w, http.StatusNotFound, "session not found or not running")
		return
	}
	replay := sess.Replay()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	w.Write(replay)
}

func (h *SessionsHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	deleteLocal := true
	if raw := r.URL.Query().Get("delete_local"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "delete_local must be true or false")
			return
		}
		deleteLocal = parsed
	}

	var worktreePath string
	var branch string
	var repoID int64
	err := h.db.QueryRow(`SELECT worktree_path, repo_id, branch FROM sessions WHERE id = ?`, id).
		Scan(&worktreePath, &repoID, &branch)
	if err == sql.ErrNoRows {
		WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Stop PTY if still running
	h.manager.Stop(id)

	if deleteLocal {
		var localPath string
		h.db.QueryRow(`SELECT local_path FROM repositories WHERE id = ?`, repoID).Scan(&localPath)
		if localPath != "" {
			if worktreePath != "" {
				if err := git.RemoveWorktree(localPath, worktreePath); err != nil {
					log.Printf("Failed to remove worktree %s: %v", worktreePath, err)
				}
			}
			if branch != "" {
				if err := git.RemoveBranch(localPath, branch); err != nil {
					log.Printf("Failed to remove branch %s: %v", branch, err)
				}
			}
		}
	}

	// Delete the session row
	h.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *SessionsHandler) HandleDiff(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var worktreePath, baseCommit, sourceBranch string
	var repoID int64
	err := h.db.QueryRow(`SELECT worktree_path, base_commit, source_branch, repo_id FROM sessions WHERE id = ?`, id).
		Scan(&worktreePath, &baseCommit, &sourceBranch, &repoID)
	if err == sql.ErrNoRows {
		WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// For sessions created before base_commit was tracked, try to compute it
	if baseCommit == "" {
		baseCommit = inferBaseCommit(h.db, worktreePath, sourceBranch, repoID)
		if baseCommit == "" {
			WriteJSON(w, http.StatusOK, git.DiffResult{})
			return
		}
		// Backfill so we don't recompute next time
		h.db.Exec(`UPDATE sessions SET base_commit = ? WHERE id = ?`, baseCommit, id)
	}

	diff, err := git.Diff(worktreePath, baseCommit)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("diff: %v", err))
		return
	}
	WriteJSON(w, http.StatusOK, diff)
}

// inferBaseCommit tries to determine the base commit for a session that
// was created before base_commit tracking was added. It uses git merge-base
// to find where the worktree branch diverged from the source branch.
func inferBaseCommit(db *sql.DB, worktreePath, sourceBranch string, repoID int64) string {
	// Try source_branch first, fall back to repo's default branch
	ref := sourceBranch
	if ref == "" {
		var defaultBranch string
		db.QueryRow(`SELECT default_branch FROM repositories WHERE id = ?`, repoID).Scan(&defaultBranch)
		if defaultBranch != "" {
			ref = defaultBranch
		} else {
			ref = "main"
		}
	}

	// Try merge-base with origin/<ref>
	if commit, err := git.MergeBase(worktreePath, "HEAD", "origin/"+ref); err == nil {
		return commit
	}
	// Try without origin/ prefix
	if commit, err := git.MergeBase(worktreePath, "HEAD", ref); err == nil {
		return commit
	}
	return ""
}

// resolveCommand returns the override command string for a CLI type if one
// exists in settings, otherwise returns the bare CLI type name.
func resolveCommand(db *sql.DB, cliType string) string {
	var val string
	err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, "cli_command."+cliType).Scan(&val)
	if err == nil && val != "" {
		return val
	}
	return cliType
}
