package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/databufflabs/databuff-diag/internal/config"
	"github.com/databufflabs/databuff-diag/internal/policy"
)

const sessionFileMode = 0o600

// SessionMetaFilename is the metadata file inside each session directory.
const SessionMetaFilename = "session.json"

// MaxChatMessageRunes is the maximum number of Unicode code points in a user message.
const MaxChatMessageRunes = 100000

// SessionMessage is one turn in a diagnostic conversation.
type SessionMessage struct {
	ID          string              `json:"id"`
	Role        string              `json:"role"`
	Content     string              `json:"content"`
	Attachments []MessageAttachment `json:"attachments,omitempty"`
	Timestamp   time.Time           `json:"timestamp"`
	Command   string    `json:"command,omitempty"`
	Stdout    string    `json:"stdout,omitempty"`
	Stderr    string    `json:"stderr,omitempty"`
	ExitCode  *int      `json:"exit_code,omitempty"`
	Risk      string    `json:"risk,omitempty"`
}

// SSHOverride holds session-scoped credentials parsed from user messages.
type SSHOverride struct {
	Host     string `json:"host"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	Port     int    `json:"port,omitempty"`
}

// SSHToolPayload is the structured SSH tool stored on pending approvals.
type SSHToolPayload struct {
	HostID        string `json:"host_id,omitempty"`
	Host          string `json:"host,omitempty"`
	User          string `json:"user,omitempty"`
	Port          int    `json:"port,omitempty"`
	RemoteCommand string `json:"remote_command"`
}

// PendingApproval is a command waiting for human approval.
type PendingApproval struct {
	ID        string          `json:"id"`
	Command   string          `json:"command"`
	Risk      policy.Risk     `json:"risk"`
	Reason    string          `json:"reason,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	ToolKind  string          `json:"tool_kind,omitempty"`
	SSHTool   *SSHToolPayload `json:"ssh_tool,omitempty"`
}

// Session is a persisted diagnostic conversation.
type Session struct {
	ID               string                    `json:"id"`
	CreatedAt        time.Time                 `json:"created_at"`
	UpdatedAt        time.Time                 `json:"updated_at"`
	PolicyMode       policy.Mode               `json:"policy_mode"`
	Messages         []SessionMessage          `json:"messages"`
	PendingApprovals []PendingApproval         `json:"pending_approvals,omitempty"`
	SSHOverrides     map[string]SSHOverride    `json:"ssh_overrides,omitempty"`
}

// SessionStore persists each session under ~/.databuff-diag/sessions/<id>/:
// session.json holds metadata; other files in the same folder are workspace artifacts.
type SessionStore struct {
	dir string
}

// NewSessionStore resolves the default sessions directory.
func NewSessionStore() (*SessionStore, error) {
	home, err := config.HomeDir()
	if err != nil {
		return nil, err
	}
	return NewSessionStoreAt(filepath.Join(home, "sessions")), nil
}

// NewSessionStoreAt creates a store at an explicit directory (for tests).
func NewSessionStoreAt(dir string) *SessionStore {
	return &SessionStore{dir: dir}
}

// Dir returns the sessions directory path.
func (s *SessionStore) Dir() string {
	return s.dir
}

// WorkspaceDir returns the on-disk directory for a session (metadata + workspace files).
func (s *SessionStore) WorkspaceDir(id string) string {
	return s.sessionDir(id)
}

// EnsureWorkspaceDir creates the session directory if needed.
func (s *SessionStore) EnsureWorkspaceDir(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("session id is required")
	}
	if _, err := s.Load(id); err != nil {
		return "", err
	}
	dir := s.sessionDir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create session dir: %w", err)
	}
	return dir, nil
}

// Create initializes a new session and writes it to disk.
func (s *SessionStore) Create(policyMode policy.Mode) (*Session, error) {
	if policyMode == "" {
		policyMode = policy.Open
	}
	now := time.Now().UTC()
	session := &Session{
		ID:         newID(),
		CreatedAt:  now,
		UpdatedAt:  now,
		PolicyMode: policyMode,
		Messages:   []SessionMessage{},
	}
	if err := os.MkdirAll(s.sessionDir(session.ID), 0o700); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}
	if err := s.Save(session); err != nil {
		return nil, err
	}
	return session, nil
}

