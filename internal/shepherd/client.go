package shepherd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"

	ptymgr "github.com/peterje/superposition/internal/pty"
)

// Client connects to the shepherd and implements ptymgr.SessionManager.
type Client struct {
	conn   net.Conn
	connMu sync.Mutex // serialize writes

	// Pending request-response correlation
	pendingMu sync.Mutex
	pending   map[string]chan Response

	// Per-session subscriber channels and done channels
	sessionMu      sync.Mutex
	sessionSubs    map[string][]chan []byte // PTY output subscribers per session
	sessionDone    map[string]chan struct{} // done channels per session
	shepherdSubbed map[string]bool         // true if cmdSubscribe already sent for this session

	reqCounter atomic.Uint64
	closed     chan struct{}
}

// NewClient connects to the shepherd at the given socket path.
func NewClient(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to shepherd: %w", err)
	}

	c := &Client{
		conn:           conn,
		pending:        make(map[string]chan Response),
		sessionSubs:    make(map[string][]chan []byte),
		sessionDone:    make(map[string]chan struct{}),
		shepherdSubbed: make(map[string]bool),
		closed:         make(chan struct{}),
	}

	go c.readLoop()
	return c, nil
}

// Close disconnects from the shepherd.
func (c *Client) Close() error {
	select {
	case <-c.closed:
		return nil
	default:
		close(c.closed)
	}
	return c.conn.Close()
}

// Ping checks if the shepherd is responsive.
func (c *Client) Ping() error {
	resp, err := c.sendRequest(Request{Command: cmdPing})
	if err != nil {
		return err
	}
	if resp.Event != evtPong {
		return fmt.Errorf("unexpected response: %s", resp.Event)
	}
	return nil
}

// ListSessions returns all active session IDs in the shepherd.
func (c *Client) ListSessions() ([]string, error) {
	resp, err := c.sendRequest(Request{Command: cmdList})
	if err != nil {
		return nil, err
	}
	return resp.Sessions, nil
}

// Start implements ptymgr.SessionManager.
func (c *Client) Start(id, cliType, workDir string) (ptymgr.SessionHandle, int, error) {
	// Pre-create done channel so we don't miss exit events
	c.sessionMu.Lock()
	c.sessionDone[id] = make(chan struct{})
	c.sessionMu.Unlock()

	resp, err := c.sendRequest(Request{
		Command:   cmdStart,
		SessionID: id,
		CLIType:   cliType,
		WorkDir:   workDir,
	})
	if err != nil {
		c.sessionMu.Lock()
		delete(c.sessionDone, id)
		c.sessionMu.Unlock()
		return nil, 0, err
	}
	if resp.Event == evtError {
		c.sessionMu.Lock()
		delete(c.sessionDone, id)
		c.sessionMu.Unlock()
		return nil, 0, fmt.Errorf("shepherd: %s", resp.Error)
	}

	handle := &ProxySession{
		client:    c,
		sessionID: id,
	}
	return handle, resp.PID, nil
}

// Stop implements ptymgr.SessionManager.
func (c *Client) Stop(id string) error {
	resp, err := c.sendRequest(Request{
		Command:   cmdStop,
		SessionID: id,
	})
	if err != nil {
		return err
	}
	if resp.Event == evtError {
		return fmt.Errorf("shepherd: %s", resp.Error)
	}

	// Clean up local state
	c.sessionMu.Lock()
	if done, ok := c.sessionDone[id]; ok {
		select {
		case <-done:
		default:
			close(done)
		}
		delete(c.sessionDone, id)
	}
	delete(c.sessionSubs, id)
	delete(c.shepherdSubbed, id)
	c.sessionMu.Unlock()

	return nil
}

// Get implements ptymgr.SessionManager.
func (c *Client) Get(id string) ptymgr.SessionHandle {
	c.sessionMu.Lock()
	_, exists := c.sessionDone[id]
	c.sessionMu.Unlock()

	if !exists {
		return nil
	}

	return &ProxySession{
		client:    c,
		sessionID: id,
	}
}

// Resize implements ptymgr.SessionManager.
func (c *Client) Resize(id string, rows, cols uint16) error {
	resp, err := c.sendRequest(Request{
		Command:   cmdResize,
		SessionID: id,
		Rows:      rows,
		Cols:      cols,
	})
	if err != nil {
		return err
	}
	if resp.Event == evtError {
		return fmt.Errorf("shepherd: %s", resp.Error)
	}
	return nil
}

// StopAll implements ptymgr.SessionManager.
func (c *Client) StopAll() {
	c.sendRequest(Request{Command: cmdStopAll})
}

