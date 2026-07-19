// Package lifecycle owns the state machine for one protected agent run.
// It deliberately does not start processes, mount filesystems, or evaluate
// policy rules; those responsibilities belong to separate packages.
package lifecycle

import (
	"fmt"
	"time"

	"github.com/rewindbpf/rewind/internal/runid"
)

type State string

const (
	Preparing  State = "preparing"
	Mounted    State = "mounted"
	Running    State = "running"
	Paused     State = "paused"
	Succeeded  State = "succeeded"
	Failed     State = "failed"
	Committed  State = "committed"
	RolledBack State = "rolled_back"
)

// Run is the serializable lifecycle record for a protected agent execution.
type Run struct {
	ID        string    `json:"run_id"`
	State     State     `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// New creates a run in the preparing state. No filesystem or process is
// touched by this function.
func New() (Run, error) {
	id, err := runid.New()
	if err != nil {
		return Run{}, err
	}
	now := time.Now().UTC()
	return Run{ID: id, State: Preparing, CreatedAt: now, UpdatedAt: now}, nil
}

// Transition advances a run through an explicitly allowed lifecycle edge.
func (r *Run) Transition(next State) error {
	if r == nil {
		return fmt.Errorf("transition run: nil run")
	}
	if r.ID == "" {
		return fmt.Errorf("transition run: missing run id")
	}
	if !validState(next) {
		return fmt.Errorf("transition run %s: unknown target state %q", r.ID, next)
	}
	if !canTransition(r.State, next) {
		return fmt.Errorf("transition run %s: cannot move from %s to %s", r.ID, r.State, next)
	}
	r.State = next
	r.UpdatedAt = time.Now().UTC()
	return nil
}

func validState(state State) bool {
	switch state {
	case Preparing, Mounted, Running, Paused, Succeeded, Failed, Committed, RolledBack:
		return true
	default:
		return false
	}
}

func canTransition(from, to State) bool {
	switch from {
	case Preparing:
		return to == Mounted || to == Failed || to == RolledBack
	case Mounted:
		return to == Running || to == Failed || to == RolledBack
	case Running:
		return to == Paused || to == Succeeded || to == Failed
	case Paused:
		return to == Running || to == RolledBack
	case Succeeded:
		return to == Committed || to == RolledBack
	case Failed:
		return to == RolledBack
	default:
		return false
	}
}
