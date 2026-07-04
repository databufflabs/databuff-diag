package sshresolve

import (
	"regexp"
	"strings"
)

var (
	userAtHostPassRE = regexp.MustCompile(`(?i)([a-z0-9._-]+)@(\d{1,3}(?:\.\d{1,3}){3}|[a-z0-9.-]+)\s*(?:[/:,]\s*|\s+(?:password|passwd|pwd|密码)\s*[:：]?\s*)(\S+)`)
	hostUserPassRE   = regexp.MustCompile(`(?i)(?:host|主机|地址)\s*[:：]?\s*(\d{1,3}(?:\.\d{1,3}){3}|[a-z0-9.-]+).*?(?:user|username|用户|账号)\s*[:：]?\s*(\S+).*?(?:password|passwd|pwd|密码)\s*[:：]?\s*(\S+)`)
	ipUserPassRE     = regexp.MustCompile(`(?i)(?:ssh\s+)?(\d{1,3}(?:\.\d{1,3}){3})\s+(?:user|username|用户|账号)\s*[:：]?\s*(\S+).*?(?:password|passwd|pwd|密码)\s*[:：]?\s*(\S+)`)
	userPassHostRE   = regexp.MustCompile(`(?i)(?:user|username|用户|账号)\s*[:：]?\s*(\S+).*?(?:password|passwd|pwd|密码)\s*[:：]?\s*(\S+).*?(?:host|主机|地址)\s*[:：]?\s*(\d{1,3}(?:\.\d{1,3}){3}|[a-z0-9.-]+)`)
)

// ParseFromUserMessage extracts explicit SSH credentials from free-form user text.
func ParseFromUserMessage(text string) []SSHAuth {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	seen := make(map[string]struct{})
	var out []SSHAuth

	add := func(host, user, password string) {
		host = NormalizeHost(host)
		if host == "" || password == "" {
			return
		}
		key := NormalizeKey(host, 22) + "|" + user
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, SSHAuth{Host: host, User: user, Password: password, Port: 22})
	}

	if m := userAtHostPassRE.FindStringSubmatch(text); len(m) == 4 {
		add(m[2], m[1], m[3])
	}
	if m := hostUserPassRE.FindStringSubmatch(text); len(m) == 4 {
		add(m[1], m[2], m[3])
	}
	if m := ipUserPassRE.FindStringSubmatch(text); len(m) == 4 {
		add(m[1], m[2], m[3])
	}
	if m := userPassHostRE.FindStringSubmatch(text); len(m) == 4 {
		add(m[3], m[1], m[2])
	}

	return out
}