func (c *Client) nextReqID() string {
	return fmt.Sprintf("r%d", c.reqCounter.Add(1))
}

func (c *Client) sendRequest(req Request) (Response, error) {
	req.ID = c.nextReqID()

	// Register pending response channel
	ch := make(chan Response, 1)
	c.pendingMu.Lock()
	c.pending[req.ID] = ch
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, req.ID)
		c.pendingMu.Unlock()
	}()

	// Send request
	c.connMu.Lock()
	err := writeControl(c.conn, req)
	c.connMu.Unlock()
	if err != nil {
		return Response{}, fmt.Errorf("send request: %w", err)
	}

	// Wait for response
	select {
	case resp := <-ch:
		return resp, nil
	case <-c.closed:
		return Response{}, fmt.Errorf("client closed")
	}
}

func (c *Client) writeInput(sessionID string, data []byte) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	return writeDataFrame(c.conn, frameInput, sessionID, data)
}

func (c *Client) readLoop() {
	reader := bufio.NewReader(c.conn)
	for {
		frameType, payload, err := readFrame(reader)
		if err != nil {
			select {
			case <-c.closed:
			default:
				log.Printf("shepherd client: read error: %v", err)
			}
			return
		}

		switch frameType {
		case frameControl:
			c.handleControlFrame(payload)
		case frameData:
			c.handleDataFrame(payload)
		}
	}
}

func (c *Client) handleControlFrame(payload []byte) {
	var resp Response
	if err := json.Unmarshal(payload, &resp); err != nil {
		log.Printf("shepherd client: bad control: %v", err)
		return
	}

	// Check if this is an exit notification (no request ID)
	if resp.Event == evtExited && resp.ID == "" {
		c.sessionMu.Lock()
		if done, ok := c.sessionDone[resp.SessionID]; ok {
			select {
			case <-done:
			default:
				close(done)
			}
		}
		c.sessionMu.Unlock()
		return
	}

	// Route response to pending request
	c.pendingMu.Lock()
	ch, ok := c.pending[resp.ID]
	c.pendingMu.Unlock()
	if ok {
		ch <- resp
	}
}

func (c *Client) handleDataFrame(payload []byte) {
	sessionID, data, err := parseDataPayload(payload)
	if err != nil {
		return
	}

	c.sessionMu.Lock()
	subs := c.sessionSubs[sessionID]
	c.sessionMu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- data:
		default:
		}
	}
}

func (c *Client) subscribe(sessionID string) (<-chan []byte, func()) {
	ch := make(chan []byte, 256)

	c.sessionMu.Lock()
	c.sessionSubs[sessionID] = append(c.sessionSubs[sessionID], ch)
	needSubscribe := !c.shepherdSubbed[sessionID]
	if needSubscribe {
		c.shepherdSubbed[sessionID] = true
	}
	c.sessionMu.Unlock()

	// Only tell the shepherd on the first local subscriber — the shepherd-side
	// goroutine persists for the lifetime of this client connection, so
	// subsequent subscribes would create duplicate forwarding goroutines.
	if needSubscribe {
		c.sendRequest(Request{Command: cmdSubscribe, SessionID: sessionID})
	}

	unsub := func() {
		c.sessionMu.Lock()
		defer c.sessionMu.Unlock()
		subs := c.sessionSubs[sessionID]
		for i, s := range subs {
			if s == ch {
				c.sessionSubs[sessionID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}
	return ch, unsub
}

func (c *Client) replay(sessionID string) []byte {
	resp, err := c.sendRequest(Request{Command: cmdReplay, SessionID: sessionID})
	if err != nil {
		return nil
	}
	return resp.Data
}

// Done returns a channel that is closed when the given session exits.
func (c *Client) Done(sessionID string) <-chan struct{} {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	ch, ok := c.sessionDone[sessionID]
	if !ok {
		ch = make(chan struct{})
		c.sessionDone[sessionID] = ch
	}
	return ch
}

// ProxySession implements ptymgr.SessionHandle by proxying to the shepherd.
type ProxySession struct {
	client    *Client
	sessionID string
}

func (p *ProxySession) Replay() []byte {
	return p.client.replay(p.sessionID)
}

func (p *ProxySession) Subscribe() (<-chan []byte, func()) {
	return p.client.subscribe(p.sessionID)
}

func (p *ProxySession) Write(data []byte) (int, error) {
	if err := p.client.writeInput(p.sessionID, data); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (p *ProxySession) Done() <-chan struct{} {
	return p.client.Done(p.sessionID)
}

// Compile-time interface checks.
var _ ptymgr.SessionManager = (*Client)(nil)
var _ ptymgr.SessionHandle = (*ProxySession)(nil)
