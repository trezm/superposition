package pty

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

const replayBufSize = 100 * 1024 // 100KB replay buffer

type Session struct {
	ID  string
	Cmd *exec.Cmd
	PTY *os.File

	done chan struct{}

	mu      sync.Mutex
	stopped bool

	// Replay buffer for reconnection
	replayMu  sync.Mutex
	replayBuf []byte

	// Subscribers for fan-out PTY output
	subMu       sync.Mutex
	subscribers map[chan []byte]struct{}
}

// Write sends data to the PTY.
func (s *Session) Write(data []byte) (int, error) {
	return s.PTY.Write(data)
}

// Done returns a channel that is closed when the session process exits.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

func (s *Session) appendReplay(data []byte) {
	s.replayMu.Lock()
	defer s.replayMu.Unlock()
	s.replayBuf = append(s.replayBuf, data...)
	if len(s.replayBuf) > replayBufSize {
		s.replayBuf = s.replayBuf[len(s.replayBuf)-replayBufSize:]
	}
}

func (s *Session) Replay() []byte {
	s.replayMu.Lock()
	defer s.replayMu.Unlock()
	cp := make([]byte, len(s.replayBuf))
	copy(cp, s.replayBuf)
	return cp
}

func (s *Session) broadcast(data []byte) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for ch := range s.subscribers {
		select {
		case ch <- data:
		default:
			// Slow subscriber, drop data
		}
	}
}

// Subscribe returns a channel of PTY output and an unsubscribe function.
func (s *Session) Subscribe() (<-chan []byte, func()) {
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

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

func (m *Manager) Start(id, cliType, workDir string) (SessionHandle, int, error) {
	cmd := exec.Command(cliType)
	cmd.Dir = workDir
	cmd.Env = os.Environ()

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		return nil, 0, fmt.Errorf("start pty: %w", err)
	}

	sess := &Session{
		ID:          id,
		Cmd:         cmd,
		PTY:         ptmx,
		done:        make(chan struct{}),
		subscribers: make(map[chan []byte]struct{}),
	}

	// Read from PTY, fan out to replay buffer + subscribers
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
		// Close all subscriber channels
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
	}()

	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()

	pid := cmd.Process.Pid
	return sess, pid, nil
}

func (m *Manager) Get(id string) SessionHandle {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sess := m.sessions[id]
	if sess == nil {
		return nil
	}
	return sess
}

func (m *Manager) getSession(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.sessions, id)
	m.mu.Unlock()

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.stopped {
		return nil
	}

	if sess.Cmd.Process != nil {
		sess.Cmd.Process.Signal(syscall.SIGTERM)
	}
	sess.PTY.Close()
	return nil
}

func (m *Manager) Resize(id string, rows, cols uint16) error {
	sess := m.getSession(id)
	if sess == nil {
		return fmt.Errorf("session not found: %s", id)
	}
	return pty.Setsize(sess.PTY, &pty.Winsize{Rows: rows, Cols: cols})
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		m.Stop(id)
	}
}

func (m *Manager) ListActive() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}
