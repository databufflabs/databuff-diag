package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/databufflabs/databuff-diag/internal/exec"
	"github.com/databufflabs/databuff-diag/internal/policy"
	"github.com/databufflabs/databuff-diag/internal/report"
	"github.com/databufflabs/databuff-diag/internal/sshresolve"
	"github.com/databufflabs/databuff-diag/internal/store"
	"github.com/go-chi/chi/v5"
)

type reportExportRequest struct {
	SessionID string `json:"session_id"`
}

// ReportExportHandler serves GET/POST /api/report/export.
type ReportExportHandler struct {
	SessionStore *store.SessionStore
}

func (h *ReportExportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPost:
	default:
		methodNotAllowed(w)
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	if r.Method == http.MethodPost && r.Body != nil {
		var req reportExportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if req.SessionID != "" {
			sessionID = req.SessionID
		}
	}
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	session, err := h.SessionStore.Load(sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	md := report.RenderMarkdown(session)
	filename := "report-" + sessionID + ".md"
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(md))
}

// EnvBundleHandler serves POST /api/collect/env-bundle.
type EnvBundleHandler struct {
	ConfigStore *store.ConfigStore
	ReportsDir  string
	Collector   *exec.EnvBundleCollector
}

func (h *EnvBundleHandler) reportsDir() string {
	if h.ReportsDir != "" {
		return h.ReportsDir
	}
	if h.ConfigStore != nil {
		return filepath.Join(filepath.Dir(h.ConfigStore.Path()), "reports")
	}
	return ""
}

func (h *EnvBundleHandler) collector(reportsDir string) *exec.EnvBundleCollector {
	if h.Collector != nil {
		return h.Collector
	}
	return exec.NewEnvBundleCollector(reportsDir, nil)
}

func (h *EnvBundleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	reportsDir := h.reportsDir()
	if reportsDir == "" {
		writeError(w, http.StatusInternalServerError, "reports directory unavailable")
		return
	}
	result, err := h.collector(reportsDir).Collect(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// EnvBundleDownloadHandler serves GET /api/collect/env-bundle/{filename}.
type EnvBundleDownloadHandler struct {
	ReportsDir string
}

func (h *EnvBundleDownloadHandler) reportsDir() string {
	if h.ReportsDir != "" {
		return h.ReportsDir
	}
	home, err := store.NewConfigStore()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(home.Path()), "reports")
}

func (h *EnvBundleDownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	filename := chi.URLParam(r, "filename")
	if filename == "" || strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	path := filepath.Join(h.reportsDir(), filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "bundle not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

type execSSHRequest struct {
	HostID     string `json:"host_id,omitempty"`
	Host       string `json:"host"`
	User       string `json:"user,omitempty"`
	Command    string `json:"command"`
	TimeoutSec *int   `json:"timeout_sec,omitempty"`
}

type execSSHResponse struct {
	Host            string `json:"host"`
	Command         string `json:"command"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	ExitCode        int    `json:"exit_code"`
	DurationMS      int64  `json:"duration_ms"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	TimedOut        bool   `json:"timed_out"`
	Risk            string `json:"risk"`
	Stub            bool   `json:"stub,omitempty"`
}

// ExecSSHHandler serves POST /api/exec/ssh (MVP via system ssh binary).
type ExecSSHHandler struct {
	Policy      *policy.Engine
	ConfigStore *store.ConfigStore
}

func (h *ExecSSHHandler) policy() *policy.Engine {
	if h.Policy != nil {
		return h.Policy
	}
	return &policy.Engine{}
}

func (h *ExecSSHHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req execSSHRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}
	if req.Host == "" && req.HostID == "" {
		writeError(w, http.StatusBadRequest, "host or host_id is required")
		return
	}
	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	risk, err := h.policy().Classify(req.Command)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if risk == policy.RiskBlocked {
		writeError(w, http.StatusForbidden, "command blocked by policy")
		return
	}

	var cfg *store.Config
	if h.ConfigStore != nil {
		cfg, err = h.ConfigStore.Load()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	resolved, err := sshresolve.Resolve(sshresolve.Request{
		HostID: req.HostID,
		Host:   req.Host,
		User:   req.User,
	}, cfg, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	cfgExec := exec.SSHConfig{Host: resolved.Host, User: resolved.User, Port: resolved.Port, Password: resolved.Password}
	if req.TimeoutSec != nil && *req.TimeoutSec > 0 {
		cfgExec.Timeout = time.Duration(*req.TimeoutSec) * time.Second
	}
	runner := exec.NewSSH(cfgExec)
	result, err := runner.Run(r.Context(), req.Command)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	displayCmd := sshresolve.DisplayCommand(resolved, req.Command)
	writeJSON(w, http.StatusOK, execSSHResponse{
		Host:            runner.Target(),
		Command:         displayCmd,
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		ExitCode:        result.ExitCode,
		DurationMS:      result.DurationMS,
		StdoutTruncated: result.StdoutTruncated,
		StderrTruncated: result.StderrTruncated,
		TimedOut:        result.TimedOut,
		Risk:            string(risk),
	})
}
