package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/databufflabs/databuff-diag/internal/policy"
	"github.com/databufflabs/databuff-diag/internal/sshresolve"
	"github.com/databufflabs/databuff-diag/internal/store"
)

const (
	ToolShell = "shell"
	ToolSSH   = "ssh"
)

// ToolCall is a parsed agent tool proposal (local shell or remote SSH).
type ToolCall struct {
	Kind          string
	ShellCommand  string
	SSHTool       *SSHToolCall
	DisplayCommand string
}

// SSHToolCall is the remote command portion of an SSH tool call.
type SSHToolCall struct {
	HostID        string
	Host          string
	User          string
	Password      string
	Port          int
	RemoteCommand string
}

func (t ToolCall) PolicyCommand() string {
	if t.Kind == ToolSSH && t.SSHTool != nil {
		return t.SSHTool.RemoteCommand
	}
	return t.ShellCommand
}

func (t ToolCall) ToPendingApproval(id string, risk policy.Risk) store.PendingApproval {
	pending := store.PendingApproval{
		ID:        id,
		Command:   t.DisplayCommand,
		Risk:      risk,
		ToolKind:  t.Kind,
		CreatedAt: time.Now().UTC(),
	}
	if t.Kind == ToolSSH && t.SSHTool != nil {
		pending.SSHTool = &store.SSHToolPayload{
			HostID:        t.SSHTool.HostID,
			Host:          t.SSHTool.Host,
			User:          t.SSHTool.User,
			Port:          t.SSHTool.Port,
			RemoteCommand: t.SSHTool.RemoteCommand,
		}
	}
	return pending
}

func ToolCallFromPending(p store.PendingApproval) (ToolCall, bool) {
	if p.ToolKind == ToolSSH && p.SSHTool != nil {
		return ToolCall{
			Kind:           ToolSSH,
			DisplayCommand: p.Command,
			SSHTool: &SSHToolCall{
				HostID:        p.SSHTool.HostID,
				Host:          p.SSHTool.Host,
				User:          p.SSHTool.User,
				Port:          p.SSHTool.Port,
				RemoteCommand: p.SSHTool.RemoteCommand,
			},
		}, true
	}
	if p.Command != "" {
		return ToolCall{
			Kind:           ToolShell,
			ShellCommand:   p.Command,
			DisplayCommand: p.Command,
		}, true
	}
	return ToolCall{}, false
}

func buildSSHToolDisplay(spec SSHToolCall, resolved sshresolve.Resolved) string {
	return sshresolve.DisplayCommand(resolved, spec.RemoteCommand)
}

func validateSSHTool(spec *SSHToolCall) error {
	if spec == nil {
		return fmt.Errorf("ssh tool is required")
	}
	if strings.TrimSpace(spec.HostID) == "" && strings.TrimSpace(spec.Host) == "" {
		return fmt.Errorf("ssh host or host_id is required")
	}
	if strings.TrimSpace(spec.RemoteCommand) == "" {
		return fmt.Errorf("ssh command is required")
	}
	if !looksLikeShellCommand(spec.RemoteCommand) {
		return fmt.Errorf("ssh command is not a valid shell command")
	}
	return nil
}
