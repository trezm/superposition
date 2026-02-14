package main

import (
	"bufio"
	"context"
	"database/sql"
	"embed"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/peterje/superposition/internal/db"
	gitops "github.com/peterje/superposition/internal/git"
	"github.com/peterje/superposition/internal/preflight"
	ptymgr "github.com/peterje/superposition/internal/pty"
	"github.com/peterje/superposition/internal/server"
	"github.com/peterje/superposition/internal/shepherd"
	"github.com/peterje/superposition/web"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	// Subcommand dispatch: "superposition shepherd" runs the shepherd process
	if len(os.Args) > 1 && os.Args[1] == "shepherd" {
		if err := shepherd.Run(); err != nil {
			log.Fatalf("Shepherd failed: %v", err)
		}
		return
	}

	port := flag.Int("port", 8800, "server port")
	flag.Parse()

	fmt.Println("Superposition - AI Coding Sessions")
	fmt.Println("===================================")
	fmt.Println()

	// Preflight checks
	fmt.Println("Running preflight checks...")
	cliStatus, gitOk := preflight.CheckAll()
	if !gitOk {
		fmt.Println("\ngit is required. Please install git and try again.")
		os.Exit(1)
	}
	fmt.Println()

	// Open database
	database, err := db.Open()
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Run migrations
	migrationSQL, err := migrationsFS.ReadFile("migrations/001_initial.sql")
	if err != nil {
		log.Fatalf("Failed to read migrations: %v", err)
	}
	if err := db.Migrate(database, string(migrationSQL)); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	migration002, err := migrationsFS.ReadFile("migrations/002_add_gemini.sql")
	if err != nil {
		log.Fatalf("Failed to read migration 002: %v", err)
	}
	if err := db.Migrate(database, string(migration002)); err != nil {
		log.Fatalf("Failed to run migration 002: %v", err)
	}

	// Connect to or start the shepherd process
	var mgr ptymgr.SessionManager
	shepherdClient, err := connectOrStartShepherd()
	if err != nil {
		log.Printf("Shepherd unavailable, falling back to in-process PTY manager: %v", err)
		mgr = ptymgr.NewManager()
	} else {
		mgr = shepherdClient
	}

	// Reconcile DB with shepherd's active sessions
	reconcileSessions(database, mgr, shepherdClient)

	// Start server
	srv := server.New(database, cliStatus, gitOk, web.SPAHandler(), mgr)

	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: loggingMiddleware(recoveryMiddleware(srv)),
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		fmt.Printf("\nReceived %s, shutting down...\n", sig)

		// Do NOT stop PTY sessions — shepherd keeps them alive
		// Do NOT clean up worktrees for running sessions

		// Close shepherd client connection
		if shepherdClient != nil {
			shepherdClient.Close()
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(ctx)
	}()

	fmt.Printf("Server running at http://%s\n", addr)
	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
	fmt.Println("Server stopped.")
}

// connectOrStartShepherd connects to an existing shepherd or launches a new one.
func connectOrStartShepherd() (*shepherd.Client, error) {
	socketPath, err := shepherd.SocketPath()
	if err != nil {
		return nil, err
	}

	// Try connecting to existing shepherd
	client, err := shepherd.NewClient(socketPath)
	if err == nil {
		if err := client.Ping(); err == nil {
			log.Println("Connected to existing shepherd")
			return client, nil
		}
		client.Close()
	}

	// Launch a new shepherd process
	log.Println("Starting shepherd process...")
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("get executable path: %w", err)
	}

	cmd := exec.Command(exe, "shepherd")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start shepherd: %w", err)
	}
	// Detach — don't wait for the shepherd to exit
	cmd.Process.Release()

	// Wait for shepherd to become available
	for i := 0; i < 40; i++ { // 40 * 50ms = 2s
		time.Sleep(50 * time.Millisecond)
		client, err = shepherd.NewClient(socketPath)
		if err == nil {
			if err := client.Ping(); err == nil {
				log.Println("Shepherd started and connected")
				return client, nil
			}
			client.Close()
		}
	}

	return nil, fmt.Errorf("shepherd did not become available within 2s")
}

