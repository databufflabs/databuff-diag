package server

import "testing"

func TestServeURL(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{":8787", "http://127.0.0.1:8787"},
		{"0.0.0.0:8787", "http://127.0.0.1:8787"},
		{"localhost:8787", "http://localhost:8787"},
		{"127.0.0.1:8787", "http://127.0.0.1:8787"},
	}
	for _, tt := range tests {
		if got := ServeURL(tt.addr); got != tt.want {
			t.Errorf("ServeURL(%q) = %q, want %q", tt.addr, got, tt.want)
		}
	}
}
