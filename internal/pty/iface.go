package pty

// SessionHandle represents a handle to a running PTY session.
type SessionHandle interface {
	Replay() []byte
	Subscribe() (<-chan []byte, func())
	Write(data []byte) (int, error)
	Done() <-chan struct{}
}

// SessionManager manages PTY session lifecycles.
type SessionManager interface {
	Start(id, cliType, workDir string) (SessionHandle, int /* pid */, error)
	Stop(id string) error
	Get(id string) SessionHandle
	Resize(id string, rows, cols uint16) error
	StopAll()
}
