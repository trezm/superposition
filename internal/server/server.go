package server

import (
	"database/sql"
	"net/http"

	"github.com/peterje/superposition/internal/api"
	"github.com/peterje/superposition/internal/models"
	ptymgr "github.com/peterje/superposition/internal/pty"
	"github.com/peterje/superposition/internal/ws"
)

type Server struct {
	mux       *http.ServeMux
	db        *sql.DB
	cliStatus []models.CLIStatus
	gitOk     bool
	PtyMgr    ptymgr.SessionManager
}

func New(db *sql.DB, cliStatus []models.CLIStatus, gitOk bool, spaHandler http.Handler, ptyMgr ptymgr.SessionManager) *Server {
	s := &Server{
		mux:       http.NewServeMux(),
		db:        db,
		cliStatus: cliStatus,
		gitOk:     gitOk,
		PtyMgr:    ptyMgr,
	}
	s.routes(spaHandler)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes(spaHandler http.Handler) {
	settings := api.NewSettingsHandler(s.db)
	repos := api.NewReposHandler(s.db)
	sessions := api.NewSessionsHandler(s.db, s.PtyMgr)
	wsHandler := ws.NewHandler(s.PtyMgr)

	// Health
	s.mux.HandleFunc("GET /api/health", s.handleHealth)

	// Settings
	s.mux.HandleFunc("GET /api/settings", settings.ServeHTTP)
	s.mux.HandleFunc("GET /api/settings/{key}", settings.ServeHTTP)
	s.mux.HandleFunc("PUT /api/settings/{key}", settings.ServeHTTP)
	s.mux.HandleFunc("DELETE /api/settings/{key}", settings.ServeHTTP)

	// GitHub
	s.mux.HandleFunc("GET /api/github/repos", repos.HandleGitHubRepos)

	// Repos
	s.mux.HandleFunc("GET /api/repos", repos.HandleList)
	s.mux.HandleFunc("POST /api/repos", repos.HandleCreate)
	s.mux.HandleFunc("DELETE /api/repos/{id}", repos.HandleDelete)
	s.mux.HandleFunc("POST /api/repos/{id}/sync", repos.HandleSync)
	s.mux.HandleFunc("GET /api/repos/{id}/branches", repos.HandleBranches)

	// Sessions
	s.mux.HandleFunc("GET /api/sessions", sessions.HandleList)
	s.mux.HandleFunc("POST /api/sessions", sessions.HandleCreate)
	s.mux.HandleFunc("GET /api/sessions/{id}/replay", sessions.HandleReplay)
	s.mux.HandleFunc("DELETE /api/sessions/{id}", sessions.HandleDelete)

	// WebSocket
	s.mux.Handle("GET /ws/session/{id}", wsHandler)

	// SPA fallback
	s.mux.Handle("/", spaHandler)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	resp := models.HealthResponse{
		Status: "ok",
		CLIs:   s.cliStatus,
		Git:    s.gitOk,
	}
	api.WriteJSON(w, http.StatusOK, resp)
}
