package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestCheckOrigin(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "https://app.entropy.dev/, https://Other.example")
	check := checkOrigin(allowedOrigins())
	cases := []struct {
		origin, host, fwd string
		want              bool
	}{
		{"", "sm:8080", "", true},
		{"http://localhost:5175", "127.0.0.1:8082", "", true},
		{"https://app.entropy.dev", "session-manager:8080", "", true},
		{"https://other.example", "x", "", true},
		{"https://evil.example", "session-manager:8080", "", false},
		{"https://app.entropy.dev.evil.example", "sm", "", false},
		{"https://public.host", "public.host", "", true},
		{"https://public.host", "session-manager:8080", "public.host", true},
		{"not a url", "sm", "", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/ws", nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.fwd != "" {
			r.Header.Set("X-Forwarded-Host", c.fwd)
		}
		if got := check(r); got != c.want {
			t.Fatalf("origin=%q host=%q fwd=%q: got %v want %v", c.origin, c.host, c.fwd, got, c.want)
		}
	}
}
