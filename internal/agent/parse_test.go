package agent

import (
	"strings"
	"testing"
)

func TestParseCommand_JSON(t *testing.T) {
	text := `I'll check disk usage.

{"tool":"shell","command":"df -h"}`

	cmd, ok := ParseCommand(text)
	if !ok {
		t.Fatal("expected command")
	}
	if cmd != "df -h" {
		t.Fatalf("command = %q, want df -h", cmd)
	}
}

func TestParseCommand_BashFence(t *testing.T) {
	text := "Run this:\n```bash\necho hello\n```"
	cmd, ok := ParseCommand(text)
	if !ok {
		t.Fatal("expected command")
	}
	if cmd != "echo hello" {
		t.Fatalf("command = %q, want echo hello", cmd)
	}
}

func TestParseCommand_None(t *testing.T) {
	if _, ok := ParseCommand("no command here"); ok {
		t.Fatal("expected no command")
	}
}

func TestParseCommand_ContainerNameListRejected(t *testing.T) {
	text := `建议检查以下容器：

` + "```bash\nai-apm-demo\nai-apm-ingest\nai-apm-web\n```"

	if _, ok := ParseCommand(text); ok {
		t.Fatal("expected container name list to be rejected")
	}
}

func TestParseCommand_JSONContainerNameListRejected(t *testing.T) {
	text := `{"tool":"shell","command":"ai-apm-demo\nai-apm-ingest"}`

	if _, ok := ParseCommand(text); ok {
		t.Fatal("expected bare container names in JSON to be rejected")
	}
}

func TestParseTool_SSH(t *testing.T) {
	text := `Checking remote docker: {"tool":"ssh","host_id":"host-abc","command":"docker ps -a"}`
	tool, ok := ParseTool(text)
	if !ok {
		t.Fatal("expected ssh tool")
	}
	if tool.Kind != ToolSSH || tool.SSHTool == nil {
		t.Fatalf("tool = %+v, want ssh", tool)
	}
	if tool.SSHTool.HostID != "host-abc" || tool.SSHTool.RemoteCommand != "docker ps -a" {
		t.Fatalf("ssh tool = %+v", tool.SSHTool)
	}
}

func TestParseTool_SSHByHost(t *testing.T) {
	text := `{"tool":"ssh","host":"10.0.0.1","user":"root","command":"uname -a"}`
	tool, ok := ParseTool(text)
	if !ok {
		t.Fatal("expected ssh tool")
	}
	if tool.SSHTool.Host != "10.0.0.1" || tool.SSHTool.User != "root" {
		t.Fatalf("ssh tool = %+v", tool.SSHTool)
	}
}

func TestParseCommand_DockerInspectAccepted(t *testing.T) {
	text := `{"tool":"shell","command":"docker inspect ai-apm-demo --format '{{.State.Health.Status}}'"}`

	cmd, ok := ParseCommand(text)
	if !ok {
		t.Fatal("expected docker inspect command")
	}
	if !strings.Contains(cmd, "docker inspect") {
		t.Fatalf("command = %q, want docker inspect", cmd)
	}
}

func TestProposalText_StripsFabricatedOutput(t *testing.T) {
	cmd := `docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Image}}"`
	full := "已重新获取最新的容器列表：\n\n```bash\n" + cmd + "\n```\n\n**输出结果：**\n\n| NAMES | STATUS |\n"

	got := ProposalText(full, cmd)
	want := "已重新获取最新的容器列表："
	if got != want {
		t.Fatalf("ProposalText = %q, want %q", got, want)
	}
}

func TestProposalText_JSONTool(t *testing.T) {
	cmd := "docker ps"
	full := "检查容器：\n\n{\"tool\":\"shell\",\"command\":\"docker ps\"}\n\n结果如下…"
	got := ProposalText(full, cmd)
	if got != "检查容器：" {
		t.Fatalf("ProposalText = %q", got)
	}
}

func TestProposalText_JSONFencedTool(t *testing.T) {
	cmd := "docker ps"
	full := "好的，再次获取当前所有容器的状态。\n\n```json\n{\"tool\":\"shell\",\"command\":\"docker ps\"}\n```\n\n结果如下"
	got := ProposalText(full, cmd)
	want := "好的，再次获取当前所有容器的状态。"
	if got != want {
		t.Fatalf("ProposalText = %q, want %q", got, want)
	}
}

func TestProposalText_NoIntroUsesShortLead(t *testing.T) {
	cmd := "docker ps"
	full := "{\"tool\":\"shell\",\"command\":\"docker ps\"}"
	got := ProposalText(full, cmd)
	if got != "将执行命令" {
		t.Fatalf("ProposalText = %q, want 将执行命令", got)
	}
}
