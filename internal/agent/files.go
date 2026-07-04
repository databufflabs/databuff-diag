package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/databufflabs/databuff-diag/internal/store"
)

const (
	maxReadFileBytes = 512 * 1024
	maxReadLines     = 2000
)

// resolveWorkspacePath resolves a path relative to the session workspace root.
func resolveWorkspacePath(workspace, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	return safeWorkspacePath(workspace, clean)
}

// resolveReadPath resolves a read target: workspace-relative paths stay in the
// session workspace; absolute paths may read any host file (except session metadata).
func resolveReadPath(workspace, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if filepath.IsAbs(clean) {
		if err := blockSessionMetaPath(clean); err != nil {
			return "", err
		}
		return clean, nil
	}
	return safeWorkspacePath(workspace, clean)
}

func safeWorkspacePath(root, rel string) (string, error) {
	clean := filepath.Clean(rel)
	rootClean := filepath.Clean(root)

	var abs string
	if filepath.IsAbs(clean) {
		abs = clean
	} else {
		if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return "", fmt.Errorf("path outside workspace")
		}
		abs = filepath.Clean(filepath.Join(root, clean))
	}

	if abs != rootClean && !strings.HasPrefix(abs, rootClean+string(os.PathSeparator)) {
		return "", fmt.Errorf("path outside workspace")
	}
	if err := blockSessionMetaPath(abs); err != nil {
		return "", err
	}
	return abs, nil
}

func blockSessionMetaPath(abs string) error {
	if filepath.Base(abs) == store.SessionMetaFilename {
		return fmt.Errorf("cannot access session metadata file")
	}
	return nil
}

func readWorkspaceFile(workspace, path string, offset, limit int) (string, error) {
	abs, err := resolveReadPath(workspace, path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory")
	}
	if info.Size() > maxReadFileBytes {
		return "", fmt.Errorf("file too large to read (max %d bytes)", maxReadFileBytes)
	}

	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxReadFileBytes+1))
	if err != nil {
		return "", err
	}
	if !isTextContent(data) {
		return "", fmt.Errorf("binary file cannot be read as text")
	}

	lines := strings.Split(string(data), "\n")
	total := len(lines)

	start := 1
	if offset > 0 {
		start = offset
	}
	end := total
	if limit > 0 {
		end = start + limit - 1
	}
	if start > total {
		return fmt.Sprintf("(file has %d lines; offset %d is past end)", total, start), nil
	}
	if end > total {
		end = total
	}

	var b strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "%6d|%s\n", i, lines[i-1])
	}
	header := fmt.Sprintf("File: %s (%d lines total)\n", path, total)
	if start > 1 || end < total {
		header += fmt.Sprintf("Showing lines %d-%d\n", start, end)
	}
	return header + b.String(), nil
}

func writeWorkspaceFile(workspace, path, content string) error {
	abs, err := resolveWorkspacePath(workspace, path)
	if err != nil {
		return err
	}
	data := []byte(content)
	if len(data) > maxReadFileBytes {
		return fmt.Errorf("content too large (max %d bytes)", maxReadFileBytes)
	}
	if !isTextContent(data) {
		return fmt.Errorf("binary content cannot be written")
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	return os.WriteFile(abs, data, 0o600)
}

func editWorkspaceFile(workspace, path string, edits []TextEdit) (string, error) {
	abs, err := resolveWorkspacePath(workspace, path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	content := string(data)
	if !isTextContent(data) {
		return "", fmt.Errorf("binary file cannot be edited")
	}

	normalized := strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	ending := "\n"
	if strings.Contains(content, "\r\n") {
		ending = "\r\n"
	}

	type match struct {
		index  int
		length int
		newText string
		editIdx int
	}
	var matches []match
	for i, edit := range edits {
		old := strings.ReplaceAll(strings.ReplaceAll(edit.OldText, "\r\n", "\n"), "\r", "\n")
		if old == "" {
			return "", fmt.Errorf("edits[%d].oldText must not be empty", i)
		}
		count := strings.Count(normalized, old)
		if count == 0 {
			return "", fmt.Errorf("edits[%d].oldText not found in %s", i, path)
		}
		if count > 1 {
			return "", fmt.Errorf("edits[%d].oldText appears %d times in %s (must be unique)", i, count, path)
		}
		idx := strings.Index(normalized, old)
		newText := strings.ReplaceAll(strings.ReplaceAll(edit.NewText, "\r\n", "\n"), "\r", "\n")
		matches = append(matches, match{index: idx, length: len(old), newText: newText, editIdx: i})
	}

	for i := 1; i < len(matches); i++ {
		prev := matches[i-1]
		cur := matches[i]
		if prev.index+prev.length > cur.index {
			return "", fmt.Errorf("edits[%d] and edits[%d] overlap in %s", prev.editIdx, cur.editIdx, path)
		}
	}

	// Apply from end to start so indices stay valid.
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		normalized = normalized[:m.index] + m.newText + normalized[m.index+m.length:]
	}

	out := normalized
	if ending == "\r\n" {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	if err := os.WriteFile(abs, []byte(out), 0o600); err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully edited %s (%d change(s))", path, len(edits)), nil
}

func isTextContent(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}
