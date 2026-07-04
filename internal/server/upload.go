package server

import (
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/databufflabs/databuff-diag/internal/store"
	"github.com/go-chi/chi/v5"
)

const maxUploadFiles = 5

// UploadHandler serves POST /api/upload for chat attachments.
type UploadHandler struct {
	Attachments *store.AttachmentStore
}

// AttachmentHandler serves GET /api/attachments/{id}.
type AttachmentHandler struct {
	Attachments *store.AttachmentStore
}

func (h *UploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	if err := r.ParseMultipartForm(store.MaxAttachmentBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	if len(files) > maxUploadFiles {
		writeError(w, http.StatusBadRequest, "too many files (max 5)")
		return
	}

	type uploadResult struct {
		ID       string `json:"id"`
		Filename string `json:"filename"`
		MimeType string `json:"mime_type"`
		Size     int64  `json:"size"`
		URL      string `json:"url"`
	}

	results := make([]uploadResult, 0, len(files))
	for _, header := range files {
		mimeType := detectMimeType(header.Filename, header.Header.Get("Content-Type"))
		if !store.AllowedUploadMime(mimeType) {
			writeError(w, http.StatusBadRequest, "unsupported file type: "+mimeType)
			return
		}

		rc, err := header.Open()
		if err != nil {
			writeError(w, http.StatusBadRequest, "cannot read uploaded file")
			return
		}

		meta, err := h.Attachments.Save(header.Filename, mimeType, rc)
		_ = rc.Close()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		results = append(results, uploadResult{
			ID:       meta.ID,
			Filename: meta.Filename,
			MimeType: meta.MimeType,
			Size:     meta.Size,
			URL:      "/api/attachments/" + meta.ID,
		})
	}

	if len(results) == 1 {
		writeJSON(w, http.StatusCreated, results[0])
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"files": results})
}

func (h *AttachmentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	id := chi.URLParam(r, "id")
	rc, meta, err := h.Attachments.Open(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", meta.MimeType)
	w.Header().Set("Content-Disposition", "inline; filename=\""+meta.Filename+"\"")
	_, _ = io.Copy(w, rc)
}

func detectMimeType(filename, headerType string) string {
	if headerType != "" && headerType != "application/octet-stream" {
		return strings.Split(headerType, ";")[0]
	}
	ext := filepath.Ext(filename)
	if ext != "" {
		if mt := mime.TypeByExtension(ext); mt != "" {
			return mt
		}
	}
	return "application/octet-stream"
}
