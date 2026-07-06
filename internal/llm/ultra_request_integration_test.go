package llm

import (
	"testing"

	"github.com/databufflabs/databuff-diag/internal/store"
)

func TestAdaptUltraMessages_BuildMessagesFromSessionFirstTurn(t *testing.T) {
	session := &store.Session{
		Messages: []store.SessionMessage{
			{Role: "user", Content: "你是谁"},
		},
	}
	sys := "你是 databuff-diag 排障助手"
	messages, err := BuildMessagesFromSession(session, sys, nil, false)
	if err != nil {
		t.Fatalf("BuildMessagesFromSession: %v", err)
	}

	adapted := AdaptUltraMessages(messages)
	if len(adapted) != 1 {
		t.Fatalf("len = %d, want 1; %#v", len(adapted), adapted)
	}
	if adapted[0].Role != "user" {
		t.Fatalf("role = %q, want user", adapted[0].Role)
	}
	text, ok := adapted[0].Content.(string)
	if !ok || text == "" {
		t.Fatalf("content = %v", adapted[0].Content)
	}
}

func TestAdaptUltraMessages_BuildMessagesFromSessionWithToolTurn(t *testing.T) {
	session := &store.Session{
		Messages: []store.SessionMessage{
			{Role: "user", Content: "检查 docker"},
			{Role: "assistant", Content: "执行查看"},
			{Role: "tool", Content: "CONTAINER ID ...", ToolCallID: "call-1", ToolName: "bash"},
		},
	}
	messages, err := BuildMessagesFromSession(session, "sys", nil, false)
	if err != nil {
		t.Fatalf("BuildMessagesFromSession: %v", err)
	}

	adapted := AdaptUltraMessages(messages)
	if adapted[len(adapted)-1].Role != "user" {
		t.Fatalf("last role = %q, want user", adapted[len(adapted)-1].Role)
	}
	for _, msg := range adapted {
		if msg.Role != "user" && msg.Role != "assistant" {
			t.Fatalf("unexpected role %q", msg.Role)
		}
	}
}
