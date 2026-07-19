package lifecycle

import "testing"

func TestNewStartsPreparing(t *testing.T) {
	run, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == "" {
		t.Fatal("expected run id")
	}
	if run.State != Preparing {
		t.Fatalf("state = %q, want %q", run.State, Preparing)
	}
	if run.CreatedAt.IsZero() || run.UpdatedAt.IsZero() {
		t.Fatal("expected lifecycle timestamps")
	}
}

func TestSuccessfulRunCanBeCommitted(t *testing.T) {
	run, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, next := range []State{Mounted, Running, Succeeded, Committed} {
		if err := run.Transition(next); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}
	if run.State != Committed {
		t.Fatalf("state = %q, want %q", run.State, Committed)
	}
}

func TestFailedRunMustBeRolledBackBeforeTerminalState(t *testing.T) {
	run, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, next := range []State{Mounted, Running, Failed, RolledBack} {
		if err := run.Transition(next); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}
	if err := run.Transition(Running); err == nil {
		t.Fatal("expected terminal rolled-back state to reject transition")
	}
}

func TestInvalidTransitionsAreRejected(t *testing.T) {
	run, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, next := range []State{Committed, Paused, State("unknown")} {
		if err := run.Transition(next); err == nil {
			t.Fatalf("expected transition to %q to fail", next)
		}
	}
}

func TestPausedRunCanResumeOrRollback(t *testing.T) {
	run, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, next := range []State{Mounted, Running, Paused, Running, Succeeded, RolledBack} {
		if err := run.Transition(next); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}
}
