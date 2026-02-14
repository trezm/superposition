package gateway

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
)

var errNoTunnel = errors.New("gateway: superposition not connected")

// Proxy forwards user HTTP and WebSocket traffic through the yamux tunnel.
type Proxy struct {
	tunnel     *Tunnel
	spaHandler http.Handler
}

func NewProxy(tunnel *Tunnel, spaHandler http.Handler) *Proxy {
	return &Proxy{tunnel: tunnel, spaHandler: spaHandler}
}

// ServeHTTP handles all proxied requests.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	isAPI := strings.HasPrefix(path, "/api/")
	isWS := strings.HasPrefix(path, "/ws/")

	if !isAPI && !isWS {
		// SPA fallback — serve frontend assets
		p.spaHandler.ServeHTTP(w, r)
		return
	}

	// Check if this is a WebSocket upgrade
	if isWS && r.Header.Get("Upgrade") == "websocket" {
		p.proxyWebSocket(w, r)
		return
	}

	p.proxyHTTP(w, r)
}

func (p *Proxy) proxyHTTP(w http.ResponseWriter, r *http.Request) {
	stream, err := p.tunnel.OpenStream()
	if err != nil {
		http.Error(w, `{"error":"gateway not connected to superposition"}`, http.StatusBadGateway)
		return
	}
	defer stream.Close()

	// Write the original HTTP request to the stream
	if err := r.Write(stream); err != nil {
		log.Printf("proxy: write request: %v", err)
		http.Error(w, `{"error":"tunnel write failed"}`, http.StatusBadGateway)
		return
	}

	// Read the response from superposition
	resp, err := http.ReadResponse(bufio.NewReader(stream), r)
	if err != nil {
		log.Printf("proxy: read response: %v", err)
		http.Error(w, `{"error":"tunnel read failed"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, vals := range resp.Header {
		for _, val := range vals {
			w.Header().Add(key, val)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (p *Proxy) proxyWebSocket(w http.ResponseWriter, r *http.Request) {
	stream, err := p.tunnel.OpenStream()
	if err != nil {
		http.Error(w, `{"error":"gateway not connected to superposition"}`, http.StatusBadGateway)
		return
	}

	// Hijack the user's connection to get raw TCP
	hj, ok := w.(http.Hijacker)
	if !ok {
		stream.Close()
		http.Error(w, "websocket hijack not supported", http.StatusInternalServerError)
		return
	}

	clientConn, clientBuf, err := hj.Hijack()
	if err != nil {
		stream.Close()
		log.Printf("proxy: hijack: %v", err)
		return
	}

	// Write the original HTTP upgrade request through the tunnel
	if err := r.Write(stream); err != nil {
		stream.Close()
		clientConn.Close()
		log.Printf("proxy: ws write upgrade: %v", err)
		return
	}

	// Flush any buffered data from the hijacked connection
	if clientBuf.Reader.Buffered() > 0 {
		buffered := make([]byte, clientBuf.Reader.Buffered())
		clientBuf.Read(buffered)
		stream.Write(buffered)
	}

	// Bidirectional copy between client and tunnel stream
	done := make(chan struct{})
	go func() {
		io.Copy(clientConn, stream)
		closeWrite(clientConn)
		close(done)
	}()
	io.Copy(stream, clientConn)
	closeWrite(stream)
	<-done

	stream.Close()
	clientConn.Close()
}

// closeWrite sends a TCP FIN if the connection supports it.
func closeWrite(c interface{}) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		cw.CloseWrite()
	}
}
