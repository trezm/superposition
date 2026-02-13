package shepherd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

const replayBufSize = 100 * 1024 // 100KB

// session is a PTY session owned by the shepherd.
type session struct {
	id   string
	cmd  *exec.Cmd
	ptmx *os.File
	done chan struct{}

	mu      sync.Mutex
	stopped bool

	replayMu  sync.Mutex
	replayBuf []byte

	subMu       sync.Mutex
	subscribers map[chan []byte]struct{}
}

func (s *session) appendReplay(data []byte) {
	s.replayMu.Lock()
	defer s.replayMu.Unlock()
	s.replayBuf = append(s.replayBuf, data...)
	if len(s.replayBuf) > replayBufSize {
		s.replayBuf = s.replayBuf[len(s.replayBuf)-replayBufSize:]
	}
}

func (s *session) getReplay() []byte {
	s.replayMu.Lock()
	defer s.replayMu.Unlock()
	cp := make([]byte, len(s.replayBuf))
	copy(cp, s.replayBuf)
	return cp
}

func (s *session) subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 256)
	s.subMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subMu.Unlock()

	unsub := func() {
		s.subMu.Lock()
		delete(s.subscribers, ch)
		s.subMu.Unlock()
	}
	return ch, unsub
}

func (s *session) broadcast(data []byte) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for ch := range s.subscribers {
		select {
		case ch <- data:
		default:
		}
	}
}

// connWriter wraps a net.Conn with a mutex for safe concurrent writes.
type connWriter struct {
	conn net.Conn
	mu   sync.Mutex
}

func (cw *connWriter) writeControl(msg any) error {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	return writeControl(cw.conn, msg)
}

func (cw *connWriter) writeDataFrame(frameType byte, sessionID string, data []byte) error {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	return writeDataFrame(cw.conn, frameType, sessionID, data)
}

// Shepherd is the long-lived process that owns PTY sessions.
type Shepherd struct {
	socketPath string
	pidPath    string

	mu       sync.RWMutex
	sessions map[string]*session

	// Connected clients that receive exit notifications
	clientMu sync.Mutex
	clients  map[*connWriter]struct{}
}

// SocketPath returns the path to the shepherd's Unix domain socket.
func SocketPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".superposition", "shepherd.sock"), nil
}

// PIDPath returns the path to the shepherd's PID file.
func PIDPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".superposition", "shepherd.pid"), nil
}

// Run starts the shepherd process. It blocks until the shepherd is shut down.
func Run() error {
	socketPath, err := SocketPath()
	if err != nil {
		return fmt.Errorf("socket path: %w", err)
	}
	pidPath, err := PIDPath()
	if err != nil {
		return fmt.Errorf("pid path: %w", err)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}

	// Clean up stale socket
	if err := cleanStaleSocket(socketPath, pidPath); err != nil {
		return fmt.Errorf("clean stale socket: %w", err)
	}

	s := &Shepherd{
		socketPath: socketPath,
		pidPath:    pidPath,
		sessions:   make(map[string]*session),
		clients:    make(map[*connWriter]struct{}),
	}

	// Write PID file
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shepherd: shutting down...")
		listener.Close()
		s.stopAll()
		os.Remove(socketPath)
		os.Remove(pidPath)
		os.Exit(0)
	}()

	log.Printf("shepherd: listening on %s (pid %d)", socketPath, os.Getpid())

	for {
		conn, err := listener.Accept()
		if err != nil {
			return nil // listener closed
		}
		go s.handleConn(conn)
	}
}

func (s *Shepherd) handleConn(conn net.Conn) {
	cw := &connWriter{conn: conn}

	s.clientMu.Lock()
	s.clients[cw] = struct{}{}
	s.clientMu.Unlock()

	defer func() {
		s.clientMu.Lock()
		delete(s.clients, cw)
		s.clientMu.Unlock()
		conn.Close()
	}()

	reader := bufio.NewReader(conn)
	for {
		frameType, payload, err := readFrame(reader)
		if err != nil {
			return // connection closed
		}

		switch frameType {
		case frameControl:
			s.handleControl(cw, payload)
		case frameInput:
			s.handleInput(payload)
		}
	}
}

func (s *Shepherd) handleControl(cw *connWriter, payload []byte) {
	var req Request
	if err := json.Unmarshal(payload, &req); err != nil {
		log.Printf("shepherd: bad control message: %v", err)
		return
	}

	switch req.Command {
	case cmdPing:
		s.sendResponse(cw, Response{ID: req.ID, Event: evtPong})

	case cmdStart:
		s.handleStart(cw, req)

	case cmdStop:
		s.handleStop(cw, req)

	case cmdResize:
		s.handleResize(cw, req)

	case cmdReplay:
		s.handleReplay(cw, req)

	case cmdSubscribe:
		s.handleSubscribe(cw, req)

	case cmdList:
		s.handleList(cw, req)

	case cmdStopAll:
		s.stopAll()
		s.sendResponse(cw, Response{ID: req.ID, Event: evtStopDone})
	}
}

