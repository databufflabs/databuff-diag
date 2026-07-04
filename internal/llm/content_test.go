package llm

import (
	"strings"
	"testing"

	"github.com/databufflabs/databuff-diag/internal/store"
)

func TestMessageFromSession_ImageWithoutVision(t *testing.T) {
	home := t.TempDir()
	attachments := store.NewAttachmentStoreAt(home)

	meta, err := attachments.Save("chart.png", "image/png", strings.NewReader("fake-png"))
	if err != nil {
		t.Fatal(err)
	}

	msg := store.SessionMessage{
		Role:    "user",
		Content: "这个图片内容是什么",
		Attachments: []store.MessageAttachment{{
			ID:       meta.ID,
			Filename: meta.Filename,
			MimeType: meta.MimeType,
		}},
	}

	chatMsg, err := MessageFromSession(msg, attachments, false)
	if err != nil {
		t.Fatal(err)
	}

	text, ok := chatMsg.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", chatMsg.Content)
	}
	if strings.Contains(text, "image_url") {
		t.Fatalf("should not contain image_url: %q", text)
	}
	if !strings.Contains(text, "chart.png") {
		t.Fatalf("expected filename placeholder: %q", text)
	}
	if !strings.Contains(text, "does not support image input") {
		t.Fatalf("expected unsupported notice: %q", text)
	}
}

func TestMessageFromSession_ImageWithVision(t *testing.T) {
	home := t.TempDir()
	attachments := store.NewAttachmentStoreAt(home)

	meta, err := attachments.Save("chart.png", "image/png", strings.NewReader("fake-png"))
	if err != nil {
		t.Fatal(err)
	}

	msg := store.SessionMessage{
		Role:    "user",
		Content: "describe",
		Attachments: []store.MessageAttachment{{
			ID:       meta.ID,
			Filename: meta.Filename,
			MimeType: meta.MimeType,
		}},
	}

	chatMsg, err := MessageFromSession(msg, attachments, true)
	if err != nil {
		t.Fatal(err)
	}

	parts, ok := chatMsg.Content.([]ContentPart)
	if !ok {
		t.Fatalf("expected []ContentPart, got %T", chatMsg.Content)
	}
	if len(parts) != 2 {
		t.Fatalf("expected text + image parts, got %d", len(parts))
	}
	if parts[1].Type != "image_url" || parts[1].ImageURL == nil {
		t.Fatalf("expected image_url part: %+v", parts[1])
	}
}
