package server

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/databufflabs/databuff-diag/internal/store"
	"github.com/go-chi/chi/v5"
)

func TestUpload_SingleFile(t *testing.T) {
	home := t.TempDir()
	attachmentStore := store.NewAttachmentStoreAt(filepath.Join(home, "uploads"))
	handler := &UploadHandler{Attachments: attachmentStore}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "error.log")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("connection refused\n"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "error.log") {
		t.Fatalf("expected filename in response: %s", rec.Body.String())
	}
}

func TestAttachment_Serve(t *testing.T) {
	home := t.TempDir()
	attachmentStore := store.NewAttachmentStoreAt(filepath.Join(home, "uploads"))
	meta, err := attachmentStore.Save("test.txt", "text/plain", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}

	handler := &AttachmentHandler{Attachments: attachmentStore}
	req := httptest.NewRequest(http.MethodGet, "/api/attachments/"+meta.ID, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", meta.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}
