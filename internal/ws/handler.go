package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	ptymgr "github.com/peterje/superposition/internal/pty"
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

	// Send replay buffer first (for reconnection)
	replay := sess.Replay()
	if len(replay) > 0 {
		log.Printf("ws: sending %d bytes replay for session %s", len(replay), sessionID)
		if err := conn.WriteMessage(websocket.BinaryMessage, replay); err != nil {
			log.Printf("ws: replay send failed: %v", err)
			return
		}
	}

	// Subscribe to PTY output
	outputCh, unsub := sess.Subscribe()
	defer unsub()

	var wg sync.WaitGroup
	done := make(chan struct{})

	// PTY output -> WebSocket (via subscriber channel)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for data := range outputCh {
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
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
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session ended"))
	}

	wg.Wait()
	log.Printf("ws: handler finished for session %s", sessionID)
}
