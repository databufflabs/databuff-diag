package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/databufflabs/databuff-diag/internal/shell"
	"github.com/databufflabs/databuff-diag/internal/store"
)

const (
	maxWorkspaceFileBytes   = 512 * 1024
	maxWorkspaceUploadBytes = 10 * 1024 * 1024 // 10 MB per uploaded file
	maxWorkspaceUploadFiles = 10
)

var workspaceSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	".databuff-diag": true, "dist": true,
}

// WorkspaceHandler serves per-session workspace file tree and content APIs.
type WorkspaceHandler struct {
	SessionStore *store.SessionStore
}

type workspaceEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

type workspaceTreeResponse struct {
	Root     string           `json:"root"`
	Path     string           `json:"path"`
	Entries  []workspaceEntry `json:"entries"`
	Parent   string           `json:"parent,omitempty"`
	ReadOnly bool             `json:"read_only"`
}

type workspaceFileResponse struct {
	Path        string             `json:"path"`
	Content     string             `json:"content"`
	Truncated   bool               `json:"truncated,omitempty"`
	ReadOnly    bool               `json:"read_only"`
	Diagnostics []shell.Diagnostic `json:"diagnostics,omitempty"`
}

type workspaceWriteRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type workspaceLintRequest struct {
	Content string `json:"content"`
}

type workspaceLintResponse struct {
	Diagnostics []shell.Diagnostic `json:"diagnostics,omitempty"`
}

type workspaceUploadEntry struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type workspaceUploadResponse struct {
	Files []workspaceUploadEntry `json:"files"`
}

// Info handles GET /api/workspace?session_id=...
func (h *WorkspaceHandler) Info(w http.ResponseWriter, r *http.Request) {
	root, sessionID, err := h.resolveRoot(r)
	if err != nil {
		status := http.StatusInternalServerError
		if isWorkspaceClientError(err) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"root":       root,
		"session_id": sessionID,
	})
}

