package agent

import "testing"

func TestParseTool_BareShellCommand(t *testing.T) {
	tool, ok := ParseTool("docker ps -a")
	if !ok || tool.ShellCommand != "docker ps -a" {
		t.Fatalf("ParseTool bare = %+v, %v", tool, ok)
	}
}

func TestLooksMalformedToolJSON(t *testing.T) {
	broken := `{"tool":"shell","command":"docker ps -a --format 'table {{.Names}}\t{{.Status}}\t{{.Health}}'" 2>/dev/null"}`
	if !looksMalformedToolJSON(broken) {
		t.Fatal("expected malformed tool json")
	}
}
