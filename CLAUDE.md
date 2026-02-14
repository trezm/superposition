# Superposition

AI coding session manager — bare-clones repos, creates isolated git worktrees, and spawns CLI processes (Claude Code, Codex, Gemini) with PTY streams to the browser.

## Build

```bash
# Full build (frontend + Go binary)
make build

# Frontend only
cd web && npm install && npm run build

# Go binary only (requires web/dist to exist)
go build .
```

## Lint / Check (run before pushing)

```bash
# Go
go vet ./...

# Web (from web/ directory)
cd web && npx eslint src --max-warnings=0
cd web && npx tsc --noEmit
cd web && npx prettier --check "src/**/*.{ts,tsx,css}"

# Fix Prettier formatting
cd web && npx prettier --write "src/**/*.{ts,tsx,css}"
```

## Architecture

- **Backend:** Go with embedded React SPA, SQLite, WebSocket terminal I/O
- **Frontend:** React 19, TypeScript, Tailwind CSS v4, xterm.js
- **Data dir:** `~/.superposition/` (repos, worktrees, DB, shepherd socket)
- **Bare clones:** `~/.superposition/repos/{owner}/{name}.git` (GitHub) or `~/.superposition/repos/local/{name}.git` (local)
- **Worktrees:** `~/.superposition/worktrees/{uuid}`

## Key directories

- `migrations/` — SQLite migrations (run sequentially in main.go, use CREATE TABLE IF NOT EXISTS + copy-drop-rename pattern for ALTERs)
- `internal/api/` — REST handlers
- `internal/git/` — bare clone, fetch, worktree operations
- `internal/models/` — data structs
- `internal/shepherd/` — long-lived PTY process manager (survives server restarts)
- `web/src/lib/api.ts` — frontend API client
- `web/src/pages/` — page components
- `web/src/components/` — shared components (NewSessionModal, Terminal, Layout)

## Conventions

- Commit messages: imperative mood, short summary line
- Go: standard library style, no frameworks beyond gorilla/websocket
- Frontend: functional components, Tailwind for styling, no CSS modules
- Migrations: numbered `NNN_description.sql`, registered manually in `main.go`
