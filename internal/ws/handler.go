package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	ptymgr "github.com/peterje/superposition/internal/pty"
)

const (
	pingInterval = 30 * time.Second
	pongWait     = 60 * time.Second
	writeWait    = 10 * time.Second
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type resizeMsg struct {
	Type string `json:"type"`
	Data struct {
		Rows uint16 `json:"rows"`
		Cols uint16 `json:"cols"`
	} `json:"data"`
}

type Handler struct {
	manager ptymgr.SessionManager
}

func NewHandler(manager ptymgr.SessionManager) *Handler {
	return &Handler{manager: manager}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	sess := h.manager.Get(sessionID)
	if sess == nil {
		log.Printf("ws: session %s not found in manager", sessionID)
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws: upgrade failed for %s: %v", sessionID, err)
		return
	}
	defer conn.Close()

	log.Printf("ws: client connected to session %s", sessionID)

	// Configure pong handler to extend read deadline on each pong
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Mutex to serialize writes (ping ticker + PTY output + close message)
	var mu sync.Mutex

	// Send replay buffer first (for reconnection)
	replay := sess.Replay()
	if len(replay) > 0 {
		log.Printf("ws: sending %d bytes replay for session %s", len(replay), sessionID)
		mu.Lock()
		conn.SetWriteDeadline(time.Now().Add(writeWait))
		err := conn.WriteMessage(websocket.BinaryMessage, replay)
		mu.Unlock()
		if err != nil {
			log.Printf("ws: replay send failed: %v", err)
			return
		}
	}

	// Subscribe to PTY output
	outputCh, unsub := sess.Subscribe()
	defer unsub()

	var wg sync.WaitGroup
	done := make(chan struct{})

	// Ping ticker to keep connection alive through proxies/gateways
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				err := conn.WriteMessage(websocket.PingMessage, nil)
				mu.Unlock()
				if err != nil {
					log.Printf("ws: ping failed for session %s: %v", sessionID, err)
					return
				}
			case <-done:
				return
			case <-sess.Done():
				return
			}
		}
	}()

	// PTY output -> WebSocket (via subscriber channel)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for data := range outputCh {
			mu.Lock()
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := conn.WriteMessage(websocket.BinaryMessage, data)
			mu.Unlock()
			if err != nil {
				log.Printf("ws: write to client failed: %v", err)
				return
			}
		}
		log.Printf("ws: output channel closed for session %s", sessionID)
	}()

	// WebSocket -> PTY (binary = input, text = control)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)
		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				log.Printf("ws: read from client failed: %v", err)
				return
			}
			switch msgType {
			case websocket.BinaryMessage:
				sess.Write(msg)
			case websocket.TextMessage:
				var resize resizeMsg
				if json.Unmarshal(msg, &resize) == nil && resize.Type == "resize" {
					h.manager.Resize(sessionID, resize.Data.Rows, resize.Data.Cols)
				}
			}
		}
	}()

	// Wait for session to end or WebSocket to close
	select {
	case <-done:
		log.Printf("ws: client disconnected from session %s", sessionID)
	case <-sess.Done():
		log.Printf("ws: session %s ended", sessionID)
		mu.Lock()
		conn.SetWriteDeadline(time.Now().Add(writeWait))
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session ended"))
		mu.Unlock()
	}

	wg.Wait()
	log.Printf("ws: handler finished for session %s", sessionID)
}