func (s *Shepherd) handleStart(cw *connWriter, req Request) {
	cmd := exec.Command(req.CLIType)
	cmd.Dir = req.WorkDir
	cmd.Env = os.Environ()

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		s.sendResponse(cw, Response{ID: req.ID, Event: evtError, Error: err.Error()})
		return
	}

	sess := &session{
		id:          req.SessionID,
		cmd:         cmd,
		ptmx:        ptmx,
		done:        make(chan struct{}),
		subscribers: make(map[chan []byte]struct{}),
	}

	// Read PTY output → replay buffer + subscribers
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				sess.appendReplay(data)
				sess.broadcast(data)
			}
			if err != nil {
				break
			}
		}
		sess.subMu.Lock()
		for ch := range sess.subscribers {
			close(ch)
			delete(sess.subscribers, ch)
		}
		sess.subMu.Unlock()
	}()

	// Monitor process exit
	go func() {
		cmd.Wait()
		sess.mu.Lock()
		sess.stopped = true
		sess.mu.Unlock()
		close(sess.done)

		// Notify all connected clients
		s.broadcastExit(req.SessionID)

		// Remove from sessions map
		s.mu.Lock()
		delete(s.sessions, req.SessionID)
		s.mu.Unlock()
	}()

	s.mu.Lock()
	s.sessions[req.SessionID] = sess
	s.mu.Unlock()

	s.sendResponse(cw, Response{
		ID:        req.ID,
		Event:     evtStarted,
		SessionID: req.SessionID,
		PID:       cmd.Process.Pid,
	})
}

func (s *Shepherd) handleStop(cw *connWriter, req Request) {
	s.mu.Lock()
	sess, ok := s.sessions[req.SessionID]
	if !ok {
		s.mu.Unlock()
		s.sendResponse(cw, Response{ID: req.ID, Event: evtStopDone})
		return
	}
	delete(s.sessions, req.SessionID)
	s.mu.Unlock()

	sess.mu.Lock()
	if !sess.stopped {
		if sess.cmd.Process != nil {
			sess.cmd.Process.Signal(syscall.SIGTERM)
		}
		sess.ptmx.Close()
	}
	sess.mu.Unlock()

	s.sendResponse(cw, Response{ID: req.ID, Event: evtStopDone})
}

func (s *Shepherd) handleResize(cw *connWriter, req Request) {
	s.mu.RLock()
	sess, ok := s.sessions[req.SessionID]
	s.mu.RUnlock()
	if !ok {
		s.sendResponse(cw, Response{ID: req.ID, Event: evtError, Error: "session not found"})
		return
	}
	if err := pty.Setsize(sess.ptmx, &pty.Winsize{Rows: req.Rows, Cols: req.Cols}); err != nil {
		s.sendResponse(cw, Response{ID: req.ID, Event: evtError, Error: err.Error()})
		return
	}
	s.sendResponse(cw, Response{ID: req.ID, Event: "resized"})
}

func (s *Shepherd) handleReplay(cw *connWriter, req Request) {
	s.mu.RLock()
	sess, ok := s.sessions[req.SessionID]
	s.mu.RUnlock()
	if !ok {
		s.sendResponse(cw, Response{ID: req.ID, Event: evtError, Error: "session not found"})
		return
	}
	replay := sess.getReplay()
	s.sendResponse(cw, Response{
		ID:        req.ID,
		Event:     evtReplay,
		SessionID: req.SessionID,
		Data:      replay,
	})
}

func (s *Shepherd) handleSubscribe(cw *connWriter, req Request) {
	s.mu.RLock()
	sess, ok := s.sessions[req.SessionID]
	s.mu.RUnlock()
	if !ok {
		s.sendResponse(cw, Response{ID: req.ID, Event: evtError, Error: "session not found"})
		return
	}

	// Acknowledge subscription
	s.sendResponse(cw, Response{ID: req.ID, Event: "subscribed", SessionID: req.SessionID})

	// Forward PTY output to this client via data frames
	ch, unsub := sess.subscribe()
	go func() {
		defer unsub()
		for data := range ch {
			if err := cw.writeDataFrame(frameData, req.SessionID, data); err != nil {
				return
			}
		}
	}()
}

func (s *Shepherd) handleList(cw *connWriter, req Request) {
	s.mu.RLock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	s.sendResponse(cw, Response{ID: req.ID, Event: evtList, Sessions: ids})
}

func (s *Shepherd) handleInput(payload []byte) {
	sessionID, data, err := parseDataPayload(payload)
	if err != nil {
		return
	}
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	sess.ptmx.Write(data)
}

func (s *Shepherd) broadcastExit(sessionID string) {
	resp := Response{Event: evtExited, SessionID: sessionID}
	s.clientMu.Lock()
	clients := make([]*connWriter, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.clientMu.Unlock()

	for _, c := range clients {
		c.writeControl(resp)
	}
}

func (s *Shepherd) stopAll() {
	s.mu.Lock()
	sessions := make([]*session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.sessions = make(map[string]*session)
	s.mu.Unlock()

	for _, sess := range sessions {
		sess.mu.Lock()
		if !sess.stopped {
			if sess.cmd.Process != nil {
				sess.cmd.Process.Signal(syscall.SIGTERM)
			}
			sess.ptmx.Close()
		}
		sess.mu.Unlock()
	}
}

func (s *Shepherd) sendResponse(cw *connWriter, resp Response) {
	cw.writeControl(resp)
}

// cleanStaleSocket removes a stale socket file if the shepherd process is not running.
func cleanStaleSocket(socketPath, pidPath string) error {
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return nil
	}

	// Try to connect to see if it's alive
	conn, err := net.Dial("unix", socketPath)
	if err == nil {
		conn.Close()
		return fmt.Errorf("shepherd already running (socket active)")
	}

	// Socket exists but can't connect — check PID file
	pidData, err := os.ReadFile(pidPath)
	if err == nil {
		pid, err := strconv.Atoi(string(pidData))
		if err == nil {
			// Check if process is alive
			proc, err := os.FindProcess(pid)
			if err == nil {
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					return fmt.Errorf("shepherd already running (pid %d)", pid)
				}
			}
		}
	}

	// Stale socket/pid — clean up
	log.Printf("shepherd: removing stale socket %s", socketPath)
	os.Remove(socketPath)
	os.Remove(pidPath)
	return nil
}
