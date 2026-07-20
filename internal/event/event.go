// Package event defines the small, serializable event contract shared by
// kernel telemetry and the userspace runtime.
package event

import "fmt"

type Operation string

const (
	Execve         Operation = "execve"
	OpenAt         Operation = "openat"
	Read           Operation = "read"
	Write          Operation = "write"
	UnlinkAt       Operation = "unlinkat"
	RenameAt2      Operation = "renameat2"
	Truncate       Operation = "truncate"
	NetworkConnect Operation = "network_connect"
	Socket         Operation = "socket"
)

// Numeric operation codes are the stable wire values used by the eBPF ring
// buffer ABI. Keep them append-only so older userspace readers can continue to
// decode events emitted by a newer kernel program.
const (
	OperationCodeExecve uint32 = iota + 1
	OperationCodeOpenAt
	OperationCodeRead
	OperationCodeWrite
	OperationCodeUnlinkAt
	OperationCodeRenameAt2
	OperationCodeTruncate
	OperationCodeNetworkConnect
	OperationCodeSocket = 9
)

type Decision string

const (
	Allow Decision = "allow"
	Audit Decision = "audit"
	Deny  Decision = "deny"
)

const (
	DecisionCodeAllow uint32 = iota
	DecisionCodeAudit
	DecisionCodeDeny
)

type Risk string

const (
	Low    Risk = "low"
	Medium Risk = "medium"
	High   Risk = "high"
)

const (
	RiskCodeLow uint32 = iota + 1
	RiskCodeMedium
	RiskCodeHigh
)

func OperationCode(value Operation) (uint32, bool) {
	switch value {
	case Execve:
		return OperationCodeExecve, true
	case OpenAt:
		return OperationCodeOpenAt, true
	case Read:
		return OperationCodeRead, true
	case Write:
		return OperationCodeWrite, true
	case UnlinkAt:
		return OperationCodeUnlinkAt, true
	case RenameAt2:
		return OperationCodeRenameAt2, true
	case Truncate:
		return OperationCodeTruncate, true
	case NetworkConnect:
		return OperationCodeNetworkConnect, true
	case Socket:
		return OperationCodeSocket, true
	default:
		return 0, false
	}
}

func OperationFromCode(code uint32) (Operation, bool) {
	switch code {
	case OperationCodeExecve:
		return Execve, true
	case OperationCodeOpenAt:
		return OpenAt, true
	case OperationCodeRead:
		return Read, true
	case OperationCodeWrite:
		return Write, true
	case OperationCodeUnlinkAt:
		return UnlinkAt, true
	case OperationCodeRenameAt2:
		return RenameAt2, true
	case OperationCodeTruncate:
		return Truncate, true
	case OperationCodeNetworkConnect:
		return NetworkConnect, true
	case OperationCodeSocket:
		return Socket, true
	default:
		return "", false
	}
}

func DecisionCode(value Decision) (uint32, bool) {
	switch value {
	case Allow:
		return DecisionCodeAllow, true
	case Audit:
		return DecisionCodeAudit, true
	case Deny:
		return DecisionCodeDeny, true
	default:
		return 0, false
	}
}

func DecisionFromCode(code uint32) (Decision, bool) {
	switch code {
	case DecisionCodeAllow:
		return Allow, true
	case DecisionCodeAudit:
		return Audit, true
	case DecisionCodeDeny:
		return Deny, true
	default:
		return "", false
	}
}

func RiskCode(value Risk) (uint32, bool) {
	switch value {
	case Low:
		return RiskCodeLow, true
	case Medium:
		return RiskCodeMedium, true
	case High:
		return RiskCodeHigh, true
	default:
		return 0, false
	}
}

func RiskFromCode(code uint32) (Risk, bool) {
	switch code {
	case RiskCodeLow:
		return Low, true
	case RiskCodeMedium:
		return Medium, true
	case RiskCodeHigh:
		return High, true
	default:
		return "", false
	}
}

// Event is the userspace representation of one observed or enforced action.
// Kernel-side programs should emit only compact primitive fields; enrichment
// and persistence remain daemon responsibilities.
type Event struct {
	RunID       string    `json:"run_id"`
	PID         uint32    `json:"pid"`
	Operation   Operation `json:"operation"`
	Path        string    `json:"path,omitempty"`
	TimestampNS uint64    `json:"timestamp_ns"`
	Decision    Decision  `json:"decision"`
	Risk        Risk      `json:"risk"`
}

// Validate rejects malformed events before they enter the run event log.
func (e Event) Validate() error {
	if e.RunID == "" {
		return fmt.Errorf("event run id cannot be empty")
	}
	if e.PID == 0 {
		return fmt.Errorf("event pid cannot be zero")
	}
	if !validOperation(e.Operation) {
		return fmt.Errorf("event operation %q is not supported", e.Operation)
	}
	if e.TimestampNS == 0 {
		return fmt.Errorf("event timestamp cannot be zero")
	}
	if !validDecision(e.Decision) {
		return fmt.Errorf("event decision %q is not supported", e.Decision)
	}
	if !validRisk(e.Risk) {
		return fmt.Errorf("event risk %q is not supported", e.Risk)
	}
	return nil
}

func validOperation(value Operation) bool {
	switch value {
	case Execve, OpenAt, Read, Write, UnlinkAt, RenameAt2, Truncate, NetworkConnect, Socket:
		return true
	default:
		return false
	}
}

func validDecision(value Decision) bool {
	switch value {
	case Allow, Audit, Deny:
		return true
	default:
		return false
	}
}

func validRisk(value Risk) bool {
	switch value {
	case Low, Medium, High:
		return true
	default:
		return false
	}
}
