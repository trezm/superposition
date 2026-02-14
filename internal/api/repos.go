package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/peterje/superposition/internal/git"
	"github.com/peterje/superposition/internal/github"
	"github.com/peterje/superposition/internal/models"
)

type ReposHandler struct {
	db          *sql.DB
	cachedRepos []github.Repo
	cachedAt    time.Time
}

func NewReposHandler(db *sql.DB) *ReposHandler {
	return &ReposHandler{db: db}
}

const repoCacheTTL = 5 * time.Minute

func (h *ReposHandler) HandleGitHubRepos(w http.ResponseWriter, r *http.Request) {
	pat := h.getPAT()
	if pat == "" {
		WriteError(w, http.StatusBadRequest, "GitHub PAT not configured")
		return
	}

	refresh := r.URL.Query().Get("refresh") == "true"

	// Fetch and cache all repos if cache is empty, stale, or refresh requested
	if h.cachedRepos == nil || refresh || time.Since(h.cachedAt) > repoCacheTTL {
		repos, err := github.ListAllRepos(pat)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		h.cachedRepos = repos
		h.cachedAt = time.Now()
	}

	// Filter by search query client-side
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if query == "" {
		WriteJSON(w, http.StatusOK, h.cachedRepos)
		return
	}

	filtered := []github.Repo{}
	for _, r := range h.cachedRepos {
		if strings.Contains(strings.ToLower(r.FullName), query) ||
			strings.Contains(strings.ToLower(r.Description), query) {
			filtered = append(filtered, r)
		}
	}
	WriteJSON(w, http.StatusOK, filtered)
}

func (h *ReposHandler) HandleList(w http.ResponseWriter, _ *http.Request) {
	rows, err := h.db.Query(`SELECT id, github_url, owner, name, local_path, clone_status, default_branch, last_synced, created_at, source_path, repo_type FROM repositories ORDER BY created_at DESC`)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	repos := []models.Repository{}
	for rows.Next() {
		var repo models.Repository
		var githubURL sql.NullString
		if err := rows.Scan(&repo.ID, &githubURL, &repo.Owner, &repo.Name, &repo.LocalPath, &repo.CloneStatus, &repo.DefaultBranch, &repo.LastSynced, &repo.CreatedAt, &repo.SourcePath, &repo.RepoType); err != nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		repo.GitHubURL = githubURL.String
		repos = append(repos, repo)
	}
	WriteJSON(w, http.StatusOK, repos)
}

func (h *ReposHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GitHubURL string `json:"github_url"`
		LocalPath string `json:"local_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if body.LocalPath != "" {
		h.createLocalRepo(w, body.LocalPath)
	} else if body.GitHubURL != "" {
		h.createGitHubRepo(w, body.GitHubURL)
	} else {
		WriteError(w, http.StatusBadRequest, "github_url or local_path is required")
	}
}

func (h *ReposHandler) createGitHubRepo(w http.ResponseWriter, githubURL string) {
	owner, name, err := parseGitHubURL(githubURL)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, name)

	result, err := h.db.Exec(
		`INSERT INTO repositories (github_url, owner, name, local_path, clone_status, default_branch, repo_type) VALUES (?, ?, ?, '', 'cloning', 'main', 'github')`,
		githubURL, owner, name,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			WriteError(w, http.StatusConflict, "repository already added")
			return
		}
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()
	go h.cloneRepo(id, cloneURL, owner, name)

	repo := models.Repository{
		ID:          id,
		GitHubURL:   githubURL,
		Owner:       owner,
		Name:        name,
		CloneStatus: "cloning",
		RepoType:    "github",
	}
	WriteJSON(w, http.StatusCreated, repo)
}

func (h *ReposHandler) createLocalRepo(w http.ResponseWriter, sourcePath string) {
	name := filepath.Base(sourcePath)
	if name == "" || name == "." || name == "/" {
		WriteError(w, http.StatusBadRequest, "invalid local path")
		return
	}

	result, err := h.db.Exec(
		`INSERT INTO repositories (owner, name, local_path, clone_status, default_branch, source_path, repo_type) VALUES ('local', ?, '', 'cloning', 'main', ?, 'local')`,
		name, sourcePath,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			WriteError(w, http.StatusConflict, "repository already added")
			return
		}
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()
	go h.cloneLocalRepo(id, sourcePath, name)

	repo := models.Repository{
		ID:          id,
		Owner:       "local",
		Name:        name,
		CloneStatus: "cloning",
		SourcePath:  &sourcePath,
		RepoType:    "local",
	}
	WriteJSON(w, http.StatusCreated, repo)
}

func (h *ReposHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	// Check for active sessions
	var count int
	h.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE repo_id = ? AND status NOT IN ('stopped', 'error')`, id).Scan(&count)
	if count > 0 {
		WriteError(w, http.StatusConflict, "repository has active sessions")
		return
	}

	result, err := h.db.Exec("DELETE FROM repositories WHERE id = ?", id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		WriteError(w, http.StatusNotFound, "repository not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ReposHandler) HandleSync(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var repo models.Repository
	err = h.db.QueryRow(`SELECT id, local_path, clone_status, repo_type FROM repositories WHERE id = ?`, id).
		Scan(&repo.ID, &repo.LocalPath, &repo.CloneStatus, &repo.RepoType)
	if err == sql.ErrNoRows {
		WriteError(w, http.StatusNotFound, "repository not found")
		return
	}
	if repo.CloneStatus != "ready" {
		WriteError(w, http.StatusBadRequest, "repository not ready")
		return
	}

	pat := ""
	if repo.RepoType != "local" {
		pat = h.getPAT()
	}
	if err := git.Fetch(repo.LocalPath, pat); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	now := time.Now()
	h.db.Exec(`UPDATE repositories SET last_synced = ? WHERE id = ?`, now, id)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}

func (h *ReposHandler) HandleBranches(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var localPath, cloneStatus string
	err = h.db.QueryRow(`SELECT local_path, clone_status FROM repositories WHERE id = ?`, id).
		Scan(&localPath, &cloneStatus)
	if err == sql.ErrNoRows {
		WriteError(w, http.StatusNotFound, "repository not found")
		return
	}
	if cloneStatus != "ready" {
		WriteError(w, http.StatusBadRequest, "repository not ready")
		return
	}

	branches, err := git.ListBranches(localPath)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, branches)
}

