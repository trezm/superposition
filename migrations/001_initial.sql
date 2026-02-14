CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS repositories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    github_url TEXT NOT NULL UNIQUE,
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    local_path TEXT NOT NULL,
    clone_status TEXT NOT NULL DEFAULT 'pending',
    default_branch TEXT NOT NULL DEFAULT 'main',
    last_synced DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    repo_id INTEGER NOT NULL REFERENCES repositories(id),
    worktree_path TEXT NOT NULL,
    branch TEXT NOT NULL,
    cli_type TEXT NOT NULL CHECK(cli_type IN ('claude', 'codex', 'gemini')),
    status TEXT NOT NULL DEFAULT 'starting',
    pid INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
