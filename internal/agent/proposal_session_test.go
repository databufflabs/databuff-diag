package agent

import (
	"strings"
	"testing"
)

func TestProposalText_Session225ceb(t *testing.T) {
	intro := "我无法直接通过 SSH 连接到远程主机 `192.168.50.140`，因为我的运行环境只能在本机执行 shell 命令。以下是在当前主机查看容器列表的命令，如果当前主机就是目标机器，输出会直接显示；否则请手动在目标机器上运行：\n\n"
	cmd := `docker ps -a --format 'table {{.Names}}\t{{.Status}}\t{{.State}}'`
	full := intro + "```json\n" + `{"tool":"shell","command":"` + cmd + `"}` + "\n```"
	got := ProposalText(full, cmd)
	if strings.Contains(got, "```") {
		t.Fatalf("ProposalText still has fence: %q", got)
	}
}

func TestSanitizeProposalContent_BrokenSessionPayload(t *testing.T) {
	broken := "我无法直接通过 SSH 连接到远程主机 `192.168.50.140`，因为我的运行环境只能在本机执行 shell 命令。以下是在当前主机查看容器列表的命令，如果当前主机就是目标机器，输出会直接显示；否则请手动在目标机器上运行：\n\n```json"
	got := sanitizeProposalContent(broken)
	if strings.Contains(got, "```") {
		t.Fatalf("sanitizeProposalContent = %q", got)
	}
}
