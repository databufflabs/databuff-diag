package server

import (
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/databufflabs/databuff-diag/internal/policy"
	"github.com/databufflabs/databuff-diag/internal/store"
)

func TestWorkspaceHandler_RequiresSessionID(t *testing.T) {
	dir := t.TempDir()
	sessionStore := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	handler := &WorkspaceHandler{SessionStore: sessionStore}

	req := httptest.NewRequest(http.MethodGet, "/api/workspace", nil)
	rec := httptest.NewRecorder()
	handler.Info(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestWorkspaceHandler_SessionScopedTree(t *testing.T) {
	dir := t.TempDir()
	sessionStore := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessionStore.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	workspaceDir := sessionStore.WorkspaceDir(session.ID)
	if err := os.WriteFile(filepath.Join(workspaceDir, "report.md"), []byte("# report"), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	handler := &WorkspaceHandler{SessionStore: sessionStore}
	req := httptest.NewRequest(http.MethodGet, "/api/workspace/tree?session_id="+session.ID, nil)
	rec := httptest.NewRecorder()
	handler.Tree(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp workspaceTreeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Root != workspaceDir {
		t.Fatalf("root = %q, want %q", resp.Root, workspaceDir)
	}
	if len(resp.Entries) != 1 || resp.Entries[0].Name != "report.md" {
		t.Fatalf("entries = %+v", resp.Entries)
	}
}

func TestWorkspaceHandler_FileShellDiagnostics(t *testing.T) {
	dir := t.TempDir()
	sessionStore := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessionStore.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	workspaceDir := sessionStore.WorkspaceDir(session.ID)
	script := "#!/bin/bash\nif true; then\n  echo ok\n"
	if err := os.WriteFile(filepath.Join(workspaceDir, "bad.sh"), []byte(script), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	handler := &WorkspaceHandler{SessionStore: sessionStore}
	req := httptest.NewRequest(http.MethodGet, "/api/workspace/file?session_id="+session.ID+"&path=bad.sh", nil)
	rec := httptest.NewRecorder()
	handler.File(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp workspaceFileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want 1", resp.Diagnostics)
	}
	if resp.Diagnostics[0].Message == "" {
		t.Fatal("expected diagnostic message")
	}
}

func TestWorkspaceHandler_FileShellPreviewDoesNotExecute(t *testing.T) {
	dir := t.TempDir()
	sessionStore := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessionStore.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	workspaceDir := sessionStore.WorkspaceDir(session.ID)
	marker := filepath.Join(workspaceDir, "preview-ran")
	script := "#!/bin/sh\ntouch " + marker + "\necho done\n"
	if err := os.WriteFile(filepath.Join(workspaceDir, "run.sh"), []byte(script), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	handler := &WorkspaceHandler{SessionStore: sessionStore}
	req := httptest.NewRequest(http.MethodGet, "/api/workspace/file?session_id="+session.ID+"&path=run.sh", nil)
	rec := httptest.NewRecorder()
	handler.File(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp workspaceFileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Content != script {
		t.Fatalf("content = %q, want original script", resp.Content)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("preview API must not execute shell script")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat marker: %v", err)
	}
}

func TestWorkspaceHandler_CreateUpdateDeleteFile(t *testing.T) {
	dir := t.TempDir()
	sessionStore := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessionStore.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	handler := &WorkspaceHandler{SessionStore: sessionStore}
	sessionQ := "?session_id=" + session.ID

	createBody := `{"path":"notes.txt","content":"hello"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/workspace/file"+sessionQ, strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.CreateFile(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	updateBody := `{"content":"updated"}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/workspace/file"+sessionQ+"&path=notes.txt", strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.UpdateFile(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateRec.Code, updateRec.Body.String())
	}

	var updated workspaceFileResponse
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if updated.Content != "updated" {
		t.Fatalf("content = %q, want %q", updated.Content, "updated")
	}
	if updated.ReadOnly {
		t.Fatal("expected editable file")
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/workspace/file"+sessionQ+"&path=notes.txt", nil)
	deleteRec := httptest.NewRecorder()
	handler.DeleteFile(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteRec.Code, deleteRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/workspace/file"+sessionQ+"&path=notes.txt", nil)
	getRec := httptest.NewRecorder()
	handler.File(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestWorkspaceHandler_UploadFiles(t *testing.T) {
	dir := t.TempDir()
	sessionStore := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessionStore.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	handler := &WorkspaceHandler{SessionStore: sessionStore}
	sessionQ := "?session_id=" + session.ID

	var body strings.Builder
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "notes.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("uploaded content")); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/workspace/upload"+sessionQ, strings.NewReader(body.String()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	handler.UploadFiles(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp workspaceUploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode upload: %v", err)
	}
	if len(resp.Files) != 1 || resp.Files[0].Path != "notes.txt" {
		t.Fatalf("upload files = %+v", resp.Files)
	}

	workspaceDir := sessionStore.WorkspaceDir(session.ID)
	data, err := os.ReadFile(filepath.Join(workspaceDir, "notes.txt"))
	if err != nil {
		t.Fatalf("read uploaded file: %v", err)
	}
	if string(data) != "uploaded content" {
		t.Fatalf("content = %q, want %q", string(data), "uploaded content")
	}

	dupReq := httptest.NewRequest(http.MethodPost, "/api/workspace/upload"+sessionQ, strings.NewReader(body.String()))
	dupReq.Header.Set("Content-Type", mw.FormDataContentType())
	dupRec := httptest.NewRecorder()
	handler.UploadFiles(dupRec, dupReq)
	if dupRec.Code != http.StatusConflict {
		t.Fatalf("duplicate upload status = %d, want %d", dupRec.Code, http.StatusConflict)
	}
}

func TestWorkspaceHandler_LintFile(t *testing.T) {
	dir := t.TempDir()
	sessionStore := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessionStore.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	handler := &WorkspaceHandler{SessionStore: sessionStore}
	sessionQ := "?session_id=" + session.ID + "&path=check.sh"

	validBody := `{"content":"#!/bin/bash\necho ok\n"}`
	validReq := httptest.NewRequest(http.MethodPost, "/api/workspace/lint"+sessionQ, strings.NewReader(validBody))
	validReq.Header.Set("Content-Type", "application/json")
	validRec := httptest.NewRecorder()
	handler.LintFile(validRec, validReq)
	if validRec.Code != http.StatusOK {
		t.Fatalf("valid lint status = %d, body = %s", validRec.Code, validRec.Body.String())
	}
	var validResp workspaceLintResponse
	if err := json.Unmarshal(validRec.Body.Bytes(), &validResp); err != nil {
		t.Fatalf("decode valid lint: %v", err)
	}
	if len(validResp.Diagnostics) != 0 {
		t.Fatalf("valid lint diagnostics = %+v, want none", validResp.Diagnostics)
	}

	invalidBody := `{"content":"#!/bin/bash\nif true; then\n  echo ok\n"}`
	invalidReq := httptest.NewRequest(http.MethodPost, "/api/workspace/lint"+sessionQ, strings.NewReader(invalidBody))
	invalidReq.Header.Set("Content-Type", "application/json")
	invalidRec := httptest.NewRecorder()
	handler.LintFile(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusOK {
		t.Fatalf("invalid lint status = %d, body = %s", invalidRec.Code, invalidRec.Body.String())
	}
	var invalidResp workspaceLintResponse
	if err := json.Unmarshal(invalidRec.Body.Bytes(), &invalidResp); err != nil {
		t.Fatalf("decode invalid lint: %v", err)
	}
	if len(invalidResp.Diagnostics) != 1 {
		t.Fatalf("invalid lint diagnostics = %+v, want 1", invalidResp.Diagnostics)
	}
}
