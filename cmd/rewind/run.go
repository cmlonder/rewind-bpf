package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rewindbpf/rewind/internal/capabilities"
	"github.com/rewindbpf/rewind/internal/cgroup"
	"github.com/rewindbpf/rewind/internal/diff"
	"github.com/rewindbpf/rewind/internal/ebpfload"
	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/overlay"
	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/protectedrun"
	"github.com/rewindbpf/rewind/internal/runplan"
	"github.com/rewindbpf/rewind/internal/runstore"
)

func handleRun(args []string) {
	if runtime.GOOS != "linux" {
		fatal("rewind run is Linux-only; use the disposable Ubuntu VM")
	}
	flags := flag.NewFlagSet("rewind run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workspace := flags.String("workspace", "", "workspace directory to protect")
	runtimeRoot := flags.String("runtime-root", "", "dedicated runtime directory outside the workspace")
	policyPath := flags.String("policy", "", "YAML policy path")
	recordPath := flags.String("record", "", "run record JSON path")
	sensorObject := flags.String("sensor-object", "", "optional compiled telemetry object")
	runtimeRoots := flags.String("runtime-roots", "", "comma-separated system roots needed by the agent")
	overlayBackend := flags.String("overlay-backend", string(overlay.BackendFuse), "overlay backend: fuse or kernel")
	if err := flags.Parse(args); err != nil {
		fatal(err.Error())
	}
	command := flags.Args()
	if len(command) == 0 || strings.TrimSpace(*workspace) == "" || strings.TrimSpace(*runtimeRoot) == "" || strings.TrimSpace(*policyPath) == "" || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind run --workspace PATH --runtime-root PATH --policy PATH --record PATH [--sensor-object PATH] [--runtime-roots PATHS] [--overlay-backend fuse|kernel] -- <agent-command>")
	}
	if *overlayBackend != string(overlay.BackendFuse) && *overlayBackend != string(overlay.BackendKernel) {
		fatal(fmt.Sprintf("unsupported overlay backend %q (want fuse or kernel)", *overlayBackend))
	}
	value, err := policy.Load(*policyPath)
	if err != nil {
		fatal(err.Error())
	}
	plan, err := runplan.Build(runplan.Config{
		Workspace:      *workspace,
		RuntimeRoot:    *runtimeRoot,
		Policy:         value,
		RuntimeRoots:   splitCSV(*runtimeRoots),
		OverlayBackend: overlay.Backend(*overlayBackend),
	})
	if err != nil {
		fatal(err.Error())
	}
	eventsPath := filepath.Join(plan.Layout.Root, "events.jsonl")
	owner, err := agentOwner()
	if err != nil {
		fatal(err.Error())
	}
	telemetry := &telemetryAdapter{path: eventsPath, owner: owner}
	capabilityReport := capabilities.Probe()
	if err := capabilityReport.ValidateForProtectedRun(string(*overlayBackend), plan.Landlock != nil); err != nil {
		fatal(fmt.Sprintf("protected-run capability check: %v", err))
	}
	plan.Capabilities = capabilityReport
	// Prepare the dedicated runtime tree before the first journal write. The
	// agent must be able to traverse the eventual merged mount; creating the
	// parent through runstore alone would leave it mode 0700 when invoked via
	// sudo.
	if err := plan.Layout.Prepare(); err != nil {
		fatal(fmt.Sprintf("prepare runtime layout: %v", err))
	}
	scope, err := cgroup.New(plan.Run.ID)
	if err != nil {
		fatal(fmt.Sprintf("create process scope: %v", err))
	}
	plan.CgroupPath = scope.Path()
	if err := persistRecord(*recordPath, plan, eventsPath); err != nil {
		fatal(fmt.Sprintf("persist prepared run: %v", err))
	}
	helper, err := os.Executable()
	if err != nil {
		fatal(fmt.Sprintf("resolve rewind helper: %v", err))
	}
	coordinator := protectedrun.Coordinator{
		Overlay: overlay.Manager{Owner: &owner, Backend: plan.OverlayBackend},
		Starter: protectedrun.ExecStarter{HelperPath: helper},
		Sensor:  telemetry,
		Scope:   &scope,
	}
	handle, err := coordinator.Start(context.Background(), &plan, command, *sensorObject)
	if err != nil {
		_ = persistRecord(*recordPath, plan, eventsPath, telemetry.Dropped())
		fatal(err.Error())
	}
	if err := persistRecord(*recordPath, plan, eventsPath, telemetry.Dropped()); err != nil {
		_ = handle.Rollback(context.Background())
		fatal(fmt.Sprintf("persist running run; transaction rolled back: %v", err))
	}
	waitErr := handle.Wait()
	if waitErr != nil {
		rollbackErr := handle.Rollback(context.Background())
		_ = persistRecord(*recordPath, plan, eventsPath, telemetry.Dropped())
		fatal(errors.Join(waitErr, rollbackErr).Error())
	}
	if err := persistRecord(*recordPath, plan, eventsPath, telemetry.Dropped()); err != nil {
		_ = handle.Rollback(context.Background())
		fatal(fmt.Sprintf("persist successful run; transaction rolled back: %v", err))
	}
	fmt.Printf("run succeeded: run_id=%s state=%s record=%s\n", plan.Run.ID, plan.Run.State, *recordPath)
	fmt.Printf("rollback with: rewind rollback --record %s\n", *recordPath)
}

