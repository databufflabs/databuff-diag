package server

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/databufflabs/databuff-diag/internal/exec"
	"github.com/databufflabs/databuff-diag/internal/policy"
)

type execLocalRequest struct {
	Command    string `json:"command"`
	TimeoutSec *int   `json:"timeout_sec,omitempty"`
}

type execLocalResponse struct {
	Command         string `json:"command"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	ExitCode        int    `json:"exit_code"`
	DurationMS      int64  `json:"duration_ms"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	TimedOut        bool   `json:"timed_out"`
	Risk            string `json:"risk"`
}

// ExecLocalHandler serves POST /api/exec/local.
type ExecLocalHandler struct {
	Executor *exec.Local
	Policy   *policy.Engine
}

func (h *ExecLocalHandler) executor() *exec.Local {
	if h.Executor != nil {
		return h.Executor
	}
	return exec.NewLocal(exec.LocalConfig{})
}

func (h *ExecLocalHandler) policy() *policy.Engine {
	if h.Policy != nil {
		return h.Policy
	}
	return &policy.Engine{}
}

func (h *ExecLocalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req execLocalRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}

	command := req.Command
	if command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	risk, err := h.policy().Classify(command)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if risk == policy.RiskBlocked {
		writeError(w, http.StatusForbidden, "command blocked by policy")
		return
	}

	cfg := exec.LocalConfig{}
	if req.TimeoutSec != nil && *req.TimeoutSec > 0 {
		cfg.Timeout = time.Duration(*req.TimeoutSec) * time.Second
	}
	runner := h.executor()
	if cfg.Timeout > 0 {
		runner = exec.NewLocal(exec.LocalConfig{
			Timeout:        cfg.Timeout,
			MaxOutputBytes: runner.MaxOutputBytes,
		})
	}

	result, err := runner.Run(r.Context(), command)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, execLocalResponse{
		Command:         command,
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