// List returns all sessions sorted by updated_at descending.
func (s *SessionStore) List() ([]*Session, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	seen := make(map[string]bool)
	var sessions []*Session
	for _, entry := range entries {
		var id string
		switch {
		case entry.IsDir():
			id = entry.Name()
		case strings.HasSuffix(entry.Name(), ".json"):
			id = strings.TrimSuffix(entry.Name(), ".json")
		default:
			continue
		}
		if id == "" || seen[id] {
			continue
		}
		session, err := s.Load(id)
		if err != nil {
			continue
		}
		seen[id] = true
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

// ValidateChatMessage checks user message length limits.
func ValidateChatMessage(content string) error {
	if len([]rune(content)) > MaxChatMessageRunes {
		return fmt.Errorf("消息不能超过 %d 个字符", MaxChatMessageRunes)
	}
	return nil
}

// SessionTitle derives a display title from the first user message.
func SessionTitle(session *Session) string {
	if session == nil {
		return "新对话"
	}
	for _, msg := range session.Messages {
		if msg.Role == "user" {
			title := strings.TrimSpace(msg.Content)
			if title == "" && len(msg.Attachments) > 0 {
				title = "[" + msg.Attachments[0].Filename + "]"
			}
			if title != "" {
				title = strings.ReplaceAll(title, "\n", " ")
				if len([]rune(title)) > 48 {
					return string([]rune(title)[:48]) + "…"
				}
				return title
			}
		}
	}
	return "新对话"
}

// Load reads a session by ID.
func (s *SessionStore) Load(id string) (*Session, error) {
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	data, err := os.ReadFile(s.metaPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			data, err = os.ReadFile(s.legacyMetaPath(id))
		}
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("session %q not found", id)
			}
			return nil, fmt.Errorf("read session: %w", err)
		}
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}
	return &session, nil
}

// Save writes the session JSON atomically.
func (s *SessionStore) Save(session *Session) error {
	if session == nil || session.ID == "" {
		return fmt.Errorf("session id is required")
	}
	session.UpdatedAt = time.Now().UTC()
	dir := s.sessionDir(session.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	tmp := filepath.Join(dir, SessionMetaFilename+".tmp")
	if err := os.WriteFile(tmp, data, sessionFileMode); err != nil {
		return fmt.Errorf("write session: %w", err)
	}
	if err := os.Rename(tmp, s.metaPath(session.ID)); err != nil {
		return fmt.Errorf("rename session: %w", err)
	}
	_ = os.Remove(s.legacyMetaPath(session.ID))
	return nil
}

func (s *SessionStore) sessionDir(id string) string {
	return filepath.Join(s.dir, id)
}

func (s *SessionStore) metaPath(id string) string {
	return filepath.Join(s.sessionDir(id), SessionMetaFilename)
}

func (s *SessionStore) legacyMetaPath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// SetPolicyMode updates the session policy mode and persists.
func (s *SessionStore) SetPolicyMode(id string, mode policy.Mode) (*Session, error) {
	if !policy.IsValidMode(mode) {
		return nil, fmt.Errorf("invalid policy_mode %q", mode)
	}
	session, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	if session.PolicyMode != mode {
		session.PolicyMode = mode
	}
	if err := s.Save(session); err != nil {
		return nil, err
	}
	return session, nil
}

// AppendMessage adds a message and persists the session.
func (s *SessionStore) AppendMessage(session *Session, msg SessionMessage) error {
	if msg.ID == "" {
		msg.ID = newID()
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	session.Messages = append(session.Messages, msg)
	return s.Save(session)
}

// PurgeBefore removes sessions whose UpdatedAt is strictly before cutoff.
func (s *SessionStore) PurgeBefore(cutoff time.Time) ([]string, error) {
	sessions, err := s.List()
	if err != nil {
		return nil, err
	}

	var purged []string
	for _, session := range sessions {
		if session == nil || !session.UpdatedAt.Before(cutoff) {
			continue
		}
		if err := s.Delete(session.ID); err != nil {
			return purged, err
		}
		purged = append(purged, session.ID)
	}
	return purged, nil
}

// Delete removes a session directory and any legacy flat metadata file.
func (s *SessionStore) Delete(id string) error {
	if id == "" {
		return fmt.Errorf("session id is required")
	}
	if err := os.RemoveAll(s.sessionDir(id)); err != nil {
		return fmt.Errorf("remove session %q: %w", id, err)
	}
	if err := os.Remove(s.legacyMetaPath(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove legacy session %q: %w", id, err)
	}
	return nil
}
