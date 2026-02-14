package gateway

import (
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/peterje/superposition/internal/tunnel"
)

var tunnelUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Tunnel manages the gateway-side of the reverse tunnel to superposition.
type Tunnel struct {
	secret  string
	mu      sync.RWMutex
	session *yamux.Session
}

func NewTunnel(secret string) *Tunnel {
	return &Tunnel{secret: secret}
}

// Handler returns the HTTP handler for the /tunnel endpoint.
func (t *Tunnel) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Validate pre-shared secret
		if r.Header.Get("X-Gateway-Secret") != t.secret {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		wsConn, err := tunnelUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("tunnel: upgrade failed: %v", err)
			return
		}

		log.Println("tunnel: superposition connected")

		// Close any existing session
		t.mu.Lock()
		if t.session != nil {
			t.session.Close()
			log.Println("tunnel: replaced existing connection")
		}
		t.mu.Unlock()

		// Gateway is the yamux client (opens streams to superposition)
		session, err := yamux.Client(tunnel.NewWSConn(wsConn), yamux.DefaultConfig())
		if err != nil {
			log.Printf("tunnel: yamux client: %v", err)
			wsConn.Close()
			return
		}

		t.mu.Lock()
		t.session = session
		t.mu.Unlock()

		// Block until the session closes
		<-session.CloseChan()

		t.mu.Lock()
		if t.session == session {
			t.session = nil
		}
		t.mu.Unlock()

		log.Println("tunnel: superposition disconnected")
	}
}

// OpenStream opens a new yamux stream to superposition.
// Returns nil, error if no tunnel is connected.
func (t *Tunnel) OpenStream() (net.Conn, error) {
	t.mu.RLock()
	session := t.session
	t.mu.RUnlock()

	if session == nil {
		return nil, errNoTunnel
	}

	return session.Open()
}

// Connected returns true if a superposition instance is connected.
func (t *Tunnel) Connected() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.session != nil && !t.session.IsClosed()
}
