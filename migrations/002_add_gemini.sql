-- Add 'gemini' to cli_type CHECK constraint.
-- SQLite doesn't support ALTER TABLE to modify constraints,
-- so we recreate the table.
CREATE TABLE IF NOT EXISTS sessions_new (
    id TEXT PRIMARY KEY,
    repo_id INTEGER NOT NULL REFERENCES repositories(id),
    worktree_path TEXT NOT NULL,
    branch TEXT NOT NULL,
    cli_type TEXT NOT NULL CHECK(cli_type IN ('claude', 'codex', 'gemini')),
    status TEXT NOT NULL DEFAULT 'starting',
    pid INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO sessions_new SELECT * FROM sessions;
DROP TABLE sessions;
ALTER TABLE sessions_new RENAME TO sessions;
