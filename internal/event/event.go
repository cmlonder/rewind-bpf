// Package event defines the small, serializable event contract shared by
// kernel telemetry and the userspace runtime.
package event

import "fmt"

type Operation string

const (
	Execve    Operation = "execve"
	OpenAt    Operation = "openat"
	Read      Operation = "read"
	Write     Operation = "write"
	UnlinkAt  Operation = "unlinkat"
	RenameAt2 Operation = "renameat2"
	Truncate  Operation = "truncate"
)

type Decision string

const (
	Allow Decision = "allow"
	Audit Decision = "audit"
	Deny  Decision = "deny"
)

type Risk string

const (
	Low    Risk = "low"
	Medium Risk = "medium"
	High   Risk = "high"
)

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
	case Execve, OpenAt, Read, Write, UnlinkAt, RenameAt2, Truncate:
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
