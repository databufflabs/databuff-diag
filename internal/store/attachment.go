package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxAttachmentBytes = 10 * 1024 * 1024 // 10 MB per file
	MaxTextEmbedBytes  = 32 * 1024        // 32 KB text embedded in LLM context
	attachmentFileMode = 0o600
	attachmentMetaFileMode = 0o600
)

const maxAttachmentBytes = MaxAttachmentBytes
const maxTextEmbedBytes = MaxTextEmbedBytes

// MessageAttachment is metadata for a file attached to a chat message.
type MessageAttachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

// AttachmentMeta is persisted metadata for an uploaded file.
type AttachmentMeta struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

// AttachmentStore persists uploaded files under ~/.databuff-diag/uploads/.
type AttachmentStore struct {
	dir string
}

// NewAttachmentStore resolves the default uploads directory.
func NewAttachmentStore() (*AttachmentStore, error) {
	home, err := HomeDirForStore()
	if err != nil {
		return nil, err
	}
	return NewAttachmentStoreAt(filepath.Join(home, "uploads")), nil
}

// NewAttachmentStoreAt creates a store at an explicit directory (for tests).
func NewAttachmentStoreAt(dir string) *AttachmentStore {
	return &AttachmentStore{dir: dir}
}

// Dir returns the uploads directory path.
func (s *AttachmentStore) Dir() string {
	return s.dir
}

// Save stores an uploaded file and returns its metadata.
func (s *AttachmentStore) Save(filename string, mimeType string, r io.Reader) (*AttachmentMeta, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, fmt.Errorf("create uploads dir: %w", err)
	}

	id := newAttachmentID()
	dir := filepath.Join(s.dir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create attachment dir: %w", err)
	}

	safeName := sanitizeFilename(filename)
	if safeName == "" {
		safeName = "file"
	}

	destPath := filepath.Join(dir, safeName)
	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, attachmentFileMode)
	if err != nil {
		return nil, fmt.Errorf("create attachment file: %w", err)
	}

	written, err := io.Copy(f, io.LimitReader(r, maxAttachmentBytes+1))
	_ = f.Close()
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("write attachment: %w", err)
	}
	if written > maxAttachmentBytes {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("file exceeds maximum size of %d bytes", maxAttachmentBytes)
	}

	meta := &AttachmentMeta{
		ID:       id,
		Filename: safeName,
		MimeType: mimeType,
		Size:     written,
	}
	metaPath := filepath.Join(dir, "meta.json")
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("marshal attachment meta: %w", err)
	}
	if err := os.WriteFile(metaPath, data, attachmentMetaFileMode); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("write attachment meta: %w", err)
	}
	return meta, nil
}

// LoadMeta reads attachment metadata by ID.
func (s *AttachmentStore) LoadMeta(id string) (*AttachmentMeta, error) {
	if id == "" {
		return nil, fmt.Errorf("attachment id is required")
	}
	metaPath := filepath.Join(s.dir, id, "meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("attachment %q not found", id)
		}
		return nil, fmt.Errorf("read attachment meta: %w", err)
	}
	var meta AttachmentMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse attachment meta: %w", err)
	}
	return &meta, nil
}

// Open returns a reader for the attachment file content.
func (s *AttachmentStore) Open(id string) (io.ReadCloser, *AttachmentMeta, error) {
	meta, err := s.LoadMeta(id)
	if err != nil {
		return nil, nil, err
	}
	path := filepath.Join(s.dir, id, meta.Filename)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("attachment %q not found", id)
		}
		return nil, nil, fmt.Errorf("open attachment: %w", err)
	}
	return f, meta, nil
}

// ReadAll reads the full attachment content (up to maxAttachmentBytes).
func (s *AttachmentStore) ReadAll(id string) ([]byte, *AttachmentMeta, error) {
	rc, meta, err := s.Open(id)
	if err != nil {
		return nil, nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxAttachmentBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("read attachment: %w", err)
	}
	return data, meta, nil
}

// ResolveMany loads metadata for the given attachment IDs.
func (s *AttachmentStore) ResolveMany(ids []string) ([]MessageAttachment, error) {
	out := make([]MessageAttachment, 0, len(ids))
	for _, id := range ids {
		meta, err := s.LoadMeta(id)
		if err != nil {
			return nil, err
		}
		out = append(out, MessageAttachment{
			ID:       meta.ID,
			Filename: meta.Filename,
			MimeType: meta.MimeType,
			Size:     meta.Size,
		})
	}
	return out, nil
}

// IsImageMime reports whether the MIME type is a supported image format.
func IsImageMime(mime string) bool {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

// IsTextMime reports whether the MIME type is treated as embeddable text.
func IsTextMime(mime string) bool {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if strings.HasPrefix(mime, "text/") {
		return true
	}
	switch mime {
	case "application/json", "application/yaml", "application/x-yaml",
		"application/xml", "application/javascript":
		return true
	default:
		return false
	}
}

// AllowedUploadMime reports whether the MIME type is allowed for upload.
func AllowedUploadMime(mime string) bool {
	return IsImageMime(mime) || IsTextMime(mime)
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return -1
		}
		return r
	}, name)
	return name
}

func newAttachmentID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// HomeDirForStore resolves ~/.databuff-diag for store packages.
func HomeDirForStore() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".databuff-diag"), nil
}
