# Superposition

[![Go Quality](https://github.com/trezm/superposition/actions/workflows/go-quality.yml/badge.svg)](https://github.com/trezm/superposition/actions/workflows/go-quality.yml)
[![Web Quality](https://github.com/trezm/superposition/actions/workflows/web-quality.yml/badge.svg)](https://github.com/trezm/superposition/actions/workflows/web-quality.yml)

A web-based application for running AI coding sessions (Claude Code and Codex) against your GitHub repositories. Each session runs in an isolated git worktree with a full browser-based terminal.

![Dashboard](docs/screenshots/dashboard.png)

## Features

- **Multi-CLI Support** — Run sessions with Claude Code or Codex
- **Branch Isolation** — Each session gets its own git worktree, so parallel sessions never conflict
- **Browser Terminal** — Full xterm.js terminal with automatic reconnection and 100 KB replay buffer, plus virtual keyboard for mobile/touch devices
- **Session Persistence** — A background shepherd process keeps PTY sessions alive across server restarts, so deploys never kill a running session
- **Repository Management** — Clone and sync GitHub repos via Personal Access Token
- **Single Binary** — Compiles to a standalone Go binary with the React frontend embedded

## Screenshots

### Sessions

View and manage all running sessions. Each card shows the repo, branch, CLI type, and session ID.

![Sessions](docs/screenshots/sessions.png)

### New Session

Create a session by picking a repo, source branch, new branch name, and CLI.

![New Session](docs/screenshots/new-session.png)

### Terminal

Interact with Claude Code or Codex directly in the browser.

![Terminal](docs/screenshots/terminal.png)

### Repositories

Add GitHub repos and sync them to keep branches up to date.

![Repositories](docs/screenshots/repositories.png)

### Settings

Configure your GitHub Personal Access Token for repo access.

![Settings](docs/screenshots/settings.png)

## Prerequisites

- **Git** — required
- **Go 1.23+** — for building from source
- **Node.js / npm** — for building the frontend
- **Claude Code** and/or **Codex** CLI installed on your `PATH`

## Quick Start

```bash
# Clone the repo
git clone https://github.com/trezm/superposition.git
cd superposition

# Build the binary (compiles frontend + Go backend)
make build

# Run it
./superposition
```

Open [http://127.0.0.1:8800](http://127.0.0.1:8800) in your browser.

> **Note:** The server binds to `0.0.0.0`, so it's accessible from other machines on your network. If you're running on a public server, put it behind a reverse proxy with authentication or limit access via firewall rules.

### First-time setup

1. Go to **Settings** and enter your [GitHub Personal Access Token](https://github.com/settings/tokens) (classic token with `repo` scope)
2. Go to **Repositories** and add a repo from the list
3. Go to **Sessions**, click **New Session**, pick your repo and branch, and start coding

## Development

Run the backend and frontend separately for hot-reload:

```bash
# Terminal 1 — Go backend on :8800
make dev-backend

# Terminal 2 — Vite dev server on :5173 (proxies API to :8800)
make dev-frontend
```

## Production Build

```bash
make build
```

This produces a single `./superposition` binary with the React SPA embedded. No external files needed.

### CLI flags

```
-port int   server port (default 8800)
```

## Architecture

```
├── main.go                  # Entry point, server bootstrap
├── migrations/              # SQLite schema
├── internal/
│   ├── api/                 # REST handlers (repos, sessions, settings)
│   ├── db/                  # Database helpers
│   ├── git/                 # Git operations (clone, worktree, fetch)
│   ├── github/              # GitHub API client
│   ├── models/              # Data models
│   ├── pty/                 # PTY process management
│   ├── preflight/           # CLI dependency checks
│   ├── server/              # HTTP server + middleware
│   ├── shepherd/            # Long-lived PTY process manager (survives restarts)
│   └── ws/                  # WebSocket terminal streaming
└── web/                     # React frontend
    └── src/
        ├── components/      # Terminal, Layout, Modal, Toast
        └── pages/           # Dashboard, Repos, Sessions, Settings
```

**Backend:** Go with standard library routing (Go 1.22+), SQLite, gorilla/websocket, creack/pty

**Frontend:** React 19, React Router 7, xterm.js, Tailwind CSS, Vite

**Data:** Stored in `~/.superposition/`:

```
~/.superposition/
├── superposition.db         # SQLite database (settings, repos, sessions)
├── repos/                   # Bare git clones (owner/name.git)
├── worktrees/               # Active session worktrees (one per session)
├── shepherd.sock            # Unix socket for shepherd IPC
└── shepherd.pid             # Shepherd process ID
```

## Troubleshooting

| Problem | Fix |
|---------|-----|
| **Port already in use** | Another instance may be running. Kill it or use `-port <other>`. |
| **Stale shepherd socket** | If sessions won't start after a crash, remove `~/.superposition/shepherd.sock` and restart. |
| **Repos not loading** | Verify your GitHub PAT has `repo` scope and hasn't expired. |
| **Terminal blank on reconnect** | Refresh the page — the replay buffer will restore output. |

## License

[MIT](LICENSE)
