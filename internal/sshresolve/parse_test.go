package sshresolve

import (
	"strings"
	"testing"
)

func TestParseFromUserMessage(t *testing.T) {
	tests := []struct {
		text     string
		wantHost string
		wantUser string
		wantPass string
	}{
		{
			text:     "请 ssh root@192.168.1.30 password: TempPass123 查看 docker",
			wantHost: "192.168.1.30",
			wantUser: "root",
			wantPass: "TempPass123",
		},
		{
			text:     "主机: 10.0.0.5 用户: admin 密码: secret123",
			wantHost: "10.0.0.5",
			wantUser: "admin",
			wantPass: "secret123",
		},
	}

	for _, tc := range tests {
		got := ParseFromUserMessage(tc.text)
		if len(got) != 1 {
			t.Fatalf("ParseFromUserMessage(%q) = %+v, want 1 entry", tc.text, got)
		}
		if got[0].Host != tc.wantHost || got[0].User != tc.wantUser || got[0].Password != tc.wantPass {
			t.Fatalf("ParseFromUserMessage(%q) = %+v", tc.text, got[0])
		}
	}
}

func TestRedactPasswordInDisplay(t *testing.T) {
	display := DisplayCommand(Resolved{Host: "1.2.3.4", User: "root"}, "docker ps")
	if strings.Contains(display, "password") || strings.Contains(display, "sshpass") {
		t.Fatalf("display leaked auth details: %s", display)
	}
}
