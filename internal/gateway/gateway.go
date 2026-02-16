package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
)

// Config holds gateway configuration.
type Config struct {
	Port     int
	TLSCert  string
	TLSKey   string
	Username string
	Password string
	Secret   string
}

// Run starts the gateway server. Called from main.go subcommand dispatch.
func Run(cfg Config, spaHandler http.Handler) error {
	// Validate required config
	if cfg.Username == "" || cfg.Password == "" {
		return fmt.Errorf("SP_USERNAME and SP_PASSWORD environment variables are required")
	}

	// Generate secret if not provided
	if cfg.Secret == "" {
		b := make([]byte, 24)
		rand.Read(b)
		cfg.Secret = hex.EncodeToString(b)
	}

	auth := NewAuth(cfg.Username, cfg.Password)
	tun := NewTunnel(cfg.Secret)
	proxy := NewProxy(tun, spaHandler)

	mux := http.NewServeMux()

	// Auth routes (not behind auth middleware)
	auth.Routes(mux)

	// Tunnel endpoint (authenticated by pre-shared secret, not user session)
	mux.HandleFunc("/tunnel", tun.Handler())

	// Gateway health endpoint for k8s liveness probe (exempt from auth).
	// Uses a distinct path so /api/health is proxied to the superposition server.
	mux.HandleFunc("GET /gateway/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","gateway":true,"connected":%t}`, tun.Connected())
	})

	// Everything else goes through auth middleware → proxy
	mux.Handle("/", auth.Middleware(proxy))

	// TLS config
	tlsCfg, err := TLSConfig(cfg.TLSCert, cfg.TLSKey)
	if err != nil {
		return fmt.Errorf("TLS config: %w", err)
	}

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	srv := &http.Server{
		Addr:      addr,
		Handler:   mux,
		TLSConfig: tlsCfg,
	}

	fmt.Println("Superposition Gateway")
	fmt.Println("=====================")
	fmt.Println()

	scheme := "https"
	displayPort := cfg.Port
	tunnelURL := fmt.Sprintf("wss://YOUR_HOST:%d/tunnel", displayPort)
	if displayPort == 443 {
		tunnelURL = "wss://YOUR_HOST/tunnel"
	}

	fmt.Printf("Listening on %s://%s\n", scheme, addr)
	fmt.Printf("Tunnel secret: %s\n", cfg.Secret)
	fmt.Println()
	fmt.Println("Connect superposition with:")
	fmt.Printf("  superposition --gateway %s --gateway-secret %s\n", tunnelURL, cfg.Secret)
	fmt.Println()

	// ListenAndServeTLS with empty cert/key since we set TLSConfig directly
	return srv.ListenAndServeTLS("", "")
}

// ParseConfig reads gateway configuration from flags and environment.
func ParseConfig(args []string) Config {
	cfg := Config{
		Port:     443,
		Username: os.Getenv("SP_USERNAME"),
		Password: os.Getenv("SP_PASSWORD"),
		Secret:   os.Getenv("SP_GATEWAY_SECRET"),
	}

	// Parse gateway-specific flags from args
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port":
			if i+1 < len(args) {
				if p, err := strconv.Atoi(args[i+1]); err == nil {
					cfg.Port = p
				}
				i++
			}
		case "--tls-cert":
			if i+1 < len(args) {
				cfg.TLSCert = args[i+1]
				i++
			}
		case "--tls-key":
			if i+1 < len(args) {
				cfg.TLSKey = args[i+1]
				i++
			}
		default:
			log.Printf("gateway: unknown flag: %s", args[i])
		}
	}

	return cfg
}
