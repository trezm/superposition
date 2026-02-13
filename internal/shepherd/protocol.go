package shepherd

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// Frame types for the binary protocol.
const (
	frameControl byte = 0x01 // JSON control message
	frameData    byte = 0x02 // PTY output data: sessionID + raw bytes
	frameInput   byte = 0x03 // PTY input data: sessionID + raw bytes
)

// Command types for JSON control messages.
const (
	cmdStart     = "start"
	cmdStop      = "stop"
	cmdResize    = "resize"
	cmdReplay    = "replay"
	cmdSubscribe = "subscribe"
	cmdList      = "list"
	cmdPing      = "ping"
	cmdStopAll   = "stop_all"
)

// Event types sent from shepherd to client.
const (
	evtStarted  = "started"
	evtStopped  = "stopped"
	evtError    = "error"
	evtReplay   = "replay"
	evtList     = "list"
	evtPong     = "pong"
	evtExited   = "exited" // process exited
	evtStopDone = "stop_done"
)

// Request is a JSON control message from client to shepherd.
type Request struct {
	ID      string `json:"id"`      // request correlation ID
	Command string `json:"command"` // cmdStart, cmdStop, etc.

	// Start fields
	SessionID string `json:"session_id,omitempty"`
	CLIType   string `json:"cli_type,omitempty"`
	WorkDir   string `json:"work_dir,omitempty"`

	// Resize fields
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
}

// Response is a JSON control message from shepherd to client.
type Response struct {
	ID    string `json:"id"`    // correlates with request ID
	Event string `json:"event"` // evtStarted, evtError, etc.

	// Start response
	PID int `json:"pid,omitempty"`

	// Error response
	Error string `json:"error,omitempty"`

	// Replay response
	SessionID string `json:"session_id,omitempty"`
	Data      []byte `json:"data,omitempty"`

	// List response
	Sessions []string `json:"sessions,omitempty"`

	// Exited notification (no request ID)
	// SessionID is set above
}

// Wire format:
//   [4 bytes big-endian length][1 byte frame type][payload]
// For frameControl: payload is JSON-encoded Request or Response
// For frameData/frameInput: payload is [session_id_len(1 byte)][session_id][raw data]

func writeFrame(w io.Writer, frameType byte, payload []byte) error {
	length := uint32(1 + len(payload)) // frame type + payload
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return fmt.Errorf("write length: %w", err)
	}
	if _, err := w.Write([]byte{frameType}); err != nil {
		return fmt.Errorf("write frame type: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}

func writeControl(w io.Writer, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return writeFrame(w, frameControl, data)
}

func writeDataFrame(w io.Writer, frameType byte, sessionID string, data []byte) error {
	idBytes := []byte(sessionID)
	payload := make([]byte, 1+len(idBytes)+len(data))
	payload[0] = byte(len(idBytes))
	copy(payload[1:], idBytes)
	copy(payload[1+len(idBytes):], data)
	return writeFrame(w, frameType, payload)
}

func readFrame(r io.Reader) (byte, []byte, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return 0, nil, err
	}
	if length == 0 {
		return 0, nil, fmt.Errorf("empty frame")
	}
	if length > 10*1024*1024 { // 10MB sanity limit
		return 0, nil, fmt.Errorf("frame too large: %d", length)
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, nil, err
	}
	return buf[0], buf[1:], nil
}

func parseDataPayload(payload []byte) (sessionID string, data []byte, err error) {
	if len(payload) < 1 {
		return "", nil, fmt.Errorf("data payload too short")
	}
	idLen := int(payload[0])
	if len(payload) < 1+idLen {
		return "", nil, fmt.Errorf("data payload too short for session ID")
	}
	sessionID = string(payload[1 : 1+idLen])
	data = payload[1+idLen:]
	return sessionID, data, nil
}
