-- Add support for local folder repos.
-- Makes github_url nullable, adds source_path and repo_type columns.
-- SQLite doesn't support ALTER TABLE to add constraints or make columns nullable,
-- so we recreate the table.
CREATE TABLE IF NOT EXISTS repositories_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    github_url TEXT UNIQUE,
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    local_path TEXT NOT NULL DEFAULT '',
    clone_status TEXT NOT NULL DEFAULT 'pending',
    default_branch TEXT NOT NULL DEFAULT 'main',
    last_synced DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    source_path TEXT UNIQUE,
    repo_type TEXT NOT NULL DEFAULT 'github'
);

INSERT OR IGNORE INTO repositories_new (id, github_url, owner, name, local_path, clone_status, default_branch, last_synced, created_at, repo_type)
    SELECT id, github_url, owner, name, local_path, clone_status, default_branch, last_synced, created_at, 'github' FROM repositories;
DROP TABLE repositories;
ALTER TABLE repositories_new RENAME TO repositories;
