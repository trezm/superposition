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
		r.owner, r.name FROM sessions s JOIN repositories r ON s.repo_id = r.id ORDER BY s.created_at DESC`)
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
		if err := rows.Scan(&s.ID, &s.RepoID, &s.WorktreePath, &s.Branch, &s.CLIType, &s.Status, &s.PID, &s.CreatedAt, &s.RepoOwner, &s.RepoName); err != nil {
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

	// Start PTY
	sess, pid, err := h.manager.Start(sessionID, body.CLIType, worktreePath)
	if err != nil {
		git.RemoveWorktree(repo.LocalPath, worktreePath)
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("start session: %v", err))
		return
	}

	now := time.Now()
	h.db.Exec(`INSERT INTO sessions (id, repo_id, worktree_path, branch, cli_type, status, pid, created_at)
		VALUES (?, ?, ?, ?, ?, 'running', ?, ?)`,
		sessionID, body.RepoID, worktreePath, body.NewBranch, body.CLIType, pid, now)

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
	})
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
