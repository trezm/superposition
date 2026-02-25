-- Add source_branch and base_commit to sessions for diff support.
-- SQLite doesn't support ADD COLUMN with constraints well, so use copy-drop-rename.

PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS sessions_new (
    id TEXT PRIMARY KEY,
    repo_id INTEGER NOT NULL REFERENCES repositories(id),
    worktree_path TEXT NOT NULL,
    branch TEXT NOT NULL,
    cli_type TEXT NOT NULL CHECK(cli_type IN ('claude', 'codex', 'gemini')),
    status TEXT NOT NULL DEFAULT 'starting',
    pid INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    source_branch TEXT NOT NULL DEFAULT '',
    base_commit TEXT NOT NULL DEFAULT ''
);

INSERT OR IGNORE INTO sessions_new (id, repo_id, worktree_path, branch, cli_type, status, pid, created_at)
    SELECT id, repo_id, worktree_path, branch, cli_type, status, pid, created_at FROM sessions;
DROP TABLE sessions;
ALTER TABLE sessions_new RENAME TO sessions;

PRAGMA foreign_keys = ON;
