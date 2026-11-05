package handlers

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

// defaultOrigins mirrors the CORS allowlist for local development.
var defaultOrigins = []string{
	"http://localhost", "http://localhost:3000", "http://127.0.0.1", "http://127.0.0.1:3000",
	"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:5175", "http://127.0.0.1:5175",
}

// allowedOrigins reads ALLOWED_ORIGINS (comma separated, for example
// https://app.example.com) and adds the development defaults.
func allowedOrigins() map[string]bool {
	out := map[string]bool{}
	for _, o := range defaultOrigins {
		out[o] = true
	}
	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			out[strings.ToLower(o)] = true
		}
	}
	return out
}

// checkOrigin rejects cross-site WebSocket hijacking: a page on another site
// must not be able to drive a learner's sandbox with their credentials.
// Requests without an Origin header (non-browser clients) are allowed, as are
// same-host requests behind the nginx proxy.
func checkOrigin(allowed map[string]bool) func(r *http.Request) bool {
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		if strings.EqualFold(u.Host, r.Host) || strings.EqualFold(u.Host, r.Header.Get("X-Forwarded-Host")) {
			return true
		}
		return allowed[strings.ToLower(u.Scheme+"://"+u.Host)]
	}
}
