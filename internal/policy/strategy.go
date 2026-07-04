package policy

// Mode is the session-level approval policy.
type Mode string

const (
	// AllApproval requires human approval for every command (most conservative).
	AllApproval Mode = "all_approval"
	// WriteApproval auto-runs readonly commands; write and dangerous need approval.
	WriteApproval Mode = "write_approval"
	// Open auto-runs readonly and write; only dangerous needs approval (default).
	Open Mode = "open"
)

// Label returns a short Chinese label for mode.
func Label(mode Mode) string {
	switch mode {
	case AllApproval:
		return "全部审批"
	case WriteApproval:
		return "写入审批"
	case Open:
		return "全部开放"
	default:
		return string(mode)
	}
}

// IsValidMode reports whether mode is a known policy mode.
func IsValidMode(mode Mode) bool {
	switch mode {
	case AllApproval, WriteApproval, Open:
		return true
	default:
		return false
	}
}

// NeedsApproval reports whether a command with the given risk requires human
// approval under mode. Blocked commands are never approvable.
func NeedsApproval(risk Risk, mode Mode) bool {
	if risk == RiskBlocked {
		return false
	}

	switch mode {
	case AllApproval:
		return true
	case Open:
		return risk == RiskDangerous
	case WriteApproval:
		return risk == RiskWrite || risk == RiskDangerous
	default:
		return risk == RiskWrite || risk == RiskDangerous
	}
}
