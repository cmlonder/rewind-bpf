package protectedrun

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rewindbpf/rewind/internal/landlock"
)

// ExecStarter launches the hidden rewind helper, which applies Landlock in a
// child process and then execs the agent command. The helper executable is
// intentionally explicit so the final CLI can use the same binary or a
// separately installed helper.
type ExecStarter struct {
	HelperPath string
}

func (s ExecStarter) Start(ctx context.Context, command []string, cwd string, plan *landlock.Plan) (Process, error) {
	if strings.TrimSpace(s.HelperPath) == "" {
		return nil, fmt.Errorf("start agent: helper path cannot be empty")
	}
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return nil, fmt.Errorf("start agent: command cannot be empty")
	}
	if !filepath.IsAbs(cwd) {
		return nil, fmt.Errorf("start agent: cwd must be absolute")
	}
	if info, err := os.Stat(s.HelperPath); err != nil || info.IsDir() {
		if err == nil {
			err = fmt.Errorf("path is a directory")
		}
		return nil, fmt.Errorf("start agent: helper: %w", err)
	}

	args := []string{"helper"}
	var planPath string
	if plan != nil {
		if plan.Root == "" {
			return nil, fmt.Errorf("start agent: Landlock plan root cannot be empty")
		}
		planPath = filepath.Join(filepath.Dir(plan.Root), ".rewind-landlock-plan.json")
		if err := writePlan(planPath, *plan); err != nil {
			return nil, err
		}
		args = append(args, "--plan-file", planPath)
	}
	args = append(args, "--")
	args = append(args, command...)
	process := exec.CommandContext(ctx, s.HelperPath, args...)
	process.Dir = cwd
	gateRead, gateWrite, err := os.Pipe()
	if err != nil {
		if planPath != "" {
			_ = os.Remove(planPath)
		}
		return nil, fmt.Errorf("start agent: create start gate: %w", err)
	}
	process.ExtraFiles = []*os.File{gateRead}
	process.Env = append(os.Environ(), "REWIND_START_GATE_FD=3")
	// Preserve the agent's terminal streams so helper and policy failures are
	// visible to the operator instead of collapsing into "exit status 2".
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	if err := process.Start(); err != nil {
		_ = gateRead.Close()
		_ = gateWrite.Close()
		if planPath != "" {
			_ = os.Remove(planPath)
		}
		return nil, fmt.Errorf("start agent helper: %w", err)
	}
	_ = gateRead.Close()
	return &execProcess{cmd: process, planPath: planPath, gate: gateWrite}, nil
}

type execProcess struct {
	cmd      *exec.Cmd
	planPath string
	gate     *os.File
}

func (p *execProcess) PID() uint32 {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return uint32(p.cmd.Process.Pid)
}

func (p *execProcess) Wait() error {
	if p == nil || p.cmd == nil {
		return fmt.Errorf("wait agent: process is nil")
	}
	err := p.cmd.Wait()
	p.closeGate()
	p.cleanupPlan()
	return err
}

func (p *execProcess) Kill() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return fmt.Errorf("kill agent: process is nil")
	}
	p.closeGate()
	err := p.cmd.Process.Kill()
	p.cleanupPlan()
	return err
}

func (p *execProcess) Release() error {
	if p == nil || p.gate == nil {
		return nil
	}
	_, err := p.gate.Write([]byte{1})
	p.closeGate()
	if err != nil {
		return fmt.Errorf("release agent start gate: %w", err)
	}
	return nil
}

func (p *execProcess) closeGate() {
	if p != nil && p.gate != nil {
		_ = p.gate.Close()
		p.gate = nil
	}
}

func (p *execProcess) cleanupPlan() {
	if p.planPath != "" {
		_ = os.Remove(p.planPath)
		p.planPath = ""
	}
}

func writePlan(path string, plan landlock.Plan) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("start agent: create Landlock plan: %w", err)
	}
	encoderErr := json.NewEncoder(file).Encode(plan)
	closeErr := file.Close()
	if encoderErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("start agent: encode Landlock plan: %w", encoderErr)
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("start agent: close Landlock plan: %w", closeErr)
	}
	return nil
}
