package tunnel

import (
	"io"
	"sync"

	"github.com/gorilla/websocket"
)

// WSConn adapts a gorilla/websocket.Conn to io.ReadWriteCloser
// so it can be used as the underlying transport for yamux.
type WSConn struct {
	conn *websocket.Conn
	mu   sync.Mutex // serializes writes
	buf  []byte     // leftover from partial reads
}

func NewWSConn(conn *websocket.Conn) *WSConn {
	return &WSConn{conn: conn}
}

func (w *WSConn) Read(p []byte) (int, error) {
	if len(w.buf) > 0 {
		n := copy(p, w.buf)
		w.buf = w.buf[n:]
		return n, nil
	}
	_, msg, err := w.conn.ReadMessage()
	if err != nil {
		return 0, err
	}
	n := copy(p, msg)
	if n < len(msg) {
		w.buf = msg[n:]
	}
	return n, nil
}

func (w *WSConn) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *WSConn) Close() error {
	return w.conn.Close()
}

var _ io.ReadWriteCloser = (*WSConn)(nil)
