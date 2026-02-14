package gateway

import (
	"html/template"
	"net/http"
)

// LoginPage renders the standalone login page.
type LoginPage struct {
	tmpl *template.Template
}

func NewLoginPage() *LoginPage {
	return &LoginPage{
		tmpl: template.Must(template.New("login").Parse(loginHTML)),
	}
}

func (lp *LoginPage) Render(w http.ResponseWriter, csrfToken, errorMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	lp.tmpl.Execute(w, map[string]string{
		"CSRFToken": csrfToken,
		"Error":     errorMsg,
	})
}

const loginHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Superposition - Login</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    background: #18181b; color: #fafafa;
    display: flex; align-items: center; justify-content: center;
    min-height: 100vh;
  }
  .card {
    background: #27272a; border: 1px solid #3f3f46; border-radius: 12px;
    padding: 2rem; width: 100%; max-width: 380px;
  }
  h1 { font-size: 1.25rem; font-weight: 600; margin-bottom: 0.25rem; }
  .subtitle { color: #a1a1aa; font-size: 0.875rem; margin-bottom: 1.5rem; }
  label { display: block; font-size: 0.875rem; color: #d4d4d8; margin-bottom: 0.375rem; }
  input[type="text"], input[type="password"] {
    width: 100%; padding: 0.5rem 0.75rem;
    background: #18181b; border: 1px solid #3f3f46; border-radius: 6px;
    color: #fafafa; font-size: 0.875rem;
    margin-bottom: 1rem; outline: none;
  }
  input:focus { border-color: #a78bfa; }
  button {
    width: 100%; padding: 0.5rem; background: #7c3aed; color: #fff;
    border: none; border-radius: 6px; font-size: 0.875rem; font-weight: 500;
    cursor: pointer;
  }
  button:hover { background: #6d28d9; }
  .error {
    background: #451a1a; border: 1px solid #7f1d1d; border-radius: 6px;
    color: #fca5a5; padding: 0.5rem 0.75rem; font-size: 0.8125rem;
    margin-bottom: 1rem;
  }
</style>
</head>
<body>
<div class="card">
  <h1>Superposition</h1>
  <p class="subtitle">Sign in to continue</p>
  {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
  <form method="POST" action="/auth/login">
    <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
    <label for="username">Username</label>
    <input type="text" id="username" name="username" autocomplete="username" required autofocus>
    <label for="password">Password</label>
    <input type="password" id="password" name="password" autocomplete="current-password" required>
    <button type="submit">Sign in</button>
  </form>
</div>
</body>
</html>`
