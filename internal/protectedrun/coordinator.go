// Package protectedrun owns the effectful lifecycle around one run plan.
// Platform-specific process helpers and eBPF adapters implement the small
// interfaces defined here; this package owns ordering and fail-closed cleanup.
package protectedrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/rewindbpf/rewind/internal/landlock"
	"github.com/rewindbpf/rewind/internal/lifecycle"
	"github.com/rewindbpf/rewind/internal/overlay"
	"github.com/rewindbpf/rewind/internal/runplan"
)

type Overlay interface {
	Mount(context.Context, overlay.Layout) error
	Rollback(context.Context, overlay.Layout) error
}

type Process interface {
	PID() uint32
	Wait() error
	Kill() error
}

// Starter must apply the supplied Landlock plan before the agent command can
// execute. A starter that cannot honor a non-nil plan must return an error.
type Starter interface {
	Start(context.Context, []string, string, *landlock.Plan) (Process, error)
}

type Sensor interface {
	Attach(context.Context, string, string, uint32) (io.Closer, error)
}

type Coordinator struct {
	Overlay Overlay
	Starter Starter
	Sensor  Sensor
}

type Handle struct {
	coordinator *Coordinator
	plan        *runplan.Plan
	process     Process
	sensor      io.Closer
	waited      bool
}

func (c Coordinator) Start(ctx context.Context, plan *runplan.Plan, command []string, sensorObject string) (*Handle, error) {
	if plan == nil {
		return nil, fmt.Errorf("start protected run: plan is nil")
	}
	if c.Overlay == nil || c.Starter == nil {
		return nil, fmt.Errorf("start protected run: overlay and process starter are required")
	}
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return nil, fmt.Errorf("start protected run: command cannot be empty")
	}
	if plan.Run.State != lifecycle.Preparing {
		return nil, fmt.Errorf("start protected run: run %s is %s, want preparing", plan.Run.ID, plan.Run.State)
	}
	if err := c.Overlay.Mount(ctx, plan.Layout); err != nil {
		return nil, fmt.Errorf("start protected run: mount: %w", err)
	}
	handle := &Handle{coordinator: &c, plan: plan}
	if err := plan.Run.Transition(lifecycle.Running); err != nil {
		return handle.failAndRollback(ctx, fmt.Errorf("start protected run: transition running: %w", err))
	}
	process, err := c.Starter.Start(ctx, command, plan.Layout.Merged, plan.Landlock)
	if err != nil {
		return handle.failAndRollback(ctx, fmt.Errorf("start protected run: start agent: %w", err))
	}
	handle.process = process
	if strings.TrimSpace(sensorObject) != "" {
		if c.Sensor == nil {
			return handle.failAndRollback(ctx, fmt.Errorf("start protected run: telemetry sensor is required when an object is configured"))
		}
		if process.PID() == 0 {
			return handle.failAndRollback(ctx, fmt.Errorf("start protected run: process returned pid zero"))
		}
		sensor, err := c.Sensor.Attach(ctx, sensorObject, plan.Run.ID, process.PID())
		if err != nil {
			return handle.failAndRollback(ctx, fmt.Errorf("start protected run: attach telemetry: %w", err))
		}
		handle.sensor = sensor
	}
	return handle, nil
}

func (h *Handle) Wait() error {
	if h == nil || h.process == nil {
		return fmt.Errorf("wait protected run: process is not started")
	}
	err := h.process.Wait()
	h.waited = true
	closeErr := h.closeSensor()
	if err != nil {
		if transitionErr := h.plan.Run.Transition(lifecycle.Failed); transitionErr != nil {
			err = errors.Join(err, transitionErr)
		}
		return errors.Join(err, closeErr)
	}
	if transitionErr := h.plan.Run.Transition(lifecycle.Succeeded); transitionErr != nil {
		return errors.Join(transitionErr, closeErr)
	}
	return closeErr
}

func (h *Handle) Rollback(ctx context.Context) error {
	if h == nil || h.plan == nil || h.coordinator == nil || h.coordinator.Overlay == nil {
		return fmt.Errorf("rollback protected run: handle is incomplete")
	}
	var errs []error
	if h.process != nil && !h.waited {
		if err := h.process.Kill(); err != nil {
			errs = append(errs, fmt.Errorf("kill agent: %w", err))
		}
		h.waited = true
	}
	if err := h.closeSensor(); err != nil {
		errs = append(errs, err)
	}
	if h.plan.Run.State == lifecycle.Running {
		if err := h.plan.Run.Transition(lifecycle.Failed); err != nil {
			errs = append(errs, err)
		}
	}
	if err := h.coordinator.Overlay.Rollback(ctx, h.plan.Layout); err != nil {
		errs = append(errs, fmt.Errorf("rollback overlay: %w", err))
	} else if h.plan.Run.State == lifecycle.Failed || h.plan.Run.State == lifecycle.Succeeded || h.plan.Run.State == lifecycle.Paused {
		if err := h.plan.Run.Transition(lifecycle.RolledBack); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (h *Handle) failAndRollback(ctx context.Context, cause error) (*Handle, error) {
	if h.process != nil && !h.waited {
		_ = h.process.Kill()
		h.waited = true
	}
	if err := h.closeSensor(); err != nil {
		cause = errors.Join(cause, err)
	}
	if h.plan.Run.State == lifecycle.Running {
		if err := h.plan.Run.Transition(lifecycle.Failed); err != nil {
			cause = errors.Join(cause, err)
		}
	}
	if err := h.coordinator.Overlay.Rollback(ctx, h.plan.Layout); err != nil {
		return h, errors.Join(cause, fmt.Errorf("rollback after start failure: %w", err))
	}
	if h.plan.Run.State == lifecycle.Failed {
		if err := h.plan.Run.Transition(lifecycle.RolledBack); err != nil {
			cause = errors.Join(cause, err)
		}
	}
	return h, cause
}

func (h *Handle) closeSensor() error {
	if h.sensor == nil {
		return nil
	}
	err := h.sensor.Close()
	h.sensor = nil
	if err != nil {
		return fmt.Errorf("close telemetry: %w", err)
	}
	return nil
}