// reconcileSessions reconciles the database with the shepherd's active sessions.
// Sessions that are in the DB as "running" but not in the shepherd are marked "stopped".
// Sessions in the shepherd but not in the DB are left alone (they'll be adopted on reconnect).
func reconcileSessions(database *sql.DB, mgr ptymgr.SessionManager, client *shepherd.Client) {
	if client == nil {
		// No shepherd — mark all running sessions as stopped (old behavior)
		cleanupStaleSessions(database)
		return
	}

	activeIDs, err := client.ListSessions()
	if err != nil {
		log.Printf("Failed to list shepherd sessions: %v", err)
		cleanupStaleSessions(database)
		return
	}

	activeSet := make(map[string]struct{}, len(activeIDs))
	for _, id := range activeIDs {
		activeSet[id] = struct{}{}
	}

	// Get all running sessions from DB
	rows, err := database.Query(`SELECT id FROM sessions WHERE status IN ('running', 'starting')`)
	if err != nil {
		log.Printf("Failed to query sessions: %v", err)
		return
	}
	defer rows.Close()

	var orphanIDs []string
	var aliveIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		if _, alive := activeSet[id]; alive {
			aliveIDs = append(aliveIDs, id)
		} else {
			orphanIDs = append(orphanIDs, id)
		}
	}

	// Mark orphaned sessions as stopped
	for _, id := range orphanIDs {
		database.Exec(`UPDATE sessions SET status = 'stopped' WHERE id = ?`, id)
	}
	if len(orphanIDs) > 0 {
		log.Printf("Marked %d orphaned sessions as stopped", len(orphanIDs))
	}

	// Re-adopt alive sessions: register done channels so we get exit notifications
	for _, id := range aliveIDs {
		sessionID := id
		// Register the session in the client's done tracking
		_ = client.Get(sessionID)
		// Monitor for exit
		go func() {
			<-client.Done(sessionID)
			database.Exec(`UPDATE sessions SET status = 'stopped' WHERE id = ?`, sessionID)
			log.Printf("Session %s stopped (detected via shepherd)", sessionID)
		}()
	}
	if len(aliveIDs) > 0 {
		log.Printf("Re-adopted %d sessions from shepherd", len(aliveIDs))
	}

	// Clean up worktrees for stopped sessions
	cleanupWorktrees(database)
}

func cleanupStaleSessions(database *sql.DB) {
	result, err := database.Exec(`UPDATE sessions SET status = 'stopped' WHERE status IN ('running', 'starting')`)
	if err != nil {
		log.Printf("Failed to clean up stale sessions: %v", err)
		return
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		log.Printf("Cleaned up %d stale sessions", rows)
	}

	// Remove orphaned worktrees
	cleanupWorktrees(database)
}

func cleanupWorktrees(database *sql.DB) {
	rows, err := database.Query(`SELECT s.worktree_path, r.local_path FROM sessions s JOIN repositories r ON s.repo_id = r.id WHERE s.status = 'stopped' AND s.worktree_path != ''`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var wtPath, repoPath string
		if err := rows.Scan(&wtPath, &repoPath); err != nil {
			continue
		}
		if _, err := os.Stat(wtPath); err == nil {
			if err := gitops.RemoveWorktree(repoPath, wtPath); err != nil {
				log.Printf("Failed to remove worktree %s: %v", wtPath, err)
			}
		}
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)

		// Don't log WebSocket upgrades or static assets
		if r.Header.Get("Upgrade") == "websocket" {
			return
		}
		if r.URL.Path == "/" || (len(r.URL.Path) > 1 && r.URL.Path[1] != 'a') {
			return // Skip SPA/static
		}

		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.status, time.Since(start).Round(time.Millisecond))
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC: %s %s: %v", r.Method, r.URL.Path, err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Implement http.Hijacker so WebSocket upgrades work through the middleware.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}
