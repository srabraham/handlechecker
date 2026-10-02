package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	ipFor := func(remote string) string {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		return clientIP(r)
	}

	if got := ipFor("203.0.113.7:54321"); got != "203.0.113.7" {
		t.Errorf("IPv4: got %q, want %q", got, "203.0.113.7")
	}

	if got := ipFor("[2001:db8::1]:8080"); got != "2001:db8::1" {
		t.Errorf("IPv6: got %q, want %q", got, "2001:db8::1")
	}

	if got := ipFor("no-port"); got != "no-port" {
		t.Errorf("no port: got %q, want %q", got, "no-port")
	}
}
