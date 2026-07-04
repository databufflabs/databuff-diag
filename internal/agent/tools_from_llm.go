package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/databufflabs/databuff-diag/internal/llm"
)

// ToolCallsFromCompletion returns agent tool calls from a chat completion.
// Falls back to parsing tool JSON embedded in assistant text.
func ToolCallsFromCompletion(content string, calls []llm.FunctionToolCall) ([]ToolCall, bool) {
	if len(calls) > 0 {
		out := make([]ToolCall, 0, len(calls))
		for _, c := range calls {
			tc, err := ToolCallFromFunction(c)
			if err != nil {
				continue
			}
			out = append(out, tc)
		}
		if len(out) > 0 {
			return out, true
		}
	}
	if !looksLikeEmbeddedToolProposal(content) {
		return nil, false
	}
	tool, ok := ParseTool(content)
	if !ok {
		return nil, false
	}
	return []ToolCall{tool}, true
}

// ToolCallFromFunction parses a native function tool call.
func ToolCallFromFunction(call llm.FunctionToolCall) (ToolCall, error) {
	name := strings.TrimSpace(call.Function.Name)
	args := strings.TrimSpace(call.Function.Arguments)
	id := strings.TrimSpace(call.ID)
	if name == "" {
		return ToolCall{}, fmt.Errorf("tool name is required")
	}

	switch name {
	case ToolBash, ToolShell:
		var input struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(args), &input); err != nil {
			return ToolCall{}, err
		}
		cmd := strings.TrimSpace(input.Command)
		if cmd == "" || !looksLikeShellCommand(cmd) || isInlineScriptBody(cmd) {
			return ToolCall{}, fmt.Errorf("invalid bash command")
		}
		return ToolCall{
			Kind:           ToolBash,
			ID:             id,
			ShellCommand:   cmd,
			DisplayCommand: cmd,
		}, nil

	case ToolSSH:
		var input struct {
			HostID   string `json:"host_id"`
			Host     string `json:"host"`
			User     string `json:"user"`
			Password string `json:"password"`
			Port     int    `json:"port"`
			Command  string `json:"command"`
		}
		if err := json.Unmarshal([]byte(args), &input); err != nil {
			return ToolCall{}, err
		}
		spec := &SSHToolCall{
			HostID:        strings.TrimSpace(input.HostID),
			Host:          strings.TrimSpace(input.Host),
			User:          strings.TrimSpace(input.User),
			Password:      strings.TrimSpace(input.Password),
			Port:          input.Port,
			RemoteCommand: strings.TrimSpace(input.Command),
		}
		if err := validateSSHTool(spec); err != nil {
			return ToolCall{}, err
		}
		tc := ToolCall{
			Kind:           ToolSSH,
			ID:             id,
			SSHTool:        spec,
			DisplayCommand: spec.RemoteCommand,
		}
		tc.DisplayCommand = displayForTool(tc)
		return tc, nil

	case ToolRead:
		var input struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}
		if err := json.Unmarshal([]byte(args), &input); err != nil {
			return ToolCall{}, err
		}
		path := strings.TrimSpace(input.Path)
		if path == "" {
			return ToolCall{}, fmt.Errorf("read path is required")
		}
		tc := ToolCall{
			Kind:       ToolRead,
			ID:         id,
			ReadPath:   path,
			ReadOffset: input.Offset,
			ReadLimit:  input.Limit,
		}
		tc.DisplayCommand = displayForTool(tc)
		return tc, nil

	case ToolWrite:
		var input struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(args), &input); err != nil {
			return ToolCall{}, err
		}
		path := strings.TrimSpace(input.Path)
		if path == "" {
			return ToolCall{}, fmt.Errorf("write path is required")
		}
		tc := ToolCall{
			Kind:         ToolWrite,
			ID:           id,
			WritePath:    path,
			WriteContent: input.Content,
		}
		tc.DisplayCommand = displayForTool(tc)
		return tc, nil

	case ToolEdit:
		var input struct {
			Path  string     `json:"path"`
			Edits []TextEdit `json:"edits"`
		}
		if err := json.Unmarshal([]byte(args), &input); err != nil {
			return ToolCall{}, err
		}
		path := strings.TrimSpace(input.Path)
		if path == "" || len(input.Edits) == 0 {
			return ToolCall{}, fmt.Errorf("edit path and edits are required")
		}
		tc := ToolCall{
			Kind:     ToolEdit,
			ID:       id,
			EditPath: path,
			Edits:    input.Edits,
		}
		tc.DisplayCommand = displayForTool(tc)
		return tc, nil

	default:
		return ToolCall{}, fmt.Errorf("unknown tool %q", name)
	}
}
