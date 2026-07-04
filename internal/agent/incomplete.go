package agent

import (
	"strings"
	"unicode"
)

const incompleteNudgeMessage = "你的上一条回复描述了下一步操作，但没有调用工具。请使用 read/write/edit/bash/ssh 工具继续诊断；若诊断已完成，请给出完整结论，不要用「接下来将…」等过渡语结尾。"

const inlineScriptNudgeMessage = `检测到将完整 shell 脚本作为 bash 命令直接执行，这不会保存为文件。请使用 write 工具创建脚本，或 bash 配合重定向写入会话工作区，例如：
{"tool":"bash","command":"cat > check.sh << 'EOF'\n#!/bin/bash\n...\nEOF\nchmod +x check.sh"}`

const emptyResponseNudgeMessage = "上一条回复为空。请根据对话上下文继续诊断：给出 tool JSON 执行下一步命令，或输出完整结论。"

// looksIncompleteAssistant reports assistant text that promises a next action
// but did not include a tool JSON block (common cause of "half-finished" replies).
func looksIncompleteAssistant(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if looksLikeFinalReport(text) {
		return false
	}

	runes := []rune(text)
	last := runes[len(runes)-1]
	if last == ':' || last == '：' {
		return true
	}

	tail := text
	if len(tail) > 100 {
		tail = tail[len(tail)-100:]
	}
	tail = strings.TrimSpace(tail)
	tailLower := strings.ToLower(tail)

	phrases := []string{
		"接下来", "现在先", "现在我来", "我来将", "我将", "让我确认", "让我查看", "让我检查",
		"需要先", "准备查看", "准备检查", "将执行", "来查看", "写入工作目录", "将脚本写入",
		"进一步检查", "正在检查", "正在查看", "正在获取", "让我先总结",
		"next step", "let me check", "let me run", "i will run", "i'll run", "i will write",
	}
	for _, p := range phrases {
		if strings.Contains(tail, p) || strings.Contains(tailLower, p) {
			return true
		}
	}
	// Also scan opening intent when the reply is long but still promises more work.
	opening := text
	if len(opening) > 200 {
		opening = opening[:200]
	}
	for _, p := range []string{"进一步检查", "然后对", "让我先总结", "正在检查"} {
		if strings.Contains(opening, p) {
			return true
		}
	}

	if strings.HasSuffix(strings.TrimSpace(text), "...") || strings.HasSuffix(strings.TrimSpace(text), "…") {
		return true
	}

	// Short reply that only sets up an action without substance.
	if len([]rune(text)) < 120 && strings.ContainsAny(text, "查看检查确认执行") {
		for _, w := range []string{"查看", "检查", "确认", "执行"} {
			if strings.HasSuffix(strings.TrimSpace(text), w) ||
				strings.HasSuffix(strings.TrimSpace(text), w+"：") ||
				strings.HasSuffix(strings.TrimSpace(text), w+":") {
				return true
			}
		}
	}

	return endsWithContinuation(text)
}

func looksLikeFinalReport(text string) bool {
	if len(text) < 200 {
		return false
	}
	lower := strings.ToLower(text)
	if strings.Contains(text, "## ") && (strings.Contains(text, "结论") || strings.Contains(text, "诊断") ||
		strings.Contains(text, "总结") || strings.Contains(text, "调研") || strings.Contains(lower, "summary")) {
		return true
	}
	return strings.HasPrefix(text, "## ") || strings.HasPrefix(strings.TrimSpace(text), "---\n\n## ")
}

func looksMalformedToolJSON(text string) bool {
	trimmed := strings.TrimSpace(text)
	if !strings.Contains(trimmed, `"tool"`) {
		return false
	}
	_, ok := ParseTool(trimmed)
	return !ok
}

func endsWithContinuation(text string) bool {
	text = strings.TrimRightFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || r == '.' || r == '。'
	})
	continuations := []string{
		"如下", "如下所示", "as follows", "following",
	}
	for _, c := range continuations {
		if strings.HasSuffix(text, c) {
			return true
		}
	}
	return false
}
