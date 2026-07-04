package sshresolve

import (
	"strings"
	"testing"

	"github.com/databufflabs/databuff-diag/internal/store"
)

func TestResolve_SavedHostPassword(t *testing.T) {
	cfg := &store.Config{
		SSH: store.SSHConfig{
			Hosts: store.SSHHostsList{
				{ID: "host-1", Name: "prod", Host: "192.168.1.10", User: "root", Password: "secret"},
			},
		},
	}

	res, err := Resolve(Request{HostID: "host-1"}, cfg, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Password != "secret" {
		t.Fatalf("password = %q, want secret", res.Password)
	}
	if res.AuthSource != AuthSourceSavedHost {
		t.Fatalf("auth source = %q, want saved_host", res.AuthSource)
	}
	if res.User != "root" || res.Host != "192.168.1.10" {
		t.Fatalf("target = %s@%s, want root@192.168.1.10", res.User, res.Host)
	}
}

func TestResolve_SessionOverride(t *testing.T) {
	session := &store.Session{
		SSHOverrides: map[string]store.SSHOverride{
			"192.168.1.20": {Host: "192.168.1.20", User: "admin", Password: "from-session"},
		},
	}

	res, err := Resolve(Request{Host: "192.168.1.20", User: "admin"}, nil, session)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Password != "from-session" {
		t.Fatalf("password = %q, want from-session", res.Password)
	}
	if res.AuthSource != AuthSourceSession {
		t.Fatalf("auth source = %q, want session_override", res.AuthSource)
	}
}

func TestResolve_ExplicitToolPassword(t *testing.T) {
	cfg := &store.Config{
		SSH: store.SSHConfig{
			Hosts: store.SSHHostsList{
				{ID: "host-1", Host: "10.0.0.1", User: "root", Password: "saved"},
			},
		},
	}

	res, err := Resolve(Request{Host: "10.0.0.1", User: "root", Password: "explicit"}, cfg, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Password != "explicit" {
		t.Fatalf("password = %q, want explicit", res.Password)
	}
	if res.AuthSource != AuthSourceUserMessage {
		t.Fatalf("auth source = %q, want user_message", res.AuthSource)
	}
}

func TestResolve_KeyOnly(t *testing.T) {
	res, err := Resolve(Request{Host: "example.com", User: "deploy"}, nil, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Password != "" {
		t.Fatalf("password = %q, want empty", res.Password)
	}
	if res.AuthSource != AuthSourceKeyOnly {
		t.Fatalf("auth source = %q, want key_only", res.AuthSource)
	}
}

func TestFindSavedHost_ByName(t *testing.T) {
	hosts := store.SSHHostsList{
		{ID: "id-a", Name: "prod-db", Host: "10.0.0.5", User: "root"},
	}
	got := FindSavedHost(hosts, "", "prod-db", "")
	if got == nil || got.Host != "10.0.0.5" {
		t.Fatalf("FindSavedHost = %+v", got)
	}
}

func TestDisplayCommand(t *testing.T) {
	got := DisplayCommand(Resolved{
		Host:        "192.168.1.10",
		User:        "root",
		DisplayName: "prod",
	}, "docker ps")
	want := `ssh prod (root@192.168.1.10) "docker ps"`
	if got != want {
		t.Fatalf("DisplayCommand = %q, want %q", got, want)
	}
}

func TestApplyMessageOverrides(t *testing.T) {
	session := &store.Session{}
	ApplyMessageOverrides(session, "请 ssh root@192.168.1.30 password: TempPass123 查看 docker")
	if session.SSHOverrides == nil {
		t.Fatal("expected overrides")
	}
	o, ok := session.SSHOverrides["192.168.1.30"]
	if !ok {
		t.Fatalf("overrides = %+v", session.SSHOverrides)
	}
	if o.Password != "TempPass123" || o.User != "root" {
		t.Fatalf("override = %+v", o)
	}
}

func TestFormatHostCatalog(t *testing.T) {
	catalog := FormatHostCatalog(store.SSHHostsList{
		{ID: "host-x", Name: "staging", Host: "10.0.0.2", User: "admin", Password: "x"},
	})
	for _, want := range []string{"host-x", "staging", "admin@10.0.0.2", "password configured"} {
		if !strings.Contains(catalog, want) {
			t.Fatalf("catalog missing %q: %s", want, catalog)
		}
	}
	if strings.Contains(catalog, `"x"`) {
		t.Fatalf("catalog must not expose password literal: %s", catalog)
	}
}