func agentOwner() (overlay.Owner, error) {
	if os.Geteuid() != 0 {
		return overlay.Owner{UID: os.Getuid(), GID: os.Getgid()}, nil
	}
	uid, err := strconv.Atoi(os.Getenv("SUDO_UID"))
	if err != nil || uid < 1 {
		return overlay.Owner{}, fmt.Errorf("run requires SUDO_UID for the unprivileged agent")
	}
	gid, err := strconv.Atoi(os.Getenv("SUDO_GID"))
	if err != nil || gid < 1 {
		return overlay.Owner{}, fmt.Errorf("run requires SUDO_GID for the unprivileged agent")
	}
	return overlay.Owner{UID: uid, GID: gid}, nil
}

func handleRollback(args []string) {
	if runtime.GOOS != "linux" {
		fatal("rewind rollback is Linux-only; use the disposable Ubuntu VM")
	}
	flags := flag.NewFlagSet("rewind rollback", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "run record JSON path")
	if err := flags.Parse(args); err != nil {
		fatal(err.Error())
	}
	if flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind rollback --record PATH")
	}
	record, err := runstore.Read(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	coordinator := protectedrun.Coordinator{Overlay: overlay.Manager{Backend: record.Plan.OverlayBackend}, Scope: persistedScope(record.Plan.CgroupPath)}
	if err := coordinator.RollbackPlan(context.Background(), &record.Plan); err != nil {
		fatal(err.Error())
	}
	if err := persistRecord(*recordPath, record.Plan, record.EventsPath, 0); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("run rolled back: run_id=%s record=%s\n", record.Plan.Run.ID, *recordPath)
}

func handleRecover(args []string) {
	if runtime.GOOS != "linux" {
		fatal("rewind recover is Linux-only; use the disposable Ubuntu VM")
	}
	flags := flag.NewFlagSet("rewind recover", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "run record JSON path")
	if err := flags.Parse(args); err != nil {
		fatal(err.Error())
	}
	if flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind recover --record PATH")
	}
	record, err := runstore.Read(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	if err := (protectedrun.Coordinator{Overlay: overlay.Manager{Backend: record.Plan.OverlayBackend}, Scope: persistedScope(record.Plan.CgroupPath)}).RollbackPlan(context.Background(), &record.Plan); err != nil {
		fatal(fmt.Sprintf("recover protected run: %v", err))
	}
	if err := persistRecord(*recordPath, record.Plan, record.EventsPath, 0); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("run recovered and rolled back: run_id=%s record=%s\n", record.Plan.Run.ID, *recordPath)
}

func persistedScope(path string) protectedrun.ProcessScope {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	scope, err := cgroup.FromPath(path)
	if err != nil {
		fatal(err.Error())
	}
	if scope.Path() == "" {
		return nil
	}
	return &scope
}

func persistRecord(path string, plan runplan.Plan, eventsPath string, dropped ...uint64) error {
	evidence, err := runstore.SummarizeEvents(eventsPath)
	if err != nil {
		return err
	}
	var droppedCount uint64
	if len(dropped) > 0 {
		droppedCount = dropped[0]
	}
	evidence = evidence.WithDropped(droppedCount)
	return runstore.Write(path, runstore.Record{Plan: plan, EventsPath: eventsPath, Events: evidence})
}

