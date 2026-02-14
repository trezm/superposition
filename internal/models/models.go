package models

import "time"

type Setting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Repository struct {
	ID            int64      `json:"id"`
	GitHubURL     string     `json:"github_url"`
	Owner         string     `json:"owner"`
	Name          string     `json:"name"`
	LocalPath     string     `json:"local_path"`
	CloneStatus   string     `json:"clone_status"`
	DefaultBranch string     `json:"default_branch"`
	LastSynced    *time.Time `json:"last_synced"`
	CreatedAt     time.Time  `json:"created_at"`
	SourcePath    *string    `json:"source_path"`
	RepoType      string     `json:"repo_type"`
}

type Session struct {
	ID           string    `json:"id"`
	RepoID       int64     `json:"repo_id"`
	WorktreePath string    `json:"worktree_path"`
	Branch       string    `json:"branch"`
	CLIType      string    `json:"cli_type"`
	Status       string    `json:"status"`
	PID          *int      `json:"pid"`
	CreatedAt    time.Time `json:"created_at"`
}

type CLIStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Authed    bool   `json:"authed"`
	Path      string `json:"path,omitempty"`
}

type HealthResponse struct {
	Status string      `json:"status"`
	CLIs   []CLIStatus `json:"clis"`
	Git    bool        `json:"git"`
}
