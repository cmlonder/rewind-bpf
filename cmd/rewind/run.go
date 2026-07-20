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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rewindbpf/rewind/internal/capabilities"
	"github.com/rewindbpf/rewind/internal/cgroup"
	"github.com/rewindbpf/rewind/internal/diff"
	"github.com/rewindbpf/rewind/internal/ebpfload"
	"github.com/rewindbpf/rewind/internal/event"
	"github.com/rewindbpf/rewind/internal/evidence"
	"github.com/rewindbpf/rewind/internal/export"
	"github.com/rewindbpf/rewind/internal/history"
	"github.com/rewindbpf/rewind/internal/lifecycle"
	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/netpolicy"
	"github.com/rewindbpf/rewind/internal/overlay"
	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/protectedrun"
	"github.com/rewindbpf/rewind/internal/runplan"
	"github.com/rewindbpf/rewind/internal/runstore"
	"github.com/rewindbpf/rewind/internal/telemetry"
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
	networkBackend := flags.String("network-backend", "", "network backend for enforce mode: proxy")
	onSuccess := flags.String("on-success", "discard", "successful-run outcome: discard (default) or review")
	historyPath := flags.String("history", "", "optional durable run history JSON path")
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
	if err := validateOnSuccess(*onSuccess); err != nil {
		fatal(err.Error())
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
		NetworkBackend: *networkBackend,
	})
	if err != nil {
		fatal(err.Error())
	}
	eventsPath := filepath.Join(plan.Layout.Root, "events.jsonl")
	owner, err := agentOwner()
	if err != nil {
		fatal(err.Error())
	}
	maxEventBytes, err := configuredTelemetryMaxBytes()
	if err != nil {
		fatal(err.Error())
	}
	rotateEventBytes, err := configuredTelemetryRotateBytes()
	if err != nil {
		fatal(err.Error())
	}
	telemetry := &telemetryAdapter{path: eventsPath, owner: owner, maxBytes: maxEventBytes, rotateBytes: rotateEventBytes, denyRawNetwork: plan.Network.RawSocketDeny}
	capabilityReport := capabilities.Probe()
	if err := capabilityReport.ValidateForProtectedRun(string(*overlayBackend), plan.Landlock != nil, plan.Network.RawSocketDeny); err != nil {
		fatal(fmt.Sprintf("protected-run capability check: %v", err))
	}
	plan.Capabilities = capabilityReport
	plan.HistoryPath = *historyPath
	// Prepare the dedicated runtime tree before the first journal write. The
	// agent must be able to traverse the eventual merged mount; creating the
	// parent through runstore alone would leave it mode 0700 when invoked via
	// sudo.
	if err := plan.Layout.Prepare(); err != nil {
		fatal(fmt.Sprintf("prepare runtime layout: %v", err))
	}
	scope, err := cgroup.NewAtWithLimits("/sys/fs/cgroup", plan.Run.ID, cgroup.Limits{
		PIDsMax:   plan.Resources.PIDsMax,
		MemoryMax: plan.Resources.MemoryMax,
		CPUMax:    plan.Resources.CPUMax,
	})
	if err != nil {
		fatal(fmt.Sprintf("create process scope: %v", err))
	}
	if err := scope.Configure(cgroup.Limits{
		PIDsMax:   plan.Resources.PIDsMax,
		MemoryMax: plan.Resources.MemoryMax,
		CPUMax:    plan.Resources.CPUMax,
	}); err != nil {
		_ = scope.Close()
		fatal(fmt.Sprintf("configure process scope: %v", err))
	}
	plan.CgroupPath = scope.Path()
	if err := persistRecord(*recordPath, plan, eventsPath); err != nil {
		fatal(fmt.Sprintf("persist prepared run: %v", err))
	}
	if err := persistHistory(*historyPath, plan, *recordPath); err != nil {
		fatal(fmt.Sprintf("persist run history: %v", err))
	}
	helper, err := os.Executable()
	if err != nil {
		fatal(fmt.Sprintf("resolve rewind helper: %v", err))
	}
	var networkProxy *netpolicy.Proxy
	var stopNetworkProxy context.CancelFunc
	closeNetworkProxy := func() {
		if stopNetworkProxy == nil {
			return
		}
		stopNetworkProxy()
		_ = networkProxy.Close()
		stopNetworkProxy = nil
		networkProxy = nil
	}
	starter := protectedrun.ExecStarter{HelperPath: helper, DenyRawNetwork: plan.Network.Mode == policy.ModeEnforce && *networkBackend == "proxy"}
	// An explicit proxy backend can observe audit mode as well as enforce mode.
	// Audit stays zero-overhead when no backend is selected; enforce remains
	// fail-closed in runplan.Build unless the proxy is explicitly requested.
	if plan.Network.Mode != policy.ModeOff && *networkBackend == "proxy" {
		networkProxy, err = netpolicy.ListenProxy(plan.Network)
		if err != nil {
			fatal(fmt.Sprintf("start network policy proxy: %v", err))
		}
		networkProxy.Audit = func(host string, decision netpolicy.Decision) {
			if err := telemetry.RecordNetwork(host, decision); err != nil {
				telemetry.markError(err)
			}
		}
		proxyCtx, cancel := context.WithCancel(context.Background())
		stopNetworkProxy = cancel
		go func() { _ = networkProxy.Serve(proxyCtx) }()
		proxyURL := networkProxy.URL()
		starter.Env = []string{"HTTP_PROXY=" + proxyURL, "HTTPS_PROXY=" + proxyURL, "ALL_PROXY=" + proxyURL, "NO_PROXY="}
	}
	coordinator := protectedrun.Coordinator{
		Overlay: overlay.Manager{Owner: &owner, Backend: plan.OverlayBackend},
		Starter: starter,
		Sensor:  telemetry,
		Scope:   &scope,
	}
	handle, err := coordinator.Start(context.Background(), &plan, command, *sensorObject)
	if err != nil {
		closeNetworkProxy()
		_ = persistRecordState(*recordPath, plan, eventsPath, telemetry.EvidenceState())
		fatal(err.Error())
	}
	if err := persistRecordState(*recordPath, plan, eventsPath, telemetry.EvidenceState()); err != nil {
		_ = handle.Rollback(context.Background())
		closeNetworkProxy()
		fatal(fmt.Sprintf("persist running run; transaction rolled back: %v", err))
	}
	if err := persistHistory(*historyPath, plan, *recordPath); err != nil {
		_ = handle.Rollback(context.Background())
		closeNetworkProxy()
		fatal(fmt.Sprintf("persist running history: %v", err))
	}
	// Drain proxy handlers before closing telemetry so userspace network
	// decisions are persisted in the same evidence chain as kernel events.
	waitErr := handle.WaitWith(func() error {
		closeNetworkProxy()
		return nil
	})
	if waitErr != nil {
		rollbackErr := handle.Rollback(context.Background())
		_ = persistRecordState(*recordPath, plan, eventsPath, telemetry.EvidenceState())
		_ = persistHistory(*historyPath, plan, *recordPath)
		fatal(errors.Join(waitErr, rollbackErr).Error())
	}
	if *onSuccess == "discard" {
		if err := handle.Rollback(context.Background()); err != nil {
			_ = persistRecordState(*recordPath, plan, eventsPath, telemetry.EvidenceState())
			fatal(fmt.Sprintf("default discard: %v", err))
		}
		if err := persistRecordState(*recordPath, plan, eventsPath, telemetry.EvidenceState()); err != nil {
			fatal(fmt.Sprintf("persist discarded run: %v", err))
		}
		if err := persistHistory(*historyPath, plan, *recordPath); err != nil {
			fatal(fmt.Sprintf("persist discarded history: %v", err))
		}
		fmt.Printf("run completed and discarded: run_id=%s state=%s record=%s\n", plan.Run.ID, plan.Run.State, *recordPath)
		return
	}
	if err := persistRecordState(*recordPath, plan, eventsPath, telemetry.EvidenceState()); err != nil {
		_ = handle.Rollback(context.Background())
		fatal(fmt.Sprintf("persist successful run; transaction rolled back: %v", err))
	}
	if err := persistHistory(*historyPath, plan, *recordPath); err != nil {
		_ = handle.Rollback(context.Background())
		fatal(fmt.Sprintf("persist review history: %v", err))
	}
	fmt.Printf("run ready for review: run_id=%s state=%s record=%s\n", plan.Run.ID, plan.Run.State, *recordPath)
	fmt.Printf("discard with: rewind rollback --record %s\n", *recordPath)
}

