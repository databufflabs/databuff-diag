package server

import (
	"net"
	"testing"
)

func TestServeURL(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{"localhost:8787", "http://localhost:8787"},
		{"127.0.0.1:8787", "http://127.0.0.1:8787"},
	}
	for _, tt := range tests {
		if got := ServeURL(tt.addr); got != tt.want {
			t.Errorf("ServeURL(%q) = %q, want %q", tt.addr, got, tt.want)
		}
	}
}

func TestServeURL_wildcardBindUsesLocalIP(t *testing.T) {
	wantHost := primaryLocalIPv4()
	for _, addr := range []string{":8787", "0.0.0.0:8787"} {
		got := ServeURL(addr)
		want := "http://" + wantHost + ":8787"
		if got != want {
			t.Errorf("ServeURL(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestPrimaryLocalIPv4(t *testing.T) {
	ip := primaryLocalIPv4()
	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Fatalf("primaryLocalIPv4() = %q, not a valid IP", ip)
	}
	if parsed.To4() == nil {
		t.Fatalf("primaryLocalIPv4() = %q, expected IPv4", ip)
	}
}
