package gateway

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	sessionCookieName = "sp_session"
	csrfCookieName    = "sp_csrf"
	sessionDuration   = 7 * 24 * time.Hour
)

// Auth handles user authentication for the gateway.
type Auth struct {
	username  string
	password  string
	hmacKey   []byte
	loginPage *LoginPage
}

func NewAuth(username, password string) *Auth {
	key := make([]byte, 32)
	rand.Read(key)
	return &Auth{
		username:  username,
		password:  password,
		hmacKey:   key,
		loginPage: NewLoginPage(),
	}
}

// Middleware returns an HTTP middleware that enforces authentication.
// Exempt paths: /api/health, /auth/*, /tunnel
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Exempt paths
		if path == "/api/health" || strings.HasPrefix(path, "/auth/") || path == "/tunnel" {
			next.ServeHTTP(w, r)
			return
		}

		if !a.validSession(r) {
			if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/ws/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			} else {
				http.Redirect(w, r, "/auth/login", http.StatusFound)
			}
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Routes registers auth-related HTTP handlers on the given mux.
func (a *Auth) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", a.handleLoginPage)
	mux.HandleFunc("POST /auth/login", a.handleLogin)
	mux.HandleFunc("POST /auth/logout", a.handleLogout)
}

func (a *Auth) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	// If already authenticated, redirect to app
	if a.validSession(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Generate CSRF token
	csrf := a.generateCSRF()
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    csrf,
		Path:     "/auth/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   true,
	})

	a.loginPage.Render(w, csrf, "")
}

func (a *Auth) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.loginPage.Render(w, "", "Invalid request")
		return
	}

	// Verify CSRF
	csrfCookie, err := r.Cookie(csrfCookieName)
	if err != nil || csrfCookie.Value == "" || csrfCookie.Value != r.FormValue("csrf_token") {
		csrf := a.generateCSRF()
		http.SetCookie(w, &http.Cookie{
			Name:     csrfCookieName,
			Value:    csrf,
			Path:     "/auth/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Secure:   true,
		})
		a.loginPage.Render(w, csrf, "Invalid request, please try again")
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username != a.username || password != a.password {
		csrf := a.generateCSRF()
		http.SetCookie(w, &http.Cookie{
			Name:     csrfCookieName,
			Value:    csrf,
			Path:     "/auth/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Secure:   true,
		})
		a.loginPage.Render(w, csrf, "Invalid username or password")
		return
	}

	// Create session cookie
	expires := time.Now().Add(sessionDuration)
	token := a.signSession(username, expires)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})

	// Clear CSRF cookie
	http.SetCookie(w, &http.Cookie{
		Name:   csrfCookieName,
		Path:   "/auth/",
		MaxAge: -1,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *Auth) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   sessionCookieName,
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/auth/login", http.StatusFound)
}

// signSession creates an HMAC-signed session token.
// Format: username|expiry_unix|signature
func (a *Auth) signSession(username string, expires time.Time) string {
	payload := fmt.Sprintf("%s|%d", username, expires.Unix())
	mac := hmac.New(sha256.New, a.hmacKey)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s|%s", payload, sig)
}

// validSession checks if the request has a valid session cookie.
func (a *Auth) validSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}

	parts := strings.SplitN(cookie.Value, "|", 3)
	if len(parts) != 3 {
		return false
	}

	username := parts[0]
	payload := fmt.Sprintf("%s|%s", parts[0], parts[1])
	sig := parts[2]

	// Verify signature
	mac := hmac.New(sha256.New, a.hmacKey)
	mac.Write([]byte(payload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return false
	}

	// Verify username
	if username != a.username {
		return false
	}

	// Verify expiry
	var expiry int64
	fmt.Sscanf(parts[1], "%d", &expiry)
	if time.Now().Unix() > expiry {
		return false
	}

	return true
}

func (a *Auth) generateCSRF() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
