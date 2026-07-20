package protectedrun

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/rewindbpf/rewind/internal/landlock"
	"github.com/rewindbpf/rewind/internal/overlay"
	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/runplan"
)

type fakeOverlay struct {
	mounted  int
	rollback int
}

func (f *fakeOverlay) Mount(context.Context, overlay.Layout) error {
	f.mounted++
	return nil
}

func (f *fakeOverlay) Rollback(context.Context, overlay.Layout) error {
	f.rollback++
	return nil
}

type fakeProcess struct {
	pid     uint32
	waitErr error
	killed  bool
}

func (p *fakeProcess) PID() uint32 { return p.pid }
func (p *fakeProcess) Wait() error { return p.waitErr }
func (p *fakeProcess) Kill() error {
	p.killed = true
	return nil
}

type fakeStarter struct {
	process *fakeProcess
	plan    *landlock.Plan
}

func (s *fakeStarter) Start(_ context.Context, _ []string, _ string, plan *landlock.Plan) (Process, error) {
	s.plan = plan
	return s.process, nil
}

type fakeSensor struct {
	closed bool
}

func (s *fakeSensor) Attach(context.Context, string, string, uint32) (io.Closer, error) {
	return closerFunc(func() error {
		s.closed = true
		return nil
	}), nil
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

func testPlan(t *testing.T) *runplan.Plan {
	t.Helper()
	plan, err := runplan.Build(runplan.Config{
		Workspace:   t.TempDir(),
		RuntimeRoot: t.TempDir(),
		Policy:      policy.Policy{Read: policy.ReadPolicy{Mode: policy.ModeOff}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &plan
}

func TestStartWaitAndRollbackLifecycle(t *testing.T) {
	overlayRunner := &fakeOverlay{}
	starter := &fakeStarter{process: &fakeProcess{pid: 42}}
	sensor := &fakeSensor{}
	coordinator := Coordinator{Overlay: overlayRunner, Starter: starter, Sensor: sensor}
	plan := testPlan(t)

	handle, err := coordinator.Start(context.Background(), plan, []string{"synthetic-agent"}, "trace.o")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Run.State != "running" || starter.plan != nil {
		t.Fatalf("run=%s starterPlan=%v", plan.Run.State, starter.plan)
	}
	if err := handle.Wait(); err != nil {
		t.Fatal(err)
	}
	if plan.Run.State != "succeeded" || !sensor.closed {
		t.Fatalf("run=%s sensorClosed=%v", plan.Run.State, sensor.closed)
	}
	if err := handle.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if plan.Run.State != "rolled_back" || overlayRunner.rollback != 1 {
		t.Fatalf("run=%s rollback=%d", plan.Run.State, overlayRunner.rollback)
	}
}

func TestWaitWithRunsBeforeSensorClose(t *testing.T) {
	overlayRunner := &fakeOverlay{}
	started := false
	starter := &fakeStarter{process: &fakeProcess{pid: 43}}
	sensor := &fakeSensor{}
	coordinator := Coordinator{Overlay: overlayRunner, Starter: starter, Sensor: sensor}
	plan := testPlan(t)
	handle, err := coordinator.Start(context.Background(), plan, []string{"synthetic-agent"}, "trace.o")
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.WaitWith(func() error {
		started = true
		if sensor.closed {
			return errors.New("sensor closed before drain callback")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !started || !sensor.closed {
		t.Fatalf("beforeClose=%v sensorClosed=%v", started, sensor.closed)
	}
}

func TestStartFailureRollsBackAndKillsAgent(t *testing.T) {
	overlayRunner := &fakeOverlay{}
	process := &fakeProcess{pid: 7, waitErr: errors.New("agent failed")}
	coordinator := Coordinator{Overlay: overlayRunner, Starter: &fakeStarter{process: process}}
	plan := testPlan(t)

	handle, err := coordinator.Start(context.Background(), plan, []string{"synthetic-agent"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.Wait(); err == nil {
		t.Fatal("expected agent failure")
	}
	if handle.plan.Run.State != "failed" {
		t.Fatalf("state = %s, want failed before rollback", handle.plan.Run.State)
	}
	if err := handle.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if handle.plan.Run.State != "rolled_back" || overlayRunner.rollback != 1 {
		t.Fatalf("state=%s rollback=%d", handle.plan.Run.State, overlayRunner.rollback)
	}
}
