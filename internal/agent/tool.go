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
	ToolRead  = "read"
	ToolWrite = "write"
	ToolEdit  = "edit"
	ToolBash  = "bash"
	ToolShell = "shell" // legacy JSON alias for bash
	ToolSSH   = "ssh"
)

// TextEdit is one exact replacement in an edit tool call.
type TextEdit struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

// ToolCall is a parsed agent tool proposal.
type ToolCall struct {
	Kind           string
	ID             string
	ShellCommand   string
	ReadPath       string
	ReadOffset     int
	ReadLimit      int
	WritePath      string
	WriteContent   string
	EditPath       string
	Edits          []TextEdit
	SSHTool        *SSHToolCall
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
	switch t.Kind {
	case ToolSSH:
		if t.SSHTool != nil {
			return t.SSHTool.RemoteCommand
		}
	case ToolRead:
		if t.ReadPath != "" {
			return "cat " + t.ReadPath
		}
	case ToolWrite:
		if t.WritePath != "" {
			return "echo > " + t.WritePath
		}
	case ToolEdit:
		if t.EditPath != "" {
			return "sed -i " + t.EditPath
		}
	}
	return t.ShellCommand
}

func (t ToolCall) ToPendingApproval(id string, risk policy.Risk) store.PendingApproval {
	pending := store.PendingApproval{
		ID:        id,
		Command:   t.DisplayCommand,
		Risk:      risk,
		ToolKind:  t.Kind,
		ToolID:    t.ID,
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
	if t.Kind == ToolRead {
		pending.ReadTool = &store.ReadToolPayload{
			Path:   t.ReadPath,
			Offset: t.ReadOffset,
			Limit:  t.ReadLimit,
		}
	}
	if t.Kind == ToolWrite {
		pending.WriteTool = &store.WriteToolPayload{
			Path:    t.WritePath,
			Content: t.WriteContent,
		}
	}
	if t.Kind == ToolEdit {
		edits := make([]store.TextEditPayload, len(t.Edits))
		for i, e := range t.Edits {
			edits[i] = store.TextEditPayload{OldText: e.OldText, NewText: e.NewText}
		}
		pending.EditTool = &store.EditToolPayload{
			Path:  t.EditPath,
			Edits: edits,
		}
	}
	if t.Kind == ToolBash || t.Kind == ToolShell {
		pending.ShellCommand = t.ShellCommand
	}
	return pending
}

func ToolCallFromPending(p store.PendingApproval) (ToolCall, bool) {
	if p.ToolKind == ToolSSH && p.SSHTool != nil {
		return ToolCall{
			Kind:           ToolSSH,
			ID:             p.ToolID,
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
	if p.ToolKind == ToolRead && p.ReadTool != nil {
		return ToolCall{
			Kind:           ToolRead,
			ID:             p.ToolID,
			DisplayCommand: p.Command,
			ReadPath:       p.ReadTool.Path,
			ReadOffset:     p.ReadTool.Offset,
			ReadLimit:      p.ReadTool.Limit,
		}, true
	}
	if p.ToolKind == ToolWrite && p.WriteTool != nil {
		return ToolCall{
			Kind:           ToolWrite,
			ID:             p.ToolID,
			DisplayCommand: p.Command,
			WritePath:      p.WriteTool.Path,
			WriteContent:   p.WriteTool.Content,
		}, true
	}
	if p.ToolKind == ToolEdit && p.EditTool != nil {
		edits := make([]TextEdit, len(p.EditTool.Edits))
		for i, e := range p.EditTool.Edits {
			edits[i] = TextEdit{OldText: e.OldText, NewText: e.NewText}
		}
		return ToolCall{
			Kind:           ToolEdit,
			ID:             p.ToolID,
			DisplayCommand: p.Command,
			EditPath:       p.EditTool.Path,
			Edits:          edits,
		}, true
	}
	shell := p.ShellCommand
	if shell == "" {
		shell = p.Command
	}
	if p.ToolKind == ToolBash || p.ToolKind == ToolShell || shell != "" {
		return ToolCall{
			Kind:           ToolBash,
			ID:             p.ToolID,
			ShellCommand:   shell,
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

func displayForTool(t ToolCall) string {
	switch t.Kind {
	case ToolRead:
		if t.ReadOffset > 0 || t.ReadLimit > 0 {
			return fmt.Sprintf("read %s (offset=%d limit=%d)", t.ReadPath, t.ReadOffset, t.ReadLimit)
		}
		return "read " + t.ReadPath
	case ToolWrite:
		return "write " + t.WritePath
	case ToolEdit:
		return fmt.Sprintf("edit %s (%d change(s))", t.EditPath, len(t.Edits))
	case ToolSSH:
		return t.DisplayCommand
	default:
		if t.DisplayCommand != "" {
			return t.DisplayCommand
		}
		return t.ShellCommand
	}
}