func persistHistory(path string, plan runplan.Plan, recordPath string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return history.Open(path).Upsert(history.Entry{RunID: plan.Run.ID, State: string(plan.Run.State), Workspace: plan.Layout.Lower, RecordPath: recordPath, UpdatedAt: plan.Run.UpdatedAt, CreatedAt: plan.Run.CreatedAt})
}

func validateOnSuccess(value string) error {
	if value != "discard" && value != "review" {
		return fmt.Errorf("unsupported --on-success value %q (want discard or review)", value)
	}
	return nil
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
	if err := persistRecordState(*recordPath, record.Plan, record.EventsPath, evidenceState{dropped: record.Events.Dropped, truncated: record.Events.Truncated}); err != nil {
		fatal(err.Error())
	}
	if err := persistHistory(record.Plan.HistoryPath, record.Plan, *recordPath); err != nil {
		fatal(fmt.Sprintf("persist rollback history: %v", err))
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
	if err := persistRecordState(*recordPath, record.Plan, record.EventsPath, evidenceState{dropped: record.Events.Dropped, truncated: record.Events.Truncated}); err != nil {
		fatal(err.Error())
	}
	if err := persistHistory(record.Plan.HistoryPath, record.Plan, *recordPath); err != nil {
		fatal(fmt.Sprintf("persist recovery history: %v", err))
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
	var state evidenceState
	if len(dropped) > 0 {
		state.dropped = dropped[0]
	}
	return persistRecordState(path, plan, eventsPath, state)
}

type evidenceState struct {
	dropped   uint64
	truncated bool
}

func persistRecordState(path string, plan runplan.Plan, eventsPath string, state evidenceState) error {
	eventPaths := discoverEventPaths(eventsPath)
	evidence, err := runstore.SummarizeEventsPaths(eventPaths)
	if err != nil {
		return err
	}
	evidence = evidence.WithDropped(state.dropped).WithTruncated(state.truncated)
	return runstore.Write(path, runstore.Record{Plan: plan, EventsPath: eventsPath, EventsPaths: eventPaths, Events: evidence})
}

func discoverEventPaths(eventsPath string) []string {
	if strings.TrimSpace(eventsPath) == "" {
		return nil
	}
	paths := []string{eventsPath}
	pattern := fmt.Sprintf("%s-*.jsonl", strings.TrimSuffix(eventsPath, filepath.Ext(eventsPath)))
	rotated, _ := filepath.Glob(pattern)
	sort.Strings(rotated)
	return append(paths, rotated...)
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

func handleInspect(args []string) {
	flags := flag.NewFlagSet("rewind inspect", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "run record JSON path")
	if err := flags.Parse(args); err != nil {
		fatal(err.Error())
	}
	if flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind inspect --record PATH")
	}
	record, err := runstore.Read(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	if err := json.NewEncoder(os.Stdout).Encode(record); err != nil {
		fatal(fmt.Sprintf("encode run inspection: %v", err))
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
	for _, path := range runstore.EventLogPaths(record) {
		file, err := os.Open(path)
		if err != nil {
			fatal(fmt.Sprintf("open events: %v", err))
		}
		if _, err := io.Copy(os.Stdout, file); err != nil {
			_ = file.Close()
			fatal(fmt.Sprintf("read events: %v", err))
		}
		if err := file.Close(); err != nil {
			fatal(fmt.Sprintf("close events: %v", err))
		}
	}
}

func handleVerify(args []string) {
	flags := flag.NewFlagSet("rewind verify", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "run record JSON path")
	if err := flags.Parse(args); err != nil {
		fatal(err.Error())
	}
	if flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind verify --record PATH")
	}
	record, err := runstore.Read(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	result, err := evidence.Verify(record)
	if err != nil {
		fatal(err.Error())
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fatal(err.Error())
	}
	if !result.Complete {
		os.Exit(2)
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

func handleExport(args []string) {
	if runtime.GOOS != "linux" {
		fatal("rewind export is Linux-only; use the disposable Ubuntu VM")
	}
	flags := flag.NewFlagSet("rewind export", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "run record JSON path")
	outputPath := flags.String("output", "", "review bundle JSON output path")
	format := flags.String("format", "json", "output format: json, patch (text), or git-patch (full fidelity)")
	if err := flags.Parse(args); err != nil {
		fatal(err.Error())
	}
	if flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" || strings.TrimSpace(*outputPath) == "" {
		fatal("usage: rewind export --record PATH --output PATH")
	}
	record, err := runstore.Read(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	if record.Plan.Run.State == lifecycle.RolledBack || record.Plan.Run.State == lifecycle.Committed {
		fatal(fmt.Sprintf("export run %s: merged view is no longer available", record.Plan.Run.ID))
	}
	outputAbs, err := filepath.Abs(*outputPath)
	if err != nil {
		fatal(fmt.Sprintf("resolve export output: %v", err))
	}
	if pathWithin(record.Plan.Layout.Lower, outputAbs) || pathWithin(record.Plan.Layout.Root, outputAbs) {
		fatal("export output must be outside the workspace and runtime root")
	}
	after, err := manifest.Build(record.Plan.Layout.Merged)
	if err != nil {
		fatal(fmt.Sprintf("build export manifest: %v", err))
	}
	bundle, err := export.Build(record.Plan.Run.ID, record.Plan.Manifest, after)
	if err != nil {
		fatal(err.Error())
	}
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		if err := export.Write(*outputPath, bundle); err != nil {
			fatal(err.Error())
		}
		fmt.Printf("wrote review export with %d changes to %s\n", len(bundle.Changes), *outputPath)
	case "patch":
		patch, err := export.UnifiedPatch(bundle, record.Plan.Layout.Lower, record.Plan.Layout.Merged)
		if err != nil {
			fatal(err.Error())
		}
		if err := export.WritePatch(*outputPath, patch); err != nil {
			fatal(err.Error())
		}
		fmt.Printf("wrote review patch with %d changes to %s\n", len(bundle.Changes), *outputPath)
	case "git-patch":
		patch, err := export.GitPatch(record.Plan.Layout.Lower, record.Plan.Layout.Merged)
		if err != nil {
			fatal(err.Error())
		}
		if err := export.WritePatch(*outputPath, patch); err != nil {
			fatal(err.Error())
		}
		fmt.Printf("wrote full-fidelity review patch with %d changes to %s\n", len(bundle.Changes), *outputPath)
	default:
		fatal(fmt.Sprintf("unsupported export format %q", *format))
	}
}

func pathWithin(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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

func configuredTelemetryMaxBytes() (uint64, error) {
	value := strings.TrimSpace(os.Getenv("REWIND_EVENT_MAX_BYTES"))
	if value == "" {
		return 0, nil
	}
	maxBytes, err := strconv.ParseUint(value, 10, 64)
	if err != nil || maxBytes == 0 {
		return 0, fmt.Errorf("REWIND_EVENT_MAX_BYTES must be a positive integer when set")
	}
	return maxBytes, nil
}

func configuredTelemetryRotateBytes() (uint64, error) {
	value := strings.TrimSpace(os.Getenv("REWIND_EVENT_ROTATE_BYTES"))
	if value == "" {
		return 0, nil
	}
	rotateBytes, err := strconv.ParseUint(value, 10, 64)
	if err != nil || rotateBytes == 0 {
		return 0, fmt.Errorf("REWIND_EVENT_ROTATE_BYTES must be a positive integer when set")
	}
	return rotateBytes, nil
}

type telemetryAdapter struct {
	path           string
	owner          overlay.Owner
	maxBytes       uint64
	rotateBytes    uint64
	denyRawNetwork bool

	mu        sync.Mutex
	writerMu  sync.Mutex
	runID     string
	pid       uint32
	session   *ebpfload.Session
	file      *os.File
	paths     []string
	nextIndex uint64
	done      chan struct{}
	once      sync.Once
	closeErr  error
	dropped   uint64
	dropErr   error
	writer    *telemetry.JournalWriter
	truncated bool
}

func (a *telemetryAdapter) Attach(_ context.Context, objectPath, runID string, pid uint32) (io.Closer, error) {
	session, err := ebpfload.Load(objectPath, runID, pid, a.denyRawNetwork)
	if err != nil {
		return nil, err
	}
	file, err := a.openFile(a.path)
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("open telemetry log: %w", err)
	}
	a.mu.Lock()
	a.session, a.file, a.done = session, file, make(chan struct{})
	a.runID, a.pid = runID, pid
	a.paths, a.nextIndex = []string{a.path}, 1
	a.writer = &telemetry.JournalWriter{Destination: file, MaxBytes: a.maxBytes, RotateBytes: a.rotateBytes, Rotate: a.rotate}
	a.mu.Unlock()
	go a.readLoop()
	return a, nil
}

func (a *telemetryAdapter) openFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open telemetry log: %w", err)
	}
	if os.Geteuid() == 0 {
		if err := os.Chown(path, a.owner.UID, a.owner.GID); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("set telemetry log owner: %w", err)
		}
	}
	return file, nil
}

func (a *telemetryAdapter) rotate() (io.Writer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.file == nil {
		return nil, fmt.Errorf("current telemetry log is closed")
	}
	if err := a.file.Close(); err != nil {
		return nil, err
	}
	path := fmt.Sprintf("%s-%06d.jsonl", strings.TrimSuffix(a.path, filepath.Ext(a.path)), a.nextIndex)
	file, err := a.openFile(path)
	if err != nil {
		return nil, err
	}
	a.file = file
	a.paths = append(a.paths, path)
	a.nextIndex++
	return file, nil
}

func (a *telemetryAdapter) readLoop() {
	a.mu.Lock()
	session, done := a.session, a.done
	a.mu.Unlock()
	defer close(done)
	for {
		value, err := session.Events().Read()
		if err != nil {
			return
		}
		if err := a.append(value); err != nil {
			a.markError(err)
			return
		}
	}
}

func (a *telemetryAdapter) append(value event.Event) error {
	a.writerMu.Lock()
	defer a.writerMu.Unlock()
	a.mu.Lock()
	writer := a.writer
	a.mu.Unlock()
	if writer == nil {
		return fmt.Errorf("append telemetry event: writer is not initialized")
	}
	if err := writer.Append(value); err != nil {
		return err
	}
	if writer.Truncated {
		a.mu.Lock()
		a.truncated = true
		a.mu.Unlock()
	}
	return nil
}

func (a *telemetryAdapter) markError(err error) {
	if err == nil {
		return
	}
	a.mu.Lock()
	a.closeErr = errors.Join(a.closeErr, err)
	a.truncated = true
	a.mu.Unlock()
}

// RecordNetwork adds a policy decision to the same ordered, hash-chained
// evidence stream as kernel events. The proxy is a userspace boundary, so its
// PID is the scoped agent PID captured when the sensor attaches.
func (a *telemetryAdapter) RecordNetwork(host string, decision netpolicy.Decision) error {
	a.mu.Lock()
	runID, pid := a.runID, a.pid
	a.mu.Unlock()
	if runID == "" {
		return fmt.Errorf("record network event: telemetry is not attached")
	}
	if pid == 0 {
		pid = uint32(os.Getpid())
	}
	eventDecision := event.Allow
	if decision == netpolicy.Deny {
		eventDecision = event.Deny
	} else if decision == netpolicy.Audit {
		eventDecision = event.Audit
	}
	return a.append(event.Event{
		RunID:       runID,
		PID:         pid,
		Operation:   event.NetworkConnect,
		Path:        host,
		TimestampNS: uint64(time.Now().UnixNano()),
		Decision:    eventDecision,
		Risk:        event.Medium,
	})
}

func (a *telemetryAdapter) Close() error {
	a.once.Do(func() {
		a.mu.Lock()
		session, done := a.session, a.done
		a.mu.Unlock()
		if session != nil {
			// A process can exit immediately after submitting a ring-buffer
			// record. Give the userspace reader a bounded drain window before
			// closing the map, otherwise short runs can lose every event.
			time.Sleep(100 * time.Millisecond)
			dropped, dropErr := session.Dropped()
			a.mu.Lock()
			a.dropped, a.dropErr = dropped, dropErr
			a.mu.Unlock()
			a.closeErr = errors.Join(a.closeErr, session.Close())
		}
		if done != nil {
			<-done
		}
		a.mu.Lock()
		file := a.file
		a.mu.Unlock()
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

func (a *telemetryAdapter) EvidenceState() evidenceState {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := evidenceState{dropped: a.dropped}
	state.truncated = a.truncated
	return state
}
