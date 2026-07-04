package sshresolve

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/databufflabs/databuff-diag/internal/store"
)

const (
	AuthSourceUserMessage = "user_message"
	AuthSourceSession     = "session_override"
	AuthSourceSavedHost   = "saved_host"
	AuthSourceKeyOnly       = "key_only"
)

// SSHAuth holds credentials parsed from a user message or session override.
type SSHAuth struct {
	Host     string
	User     string
	Password string
	Port     int
}

// Request is the SSH target requested by an agent tool call.
type Request struct {
	HostID   string
	Host     string
	User     string
	Password string
	Port     int
}

// Resolved is a fully resolved SSH connection spec.
type Resolved struct {
	Host        string
	User        string
	Password    string
	Port        int
	AuthSource  string
	DisplayName string
	HostID      string
}

// NormalizeHost strips an optional :port suffix and lowercases hostnames (not IPs).
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if h, p, err := net.SplitHostPort(host); err == nil && p != "" {
		return h
	}
	return host
}

// NormalizeKey returns a lookup key for host-based override maps.
func NormalizeKey(host string, port int) string {
	h := NormalizeHost(host)
	if h == "" {
		return ""
	}
	if port > 0 && port != 22 {
		return net.JoinHostPort(h, strconv.Itoa(port))
	}
	return strings.ToLower(h)
}

// ApplyMessageOverrides parses credentials from a user message and merges them
// into the session override map.
func ApplyMessageOverrides(session *store.Session, message string) {
	if session == nil {
		return
	}
	auths := ParseFromUserMessage(message)
	if len(auths) == 0 {
		return
	}
	if session.SSHOverrides == nil {
		session.SSHOverrides = make(map[string]store.SSHOverride)
	}
	for _, auth := range auths {
		key := NormalizeKey(auth.Host, auth.Port)
		if key == "" {
			continue
		}
		session.SSHOverrides[key] = store.SSHOverride{
			Host:     auth.Host,
			User:     auth.User,
			Password: auth.Password,
			Port:     auth.Port,
		}
	}
}

// FindSavedHost locates a configured host by id, display name, or address.
func FindSavedHost(hosts store.SSHHostsList, hostID, name, host string) *store.SSHHost {
	host = NormalizeHost(host)
	for i := range hosts {
		h := &hosts[i]
		if hostID != "" && h.ID == hostID {
			return h
		}
	}
	if name != "" {
		nameLower := strings.ToLower(strings.TrimSpace(name))
		for i := range hosts {
			h := &hosts[i]
			if strings.EqualFold(strings.TrimSpace(h.Name), nameLower) {
				return h
			}
		}
	}
	if host != "" {
		hostLower := strings.ToLower(host)
		for i := range hosts {
			h := &hosts[i]
			if strings.EqualFold(NormalizeHost(h.Host), hostLower) {
				return h
			}
		}
	}
	return nil
}

// Resolve picks credentials using priority: explicit tool password, session
// override, saved host config, then key-only SSH.
func Resolve(req Request, cfg *store.Config, session *store.Session) (Resolved, error) {
	if req.HostID == "" && strings.TrimSpace(req.Host) == "" {
		return Resolved{}, fmt.Errorf("ssh host or host_id is required")
	}

	var saved *store.SSHHost
	if cfg != nil {
		saved = FindSavedHost(cfg.SSH.Hosts, req.HostID, "", req.Host)
	}

	out := Resolved{
		Host: strings.TrimSpace(req.Host),
		User: strings.TrimSpace(req.User),
		Port: req.Port,
	}

	if saved != nil {
		out.HostID = saved.ID
		out.DisplayName = saved.Name
		if out.Host == "" {
			out.Host = saved.Host
		}
		if out.User == "" {
			out.User = saved.User
		}
		if out.Port == 0 {
			out.Port = saved.Port
		}
	}

	if out.Host == "" {
		return Resolved{}, fmt.Errorf("ssh host is required")
	}
	out.Host = NormalizeHost(out.Host)
	if out.Port == 0 {
		out.Port = 22
	}

	if pw := strings.TrimSpace(req.Password); pw != "" {
		out.Password = pw
		out.AuthSource = AuthSourceUserMessage
		return out, nil
	}

	if session != nil && session.SSHOverrides != nil {
		if o, ok := session.SSHOverrides[NormalizeKey(out.Host, out.Port)]; ok {
			if o.User != "" && out.User == "" {
				out.User = o.User
			}
			if o.Port > 0 && out.Port == 22 {
				out.Port = o.Port
			}
			if o.Password != "" {
				out.Password = o.Password
				out.AuthSource = AuthSourceSession
				return out, nil
			}
		}
	}

	if saved != nil && saved.Password != "" {
		out.Password = saved.Password
		out.AuthSource = AuthSourceSavedHost
		return out, nil
	}

	out.AuthSource = AuthSourceKeyOnly
	return out, nil
}

// DisplayCommand returns a sanitized command string safe for logs and LLM history.
func DisplayCommand(res Resolved, remoteCmd string) string {
	target := res.User + "@" + res.Host
	if res.Port > 0 && res.Port != 22 {
		target = net.JoinHostPort(target, strconv.Itoa(res.Port))
	}
	label := target
	if res.DisplayName != "" {
		label = res.DisplayName + " (" + target + ")"
	}
	quoted := strconv.Quote(remoteCmd)
	return fmt.Sprintf("ssh %s %s", label, quoted)
}

// HostCatalogLine formats one saved host for the system prompt (no secrets).
func HostCatalogLine(h store.SSHHost) string {
	port := h.Port
	if port == 0 {
		port = 22
	}
	user := h.User
	if user == "" {
		user = "—"
	}
	name := h.Name
	if name == "" {
		name = h.Host
	}
	addr := h.Host
	if port != 22 {
		addr = net.JoinHostPort(h.Host, strconv.Itoa(port))
	}
	pwNote := ""
	if h.Password != "" {
		pwNote = ", password configured"
	}
	return fmt.Sprintf("- %s (id: %s): %s@%s%s", name, h.ID, user, addr, pwNote)
}

// FormatHostCatalog builds the saved-host section for the agent system prompt.
func FormatHostCatalog(hosts store.SSHHostsList) string {
	if len(hosts) == 0 {
		return "No saved SSH hosts configured."
	}
	var b strings.Builder
	b.WriteString("Saved SSH hosts (passwords are injected by the system — never include passwords in tool calls):\n")
	for _, h := range hosts {
		b.WriteString(HostCatalogLine(h))
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}