func handleStatus(args []string) {
	flags := flag.NewFlagSet("rewind status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "run record JSON path")
	if err := flags.Parse(args); err != nil {
		fatal(err.Error())
	}
	if flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind status --record PATH")
	}
	record, err := runstore.Read(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	if err := json.NewEncoder(os.Stdout).Encode(record.Plan.Run); err != nil {
		fatal(err.Error())
	}
}

func handleEvents(args []string) {
	flags := flag.NewFlagSet("rewind events", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "run record JSON path")
	if err := flags.Parse(args); err != nil {
		fatal(err.Error())
	}
	if flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind events --record PATH")
	}
	record, err := runstore.Read(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	file, err := os.Open(record.EventsPath)
	if err != nil {
		fatal(fmt.Sprintf("open events: %v", err))
	}
	defer file.Close()
	if _, err := io.Copy(os.Stdout, file); err != nil {
		fatal(fmt.Sprintf("read events: %v", err))
	}
}

func handleDiff(args []string) {
	if runtime.GOOS != "linux" {
		fatal("rewind diff is Linux-only; use the disposable Ubuntu VM")
	}
	flags := flag.NewFlagSet("rewind diff", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "run record JSON path")
	if err := flags.Parse(args); err != nil {
		fatal(err.Error())
	}
	if flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind diff --record PATH")
	}
	record, err := runstore.Read(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	after, err := manifest.Build(record.Plan.Layout.Merged)
	if err != nil {
		fatal(fmt.Sprintf("build merged diff manifest: %v (the run may already be rolled back)", err))
	}
	if err := json.NewEncoder(os.Stdout).Encode(diff.Compare(record.Plan.Manifest, after)); err != nil {
		fatal(fmt.Sprintf("encode diff: %v", err))
	}
}

func handleCapabilities(args []string) {
	if len(args) != 0 {
		fatal("usage: rewind capabilities")
	}
	report := capabilities.Probe()
	data, err := report.JSON()
	if err != nil {
		fatal(fmt.Sprintf("encode capabilities: %v", err))
	}
	fmt.Println(string(data))
}

func splitCSV(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

type telemetryAdapter struct {
	path  string
	owner overlay.Owner

	mu       sync.Mutex
	session  *ebpfload.Session
	file     *os.File
	done     chan struct{}
	once     sync.Once
	closeErr error
	dropped  uint64
	dropErr  error
}

func (a *telemetryAdapter) Attach(_ context.Context, objectPath, runID string, pid uint32) (io.Closer, error) {
	session, err := ebpfload.Load(objectPath, runID, pid)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("open telemetry log: %w", err)
	}
	if os.Geteuid() == 0 {
		if err := os.Chown(a.path, a.owner.UID, a.owner.GID); err != nil {
			_ = file.Close()
			_ = session.Close()
			return nil, fmt.Errorf("set telemetry log owner: %w", err)
		}
	}
	a.mu.Lock()
	a.session, a.file, a.done = session, file, make(chan struct{})
	a.mu.Unlock()
	go a.readLoop()
	return a, nil
}

func (a *telemetryAdapter) readLoop() {
	a.mu.Lock()
	session, file, done := a.session, a.file, a.done
	a.mu.Unlock()
	defer close(done)
	encoder := json.NewEncoder(file)
	for {
		value, err := session.Events().Read()
		if err != nil {
			return
		}
		if err := encoder.Encode(value); err != nil {
			a.mu.Lock()
			a.closeErr = err
			a.mu.Unlock()
			return
		}
	}
}

func (a *telemetryAdapter) Close() error {
	a.once.Do(func() {
		a.mu.Lock()
		session, file, done := a.session, a.file, a.done
		a.mu.Unlock()
		if session != nil {
			// A process can exit immediately after submitting a ring-buffer
			// record. Give the userspace reader a bounded drain window before
			// closing the map, otherwise short runs can lose every event.
			time.Sleep(100 * time.Millisecond)
			a.dropped, a.dropErr = session.Dropped()
			a.closeErr = session.Close()
		}
		if done != nil {
			<-done
		}
		if file != nil {
			a.closeErr = errors.Join(a.closeErr, a.dropErr, file.Close())
		}
	})
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closeErr
}

func (a *telemetryAdapter) Dropped() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dropped
}
