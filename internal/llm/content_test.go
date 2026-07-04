package llm

import (
	"strings"
	"testing"

	"github.com/databufflabs/databuff-diag/internal/store"
)

func TestMessageFromSession_ImageWithoutVision(t *testing.T) {
	home := t.TempDir()
	attachments := store.NewAttachmentStoreAt(home)

	meta, err := attachments.Save("chart.png", "image/png", strings.NewReader("fake-png"))
	if err != nil {
		t.Fatal(err)
	}

	msg := store.SessionMessage{
		Role:    "user",
		Content: "这个图片内容是什么",
		Attachments: []store.MessageAttachment{{
			ID:       meta.ID,
			Filename: meta.Filename,
			MimeType: meta.MimeType,
		}},
	}

	chatMsg, err := MessageFromSession(msg, attachments, false)
	if err != nil {
		t.Fatal(err)
	}

	text, ok := chatMsg.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", chatMsg.Content)
	}
	if strings.Contains(text, "image_url") {
		t.Fatalf("should not contain image_url: %q", text)
	}
	if !strings.Contains(text, "chart.png") {
		t.Fatalf("expected filename placeholder: %q", text)
	}
	if !strings.Contains(text, "does not support image input") {
		t.Fatalf("expected unsupported notice: %q", text)
	}
}

func TestMessageFromSession_ImageWithVision(t *testing.T) {
	home := t.TempDir()
	attachments := store.NewAttachmentStoreAt(home)

	meta, err := attachments.Save("chart.png", "image/png", strings.NewReader("fake-png"))
	if err != nil {
		t.Fatal(err)
	}

	msg := store.SessionMessage{
		Role:    "user",
		Content: "describe",
		Attachments: []store.MessageAttachment{{
			ID:       meta.ID,
			Filename: meta.Filename,
			MimeType: meta.MimeType,
		}},
	}

	chatMsg, err := MessageFromSession(msg, attachments, true)
	if err != nil {
		t.Fatal(err)
	}

	parts, ok := chatMsg.Content.([]ContentPart)
	if !ok {
		t.Fatalf("expected []ContentPart, got %T", chatMsg.Content)
	}
	if len(parts) != 2 {
		t.Fatalf("expected text + image parts, got %d", len(parts))
	}
	if parts[1].Type != "image_url" || parts[1].ImageURL == nil {
		t.Fatalf("expected image_url part: %+v", parts[1])
	}
}

func TestRepairToolCallSequences_FillsMissingToolResults(t *testing.T) {
	messages := []store.SessionMessage{
		{Role: "user", Content: "go"},
		{
			Role: "assistant",
			ToolCalls: []store.StoredToolCall{
				{ID: "call_a", Name: "bash", Arguments: `{"command":"echo 1"}`},
				{ID: "call_b", Name: "read", Arguments: `{"path":"/tmp/x"}`},
			},
		},
		{Role: "tool", Content: "ok", ToolCallID: "call_a", ToolName: "bash"},
		{Role: "user", Content: "next"},
	}

	repaired := RepairToolCallSequences(messages)
	if len(repaired) != 5 {
		t.Fatalf("len = %d, want 5", len(repaired))
	}
	if repaired[3].Role != "tool" || repaired[3].ToolCallID != "call_b" {
		t.Fatalf("synthetic tool = %+v", repaired[3])
	}
	if repaired[3].Content != interruptedToolResult {
		t.Fatalf("content = %q", repaired[3].Content)
	}
	if repaired[4].Role != "user" || repaired[4].Content != "next" {
		t.Fatalf("user tail = %+v", repaired[4])
	}
}

func TestRepairToolCallSequences_NoChangeWhenComplete(t *testing.T) {
	messages := []store.SessionMessage{
		{
			Role: "assistant",
			ToolCalls: []store.StoredToolCall{
				{ID: "call_a", Name: "bash", Arguments: `{}`},
			},
		},
		{Role: "tool", Content: "ok", ToolCallID: "call_a", ToolName: "bash"},
	}
	repaired := RepairToolCallSequences(messages)
	if len(repaired) != len(messages) {
		t.Fatalf("len = %d, want %d", len(repaired), len(messages))
	}
}

func TestMessageFromSession_AssistantToolCallsWireFormat(t *testing.T) {
	msg := store.SessionMessage{
		Role: "assistant",
		ToolCalls: []store.StoredToolCall{{
			ID:        "call_abc",
			Name:      "bash",
			Arguments: `{"command":"docker ps"}`,
		}},
	}
	chatMsg, err := MessageFromSession(msg, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(chatMsg.ToolCalls) != 1 {
		t.Fatalf("tool_calls = %+v", chatMsg.ToolCalls)
	}
	tc := chatMsg.ToolCalls[0]
	if tc.Type != "function" {
		t.Fatalf("type = %q, want function", tc.Type)
	}
	if tc.Function.Name != "bash" || tc.Function.Arguments == "" {
		t.Fatalf("function = %+v", tc.Function)
	}
}
