package tunnel

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

// Client connects outbound to a gateway and multiplexes traffic via yamux.
type Client struct {
	gatewayURL string // wss://gateway.example.com/tunnel
	secret     string // pre-shared secret
	localAddr  string // e.g. localhost:8800
}

func NewClient(gatewayURL, secret, localAddr string) *Client {
	return &Client{
		gatewayURL: gatewayURL,
		secret:     secret,
		localAddr:  localAddr,
	}
}

// Run connects to the gateway and serves tunnel traffic. Reconnects on failure.
// Blocks until ctx-like cancellation (caller should run in a goroutine).
func (c *Client) Run() {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		err := c.connect()
		if err != nil {
			log.Printf("tunnel: connection failed: %v", err)
			log.Printf("tunnel: reconnecting in %s...", backoff)
			time.Sleep(backoff)
			backoff = min(backoff*2, maxBackoff)
		} else {
			// Connected successfully at some point, reset backoff
			backoff = time.Second
		}
	}
}

func (c *Client) connect() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		// Allow self-signed certs (gateway defaults to self-signed;
		// the pre-shared secret authenticates the connection)
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	header := http.Header{}
	header.Set("X-Gateway-Secret", c.secret)

	wsConn, _, err := dialer.Dial(c.gatewayURL, header)
	if err != nil {
		return fmt.Errorf("dial gateway: %w", err)
	}
	defer wsConn.Close()

	log.Printf("tunnel: connected to gateway %s", c.gatewayURL)

	// Superposition is the yamux server (accepts streams opened by gateway)
	session, err := yamux.Server(NewWSConn(wsConn), yamux.DefaultConfig())
	if err != nil {
		return fmt.Errorf("yamux server: %w", err)
	}
	defer session.Close()

	for {
		stream, err := session.Accept()
		if err != nil {
			return fmt.Errorf("accept stream: %w", err)
		}
		go c.handleStream(stream)
	}
}

func (c *Client) handleStream(stream net.Conn) {
	defer stream.Close()

	local, err := net.Dial("tcp", c.localAddr)
	if err != nil {
		log.Printf("tunnel: dial local %s: %v", c.localAddr, err)
		return
	}
	defer local.Close()

	// Bidirectional copy
	done := make(chan struct{})
	go func() {
		io.Copy(local, stream)
		close(done)
	}()
	io.Copy(stream, local)
	<-done
}