func (h *ReposHandler) cloneRepo(id int64, cloneURL, owner, name string) {
	pat := h.getPAT()
	localPath, err := git.CloneBare(cloneURL, pat, owner, name)
	if err != nil {
		log.Printf("Clone failed for %s/%s: %v", owner, name, err)
		h.db.Exec(`UPDATE repositories SET clone_status = 'error' WHERE id = ?`, id)
		return
	}

	// Get default branch
	defaultBranch := "main"
	branches, err := git.ListBranches(localPath)
	if err == nil && len(branches) > 0 {
		defaultBranch = branches[0]
	}

	now := time.Now()
	h.db.Exec(`UPDATE repositories SET local_path = ?, clone_status = 'ready', default_branch = ?, last_synced = ? WHERE id = ?`,
		localPath, defaultBranch, now, id)
	log.Printf("Cloned %s/%s to %s", owner, name, localPath)
}

func (h *ReposHandler) cloneLocalRepo(id int64, sourcePath, name string) {
	localPath, err := git.CloneBareLocal(sourcePath, name)
	if err != nil {
		log.Printf("Clone failed for local repo %s: %v", sourcePath, err)
		h.db.Exec(`UPDATE repositories SET clone_status = 'error' WHERE id = ?`, id)
		return
	}

	defaultBranch := "main"
	branches, err := git.ListBranches(localPath)
	if err == nil && len(branches) > 0 {
		defaultBranch = branches[0]
	}

	now := time.Now()
	h.db.Exec(`UPDATE repositories SET local_path = ?, clone_status = 'ready', default_branch = ?, last_synced = ? WHERE id = ?`,
		localPath, defaultBranch, now, id)
	log.Printf("Cloned local repo %s to %s", sourcePath, localPath)
}

func (h *ReposHandler) getPAT() string {
	var pat string
	h.db.QueryRow(`SELECT value FROM settings WHERE key = 'github_pat'`).Scan(&pat)
	return pat
}

func parseGitHubURL(rawURL string) (string, string, error) {
	// Handle formats: https://github.com/owner/name, github.com/owner/name, owner/name
	rawURL = strings.TrimSuffix(rawURL, ".git")
	rawURL = strings.TrimPrefix(rawURL, "https://")
	rawURL = strings.TrimPrefix(rawURL, "http://")
	rawURL = strings.TrimPrefix(rawURL, "github.com/")

	parts := strings.Split(rawURL, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid GitHub URL: need owner/name format")
	}
	return parts[len(parts)-2], parts[len(parts)-1], nil
}
