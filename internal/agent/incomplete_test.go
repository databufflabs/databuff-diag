package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/databufflabs/databuff-diag/internal/exec"
	"github.com/databufflabs/databuff-diag/internal/llm"
	"github.com/databufflabs/databuff-diag/internal/policy"
	"github.com/databufflabs/databuff-diag/internal/store"
)

func TestLooksIncompleteAssistant(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "ends with chinese colon",
			text: "内核版本过低。让我确认一下当前内核版本：",
			want: true,
		},
		{
			name: "ends with english colon",
			text: "Next, check the kernel version:",
			want: true,
		},
		{
			name: "接下来 phrase",
			text: "好的，接下来查看宿主机内核版本：",
			want: true,
		},
		{
			name: "complete conclusion",
			text: "根因是宿主机内核 3.10 不支持 RENAME EXCHANGE，需升级内核或跳过该迁移。",
			want: false,
		},
		{
			name: "tool json present is not tested here",
			text: `结论：容器已退出，建议升级内核。`,
			want: false,
		},
		{
			name: "ellipsis ending",
			text: "正在检查三个 Up 但未标记 healthy 的容器内部健康状态...",
			want: true,
		},
		{
			name: "final report not incomplete",
			text: "## 最终诊断结论\n\n" + strings.Repeat("x", 300),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksIncompleteAssistant(tt.text); got != tt.want {
				t.Fatalf("looksIncompleteAssistant(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestAgent_NudgesIncompleteAssistant(t *testing.T) {
	dir := t.TempDir()
	sessions := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessions.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ag := &Agent{
		LLM: &mockLLM{
			responses: []string{
				"让我确认一下当前内核版本：",
				`{"tool":"shell","command":"uname -r"}`,
				"内核版本为 3.10.0，低于 ClickHouse 迁移所需版本。",
			},
		},
		Policy:   &policy.Engine{},
		Executor: exec.NewLocal(exec.LocalConfig{}),
		Sessions: sessions,
	}

	provider := llm.MergedProvider{ProviderCode: "test", BaseURL: "http://test", Model: "m"}
	if err := ag.HandleUserMessage(context.Background(), session, "继续", provider); err != nil {
		t.Fatalf("HandleUserMessage: %v", err)
	}

	reloaded, err := sessions.Load(session.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var sawNudge, sawTool bool
	for _, msg := range reloaded.Messages {
		if msg.Role == "system" && msg.Content == incompleteNudgeMessage {
			sawNudge = true
		}
		if msg.Role == "tool" && msg.Command == "uname -r" {
			sawTool = true
		}
	}
	if !sawNudge {
		t.Fatal("expected incomplete nudge system message")
	}
	if !sawTool {
		t.Fatal("expected uname -r to run after nudge")
	}
}
