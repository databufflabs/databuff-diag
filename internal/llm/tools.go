package llm

// ToolDefinition is an OpenAI-compatible function tool schema.
type ToolDefinition struct {
	Type     string             `json:"type"`
	Function ToolFunctionSchema `json:"function"`
}

// ToolFunctionSchema describes a callable tool for the model.
type ToolFunctionSchema struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// AgentToolNames are the tools exposed to the model (Pi core + ssh).
var AgentToolNames = []string{"read", "write", "edit", "bash", "ssh"}

// AgentTools returns OpenAI function tool definitions aligned with Pi's built-in tools.
func AgentTools() []ToolDefinition {
	return []ToolDefinition{
		readToolDef(),
		writeToolDef(),
		editToolDef(),
		bashToolDef(),
		sshToolDef(),
	}
}

func readToolDef() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolFunctionSchema{
			Name: "read",
			Description: "Read the contents of a file. Supports text files. " +
				"For large files, use offset/limit (1-indexed line numbers). " +
				"Paths may be relative to the session workspace or absolute within it.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the file to read (relative or absolute within workspace)",
					},
					"offset": map[string]any{
						"type":        "integer",
						"description": "Line number to start reading from (1-indexed)",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of lines to read",
					},
				},
				"required": []string{"path"},
			},
		},
	}
}

func writeToolDef() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolFunctionSchema{
			Name:        "write",
			Description: "Create or overwrite a file in the session workspace.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the file to write (relative or absolute within workspace)",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Content to write to the file",
					},
				},
				"required": []string{"path", "content"},
			},
		},
	}
}

func editToolDef() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolFunctionSchema{
			Name: "edit",
			Description: "Edit a file using exact text replacement. Each edits[].oldText must match a unique, " +
				"non-overlapping region of the original file. If two changes affect the same block or nearby lines, " +
				"merge them into one edit instead of emitting overlapping edits.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the file to edit (relative or absolute within workspace)",
					},
					"edits": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"oldText": map[string]any{
									"type":        "string",
									"description": "Exact text for one targeted replacement (unique in file)",
								},
								"newText": map[string]any{
									"type":        "string",
									"description": "Replacement text",
								},
							},
							"required": []string{"oldText", "newText"},
						},
						"description": "One or more targeted replacements matched against the original file",
					},
				},
				"required": []string{"path", "edits"},
			},
		},
	}
}

func bashToolDef() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolFunctionSchema{
			Name:        "bash",
			Description: "Execute a bash command on the local machine. Returns stdout and stderr.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Bash command to execute",
					},
					"timeout": map[string]any{
						"type":        "integer",
						"description": "Timeout in seconds (optional)",
					},
				},
				"required": []string{"command"},
			},
		},
	}
}

func sshToolDef() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolFunctionSchema{
			Name: "ssh",
			Description: "Run a command on a remote host via SSH. Use host_id for saved hosts; " +
				"passwords for saved hosts are injected by the system.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"host_id": map[string]any{
						"type":        "string",
						"description": "Saved SSH host id (preferred)",
					},
					"host": map[string]any{
						"type":        "string",
						"description": "Remote host IP or hostname",
					},
					"user": map[string]any{
						"type":        "string",
						"description": "SSH username",
					},
					"port": map[string]any{
						"type":        "integer",
						"description": "SSH port",
					},
					"command": map[string]any{
						"type":        "string",
						"description": "Remote shell command to execute",
					},
				},
				"required": []string{"command"},
			},
		},
	}
}