// Tree handles GET /api/workspace/tree?session_id=...
func (h *WorkspaceHandler) Tree(w http.ResponseWriter, r *http.Request) {
	root, _, err := h.resolveRoot(r)
	if err != nil {
		status := http.StatusInternalServerError
		if isWorkspaceClientError(err) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	rel := strings.TrimPrefix(r.URL.Query().Get("path"), "/")
	abs, err := safeWorkspacePath(root, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	info, err := os.Stat(abs)
	if err != nil {
		writeError(w, http.StatusNotFound, "path not found")
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is not a directory")
		return
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := make([]workspaceEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && name != "." {
			continue
		}
		if name == store.SessionMetaFilename {
			continue
		}
		if entry.IsDir() && workspaceSkipDirs[name] {
			continue
		}
		childRel := name
		if rel != "" {
			childRel = filepath.ToSlash(filepath.Join(rel, name))
		}
		out = append(out, workspaceEntry{
			Name:  name,
			Path:  childRel,
			IsDir: entry.IsDir(),
		})
	}

	parent := ""
	if rel != "" {
		parent = filepath.ToSlash(filepath.Dir(rel))
		if parent == "." {
			parent = ""
		}
	}

	writeJSON(w, http.StatusOK, workspaceTreeResponse{
		Root:     root,
		Path:     rel,
		Entries:  out,
		Parent:   parent,
		ReadOnly: false,
	})
}

// File handles GET /api/workspace/file?session_id=...
func (h *WorkspaceHandler) File(w http.ResponseWriter, r *http.Request) {
	root, _, err := h.resolveRoot(r)
	if err != nil {
		status := http.StatusInternalServerError
		if isWorkspaceClientError(err) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	rel := strings.TrimPrefix(r.URL.Query().Get("path"), "/")
	if rel == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	resp, status, err := readWorkspaceFile(root, rel)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// CreateFile handles POST /api/workspace/file?session_id=...
func (h *WorkspaceHandler) CreateFile(w http.ResponseWriter, r *http.Request) {
	root, _, err := h.resolveRoot(r)
	if err != nil {
		status := http.StatusInternalServerError
		if isWorkspaceClientError(err) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	var req workspaceWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	rel := strings.TrimPrefix(strings.TrimSpace(req.Path), "/")
	if rel == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if err := validateWorkspaceFilePath(rel); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	abs, err := safeWorkspacePath(root, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := os.Stat(abs); err == nil {
		writeError(w, http.StatusConflict, "file already exists")
		return
	} else if !os.IsNotExist(err) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := writeWorkspaceFileContent(abs, req.Content); err != nil {
		status := http.StatusInternalServerError
		if isWorkspaceWriteClientError(err) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	resp, status, err := readWorkspaceFile(root, rel)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

// UpdateFile handles PUT /api/workspace/file?session_id=...
func (h *WorkspaceHandler) UpdateFile(w http.ResponseWriter, r *http.Request) {
	root, _, err := h.resolveRoot(r)
	if err != nil {
		status := http.StatusInternalServerError
		if isWorkspaceClientError(err) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	rel := strings.TrimPrefix(r.URL.Query().Get("path"), "/")
	if rel == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if err := validateWorkspaceFilePath(rel); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req workspaceWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	abs, err := safeWorkspacePath(root, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is a directory")
		return
	}

	if err := writeWorkspaceFileContent(abs, req.Content); err != nil {
		status := http.StatusInternalServerError
		if isWorkspaceWriteClientError(err) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	resp, status, err := readWorkspaceFile(root, rel)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// LintFile handles POST /api/workspace/lint?session_id=...&path=...
// Parse-only syntax check for in-editor content; does not read or write the file.
func (h *WorkspaceHandler) LintFile(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.resolveRoot(r); err != nil {
		status := http.StatusInternalServerError
		if isWorkspaceClientError(err) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	rel := strings.TrimPrefix(r.URL.Query().Get("path"), "/")
	if rel == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	var req workspaceLintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if len(req.Content) > maxWorkspaceFileBytes {
		writeError(w, http.StatusBadRequest, "content too large")
		return
	}

	diagnostics := shell.Lint(req.Content, rel)
	writeJSON(w, http.StatusOK, workspaceLintResponse{Diagnostics: diagnostics})
}

// UploadFiles handles POST /api/workspace/upload?session_id=...&path=...
// Optional path query selects the target directory (relative to workspace root).
func (h *WorkspaceHandler) UploadFiles(w http.ResponseWriter, r *http.Request) {
	root, _, err := h.resolveRoot(r)
	if err != nil {
		status := http.StatusInternalServerError
		if isWorkspaceClientError(err) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	dirRel := strings.TrimPrefix(r.URL.Query().Get("path"), "/")
	dirAbs := root
	if dirRel != "" {
		var dirErr error
		dirAbs, dirErr = safeWorkspacePath(root, dirRel)
		if dirErr != nil {
			writeError(w, http.StatusBadRequest, dirErr.Error())
			return
		}
		info, statErr := os.Stat(dirAbs)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				writeError(w, http.StatusNotFound, "directory not found")
				return
			}
			writeError(w, http.StatusInternalServerError, statErr.Error())
			return
		}
		if !info.IsDir() {
			writeError(w, http.StatusBadRequest, "path is not a directory")
			return
		}
	}

	if err := r.ParseMultipartForm(maxWorkspaceUploadBytes * 2); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	headers := r.MultipartForm.File["file"]
	if len(headers) == 0 {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	if len(headers) > maxWorkspaceUploadFiles {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("too many files (max %d)", maxWorkspaceUploadFiles))
		return
	}

	out := make([]workspaceUploadEntry, 0, len(headers))
	for _, header := range headers {
		name := sanitizeWorkspaceFilename(header.Filename)
		if name == "" {
			writeError(w, http.StatusBadRequest, "invalid filename")
			return
		}

		rel := name
		if dirRel != "" {
			rel = filepath.ToSlash(filepath.Join(dirRel, name))
		}
		if err := validateWorkspaceFilePath(rel); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		abs, pathErr := safeWorkspacePath(root, rel)
		if pathErr != nil {
			writeError(w, http.StatusBadRequest, pathErr.Error())
			return
		}

		if _, statErr := os.Stat(abs); statErr == nil {
			writeError(w, http.StatusConflict, "file already exists: "+rel)
			return
		} else if !os.IsNotExist(statErr) {
			writeError(w, http.StatusInternalServerError, statErr.Error())
			return
		}

		rc, openErr := header.Open()
		if openErr != nil {
			writeError(w, http.StatusBadRequest, "cannot read uploaded file")
			return
		}

		written, writeErr := writeWorkspaceUploadedFile(abs, rc)
		_ = rc.Close()
		if writeErr != nil {
			status := http.StatusInternalServerError
			if isWorkspaceUploadClientError(writeErr) {
				status = http.StatusBadRequest
			}
			writeError(w, status, writeErr.Error())
			return
		}

		out = append(out, workspaceUploadEntry{
			Path: rel,
			Name: name,
			Size: written,
		})
	}

	writeJSON(w, http.StatusCreated, workspaceUploadResponse{Files: out})
}

// DeleteFile handles DELETE /api/workspace/file?session_id=...
func (h *WorkspaceHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	root, _, err := h.resolveRoot(r)
	if err != nil {
		status := http.StatusInternalServerError
		if isWorkspaceClientError(err) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	rel := strings.TrimPrefix(r.URL.Query().Get("path"), "/")
	if rel == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if err := validateWorkspaceFilePath(rel); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	abs, err := safeWorkspacePath(root, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "cannot delete a directory")
		return
	}

	if err := os.Remove(abs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"path": rel})
}

func readWorkspaceFile(root, rel string) (workspaceFileResponse, int, error) {
	abs, err := safeWorkspacePath(root, rel)
	if err != nil {
		return workspaceFileResponse{}, http.StatusBadRequest, err
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return workspaceFileResponse{}, http.StatusNotFound, fmt.Errorf("file not found")
		}
		return workspaceFileResponse{}, http.StatusInternalServerError, err
	}
	if info.IsDir() {
		return workspaceFileResponse{}, http.StatusBadRequest, fmt.Errorf("path is a directory")
	}
	if info.Size() > maxWorkspaceFileBytes {
		return workspaceFileResponse{}, http.StatusBadRequest, fmt.Errorf("file too large to preview")
	}

	f, err := os.Open(abs)
	if err != nil {
		return workspaceFileResponse{}, http.StatusInternalServerError, err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxWorkspaceFileBytes+1))
	if err != nil {
		return workspaceFileResponse{}, http.StatusInternalServerError, err
	}
	if !isTextContent(data) {
		return workspaceFileResponse{}, http.StatusBadRequest, fmt.Errorf("binary file cannot be previewed")
	}

	truncated := len(data) > maxWorkspaceFileBytes
	if truncated {
		data = data[:maxWorkspaceFileBytes]
	}

	content := string(data)
	var diagnostics []shell.Diagnostic
	readOnly := truncated
	if !truncated {
		// Syntax check only (AST parse); preview must never execute the script.
		diagnostics = shell.Lint(content, rel)
	}

	return workspaceFileResponse{
		Path:        rel,
		Content:     content,
		Truncated:   truncated,
		ReadOnly:    readOnly,
		Diagnostics: diagnostics,
	}, http.StatusOK, nil
}

func writeWorkspaceUploadedFile(abs string, r io.Reader) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return 0, fmt.Errorf("create parent directory: %w", err)
	}

	f, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return 0, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	written, err := io.Copy(f, io.LimitReader(r, maxWorkspaceUploadBytes+1))
	if err != nil {
		_ = os.Remove(abs)
		return 0, fmt.Errorf("write file: %w", err)
	}
	if written > maxWorkspaceUploadBytes {
		_ = os.Remove(abs)
		return 0, errWorkspaceUploadTooLarge
	}
	return written, nil
}

func sanitizeWorkspaceFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return -1
		}
		return r
	}, name)
	if name == "." || name == ".." {
		return ""
	}
	return name
}

func writeWorkspaceFileContent(abs, content string) error {
	data := []byte(content)
	if len(data) > maxWorkspaceFileBytes {
		return errWorkspaceFileTooLarge
	}
	if !isTextContent(data) {
		return errWorkspaceBinaryFile
	}

	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	if err := os.WriteFile(abs, data, 0o600); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func validateWorkspaceFilePath(rel string) error {
	if rel == "" || rel == "." {
		return errWorkspaceInvalidPath
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return errWorkspaceInvalidPath
	}
	base := filepath.Base(clean)
	if base == "." || base == ".." || base == "" {
		return errWorkspaceInvalidPath
	}
	if base == store.SessionMetaFilename {
		return errWorkspaceInvalidPath
	}
	return nil
}

func isWorkspaceWriteClientError(err error) bool {
	return err == errWorkspaceFileTooLarge || err == errWorkspaceBinaryFile
}

func isWorkspaceUploadClientError(err error) bool {
	return err == errWorkspaceUploadTooLarge
}

func (h *WorkspaceHandler) resolveRoot(r *http.Request) (string, string, error) {
	if h.SessionStore == nil {
		return "", "", fmt.Errorf("session store is not configured")
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		return "", "", errSessionIDRequired
	}
	root, err := h.SessionStore.EnsureWorkspaceDir(sessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return "", "", errSessionNotFound
		}
		return "", "", err
	}
	return filepath.Clean(root), sessionID, nil
}

func isWorkspaceClientError(err error) bool {
	return err == errSessionIDRequired || err == errSessionNotFound
}

func safeWorkspacePath(root, rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errPathOutsideWorkspace
	}
	abs := filepath.Join(root, clean)
	abs = filepath.Clean(abs)
	rootClean := filepath.Clean(root)
	if abs != rootClean && !strings.HasPrefix(abs, rootClean+string(os.PathSeparator)) {
		return "", errPathOutsideWorkspace
	}
	return abs, nil
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

var (
	errPathOutsideWorkspace  = &pathError{msg: "path outside workspace"}
	errSessionIDRequired     = &pathError{msg: "session_id is required"}
	errSessionNotFound       = &pathError{msg: "session not found"}
	errWorkspaceInvalidPath  = &pathError{msg: "invalid file path"}
	errWorkspaceFileTooLarge   = &pathError{msg: "file too large"}
	errWorkspaceBinaryFile     = &pathError{msg: "binary file cannot be written"}
	errWorkspaceUploadTooLarge = &pathError{msg: fmt.Sprintf("file exceeds maximum size of %d bytes", maxWorkspaceUploadBytes)}
)

type pathError struct{ msg string }

func (e *pathError) Error() string { return e.msg }
