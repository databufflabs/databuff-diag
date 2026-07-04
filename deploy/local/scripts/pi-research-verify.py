#!/usr/bin/env python3
"""Verify pi project research chat session quality."""
import json
import re
import sys


def analyze_session(data: dict) -> list[str]:
    issues = []
    msgs = data.get("messages") or []

    readonly_cmd_re = re.compile(
        r"\b(?:ls|cat|head|tail|grep|pwd|echo|uname|git\s+(?:log|remote|status|show))\b",
        re.I,
    )
    write_cmd_re = re.compile(
        r"\b(?:tee|npm\s+install|npm\s+run|write)\b|sed\s+-i|[>]{1,2}\s|"
        r"find\b[^\n]*\s-(?:exec|delete|ok)\b",
        re.I,
    )

    for i, m in enumerate(msgs):
        role = m.get("role")
        content = m.get("content") or ""
        cmd = m.get("command") or ""
        risk = m.get("risk") or ""

        if (
            role == "tool"
            and risk == "write"
            and cmd
            and readonly_cmd_re.search(cmd)
            and not write_cmd_re.search(cmd)
        ):
            issues.append(f"msg[{i}]: readonly command misclassified as write: {cmd[:80]}")

        if role != "assistant" or not content:
            continue

        fence_count = len(re.findall(r"```", content))
        if fence_count % 2 != 0:
            issues.append(f"msg[{i}]: unbalanced code fences ({fence_count})")

        if re.search(r"\n?```(?:json|bash|sh|shell)?\s*\n?\s*$", content):
            issues.append(f"msg[{i}]: trailing orphan fence opener")

        if cmd and re.search(r"[├└│]", cmd):
            issues.append(f"msg[{i}]: directory tree misclassified as command")

        if role == "assistant" and not content.strip() and not m.get("tool_calls"):
            issues.append(f"msg[{i}]: empty assistant without tool_calls")

        if role == "assistant" and content.strip() in ("将执行命令", "将执行命令：") and m.get("tool_calls"):
            issues.append(f"msg[{i}]: generic execute lead with native tool_calls")

    for i, m in enumerate(msgs):
        if m.get("role") == "system" and "tool JSON" in (m.get("content") or ""):
            issues.append(f"msg[{i}]: incomplete-response nudge")

    if not msgs:
        issues.append("empty session")
        return issues

    last = msgs[-1]
    if last.get("role") != "assistant":
        issues.append(f"final: last role is {last.get('role')}, not assistant")
        return issues

    lc = (last.get("content") or "").strip()
    if len(lc) < 150:
        issues.append(f"final: summary too short ({len(lc)} chars)")

    if not (re.search(r"##\s+", lc) or "总结" in lc or "概览" in lc):
        issues.append("final: missing summary headings or keywords")

    if lc.startswith("---") and not re.search(r"^---\s*\n+\s*##\s+", lc):
        issues.append("final: starts with horizontal rule without heading")

    if re.search(r"^---\s*\n+\s*##\s+", lc):
        pass  # valid markdown HR before summary sections

    if "将执行命令" in lc and "## " not in lc:
        issues.append("final: stuck in command-proposal mode")

    if last.get("command") and re.search(r"[├└│]", last.get("command") or ""):
        issues.append("final: has tree diagram as pending command")

    pending = data.get("pending_approvals") or []
    if pending:
        issues.append(f"pending_approvals: {len(pending)}")

    return issues


def main() -> int:
    data = json.load(sys.stdin)
    issues = analyze_session(data)
    sid = data.get("id", "?")
    msgs = data.get("messages") or []
    last = msgs[-1] if msgs else {}
    preview = ((last.get("content") or "")[:160]).replace("\n", " ")

    print(json.dumps({
        "session_id": sid,
        "msg_count": len(msgs),
        "last_preview": preview,
        "issues": issues or ["ok"],
    }, ensure_ascii=False))

    for i, m in enumerate(msgs):
        if m.get("role") == "assistant":
            c = (m.get("content") or "")[:120].replace("\n", " ")
            cmd = (m.get("command") or "")[:80]
            print(f"  assistant[{i}] cmd={cmd!r} text={c!r}")

    if issues:
        print("\n=== FINAL MESSAGE ===")
        print((last.get("content") or "")[:3000])
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
