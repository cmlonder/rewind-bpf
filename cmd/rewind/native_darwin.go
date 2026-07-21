//go:build darwin

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rewindbpf/rewind/internal/diff"
	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/pii"
	"github.com/rewindbpf/rewind/internal/platform"
	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/runid"
)

func handleNative(args []string) {
	if len(args) == 0 {
		fatal("usage: rewind native run|status|events|diff|rollback|commit")
	}
	switch args[0] {
	case "run":
		handleNativeRun(args[1:])
	case "status":
		handleNativeStatus(args[1:])
	case "events":
		handleNativeEvents(args[1:])
	case "diff":
		handleNativeDiff(args[1:])
	case "rollback":
		handleNativeRollback(args[1:])
	case "commit":
		handleNativeCommit(args[1:])
	default:
		fatal("usage: rewind native run|status|events|diff|rollback|commit")
	}
}

func handleNativeRun(args []string) {
	flags := flag.NewFlagSet("rewind native run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workspace := flags.String("workspace", "", "workspace directory to protect")
	runtimeRoot := flags.String("runtime-root", "", "dedicated disposable runtime directory")
	policyPath := flags.String("policy", "", "YAML policy path")
	recordPath := flags.String("record", "", "native run record JSON path")
	historyPath := flags.String("history", "", "optional durable local supervisor history JSON path")
	onSuccess := flags.String("on-success", "discard", "successful-run outcome: discard or review")
	runtimeRoots := flags.String("runtime-roots", "/usr,/bin,/sbin,/System/Library,/Library/Frameworks,/private/etc", "comma-separated read-only runtime roots")
	if err := flags.Parse(args); err != nil {
		fatal(err.Error())
	}
	command := flags.Args()
	if len(command) == 0 || strings.TrimSpace(*workspace) == "" || strings.TrimSpace(*runtimeRoot) == "" || strings.TrimSpace(*policyPath) == "" || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind native run --workspace PATH --runtime-root PATH --policy PATH --record PATH [--history PATH --on-success discard|review] -- <agent-command>")
	}
	if *onSuccess != "discard" && *onSuccess != "review" {
		fatal("--on-success must be discard or review")
	}
	recordAbs, err := filepath.Abs(*recordPath)
	if err != nil {
		fatal(fmt.Sprintf("resolve native record path: %v", err))
	}
	runtimeAbs, err := filepath.Abs(*runtimeRoot)
	if err != nil {
		fatal(fmt.Sprintf("resolve native runtime root: %v", err))
	}
	if withinNative(filepath.Clean(runtimeAbs), filepath.Clean(recordAbs)) {
		fatal("native --record must be outside --runtime-root because rollback/discard removes the runtime root")
	}
	recordFile := filepath.Clean(recordAbs)
	historyFile := strings.TrimSpace(*historyPath)
	if historyFile != "" {
		historyFile, err = filepath.Abs(historyFile)
		if err != nil {
			fatal(fmt.Sprintf("resolve native history path: %v", err))
		}
		historyFile = filepath.Clean(historyFile)
	}
	workspacePath, err := filepath.EvalSymlinks(*workspace)
	if err != nil {
		fatal(fmt.Sprintf("resolve macOS workspace: %v", err))
	}
	runtimePath := filepath.Clean(runtimeAbs)
	readRoots := splitCSV(*runtimeRoots)
	if err := validateNativeReadRoots(workspacePath, readRoots); err != nil {
		fatal(err.Error())
	}
	value, err := policy.Load(*policyPath)
	if err != nil {
		fatal(err.Error())
	}
	if value.Network.Mode == policy.ModeEnforce {
		fatal("macOS native run refuses network.mode=enforce until the signed EndpointSecurity/network helper is installed")
	}
	if value.Write.Scope == "system" {
		fatal("macOS native run refuses write.scope=system; only staged workspace writes are supported")
	}
	if value.Resources.PIDsMax != "" || value.Resources.MemoryMax != "" || value.Resources.CPUMax != "" {
		fatal("macOS native run refuses resources.* limits until a native process/resource helper is installed")
	}
	backend := platform.NewMacOSBackend()
	tx, err := backend.PrepareAt(context.Background(), workspacePath, runtimePath)
	if err != nil {
		fatal(fmt.Sprintf("prepare macOS native transaction: %v", err))
	}
	runID, err := runid.New()
	if err != nil {
		_ = tx.Discard(context.Background())
		fatal(err.Error())
	}
	base, err := manifest.Build(workspacePath)
	if err != nil {
		_ = tx.Discard(context.Background())
		fatal(err.Error())
	}
	// Keep lifecycle evidence beside the durable record. The disposable runtime
	// is deleted on rollback/discard, so placing events inside it would erase
	// the only audit trail for failed runs.
	eventsPath := recordFile + ".events.jsonl"
	record := platform.NativeRecord{RunID: runID, Platform: "darwin", Backend: "apfs-clone-seatbelt", Workspace: workspacePath, RuntimeRoot: runtimePath, View: tx.View(), PolicyPath: *policyPath, HistoryPath: historyFile, Command: command, State: "prepared", BaseManifest: base, EventsPath: eventsPath, CreatedAt: time.Now().UTC()}
	if err := writeNativeRecordWithHistory(recordFile, record); err != nil {
		_ = tx.Discard(context.Background())
		fatal(err.Error())
	}
	_ = appendNativeEvent(eventsPath, platform.NativeEvent{Operation: "prepare", Decision: "allow", Timestamp: time.Now().UTC().Format(time.RFC3339Nano)})
	denyPaths, err := nativeDenyPaths(value, tx.View())
	if err != nil {
		_ = tx.Discard(context.Background())
		fatal(fmt.Sprintf("compile macOS read policy: %v", err))
	}
	for _, path := range denyPaths {
		_ = appendNativeEvent(eventsPath, platform.NativeEvent{Operation: "read_policy", Path: path, Decision: "deny", Timestamp: time.Now().UTC().Format(time.RFC3339Nano)})
	}
	hidden, err := hideNativePaths(tx.View(), runtimePath, denyPaths)
	if err != nil {
		_ = tx.Discard(context.Background())
		fatal(fmt.Sprintf("stage macOS sensitive paths: %v", err))
	}
	commandEnv := append(os.Environ(), "REWIND_NATIVE_RUN_ID="+runID)
	child, cleanup, err := platform.SeatbeltCommandWithOptions(platform.SeatbeltCommandOptions{Workspace: tx.View(), Command: command[0], Args: command[1:], WorkingDir: tx.View(), Environment: commandEnv, RuntimeRoots: readRoots})
	if err != nil {
		_ = tx.Discard(context.Background())
		fatal(fmt.Sprintf("prepare macOS Seatbelt command: %v", err))
	}
	defer cleanup()
	record.State = "running"
	_ = writeNativeRecordWithHistory(recordFile, record)
	_ = appendNativeEvent(eventsPath, platform.NativeEvent{Operation: "execve", Path: command[0], Decision: "allow", Timestamp: time.Now().UTC().Format(time.RFC3339Nano)})
	runErr := child.Run()
	if restoreErr := restoreNativePaths(hidden); restoreErr != nil {
		_ = tx.Discard(context.Background())
		fatal(fmt.Sprintf("restore macOS sensitive paths: %v", restoreErr))
	}
	if runErr != nil {
		record.State = "failed"
		if exitErr := new(exec.ExitError); errors.As(runErr, &exitErr) {
			record.ExitCode = exitErr.ExitCode()
		} else {
			record.ExitCode = 1
		}
		_ = appendNativeEvent(eventsPath, platform.NativeEvent{Operation: "exit", Decision: "rollback", ExitCode: record.ExitCode, Timestamp: time.Now().UTC().Format(time.RFC3339Nano)})
		_ = tx.Discard(context.Background())
		_ = writeNativeRecordWithHistory(recordFile, record)
		fatal(fmt.Sprintf("macOS native agent failed: %v", runErr))
	}
	after, err := manifest.Build(tx.View())
	if err != nil {
		_ = tx.Discard(context.Background())
		fatal(fmt.Sprintf("build macOS candidate manifest: %v", err))
	}
	record.Changes, _ = nativeDiff(record.BaseManifest, after)
	if *onSuccess == "discard" {
		if err := tx.Discard(context.Background()); err != nil {
			fatal(err.Error())
		}
		record.State = "discarded"
		_ = appendNativeEvent(eventsPath, platform.NativeEvent{Operation: "exit", Decision: "discard", Timestamp: time.Now().UTC().Format(time.RFC3339Nano)})
	} else {
		record.State = "succeeded"
		_ = appendNativeEvent(eventsPath, platform.NativeEvent{Operation: "exit", Decision: "review", Timestamp: time.Now().UTC().Format(time.RFC3339Nano)})
	}
	if err := writeNativeRecordWithHistory(recordFile, record); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("macOS native run %s: state=%s record=%s\n", record.RunID, record.State, recordFile)
	if *onSuccess == "review" {
		fmt.Printf("review with: rewind native diff --record %s\n", recordFile)
		fmt.Printf("discard with: rewind native rollback --record %s\n", recordFile)
		fmt.Printf("accept with: rewind native commit --record %s --confirm\n", recordFile)
	}
}

type hiddenNativePath struct {
	Original string
	Hidden   string
}

func hideNativePaths(view, runtimeRoot string, paths []string) ([]hiddenNativePath, error) {
	view, err := filepath.Abs(view)
	if err != nil {
		return nil, err
	}
	hiddenRoot := filepath.Join(runtimeRoot, "hidden")
	if err := os.MkdirAll(hiddenRoot, 0o700); err != nil {
		return nil, err
	}
	paths = append([]string(nil), paths...)
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) < len(paths[j]) })
	result := make([]hiddenNativePath, 0, len(paths))
	for _, path := range paths {
		path, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(view, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == "." {
			return nil, fmt.Errorf("sensitive path escapes macOS view: %s", path)
		}
		if _, err := os.Lstat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		nested := false
		for _, existing := range result {
			if existing.Original == path || withinNative(existing.Original, path) {
				nested = true
				break
			}
		}
		if nested {
			continue
		}
		target := filepath.Join(hiddenRoot, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return nil, err
		}
		if err := os.Rename(path, target); err != nil {
			return nil, fmt.Errorf("hide %s: %w", rel, err)
		}
		result = append(result, hiddenNativePath{Original: path, Hidden: target})
	}
	return result, nil
}

func restoreNativePaths(paths []hiddenNativePath) error {
	for i := len(paths) - 1; i >= 0; i-- {
		item := paths[i]
		if _, err := os.Lstat(item.Hidden); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := os.RemoveAll(item.Original); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(item.Original), 0o700); err != nil {
			return err
		}
		if err := os.Rename(item.Hidden, item.Original); err != nil {
			return err
		}
	}
	return nil
}

func withinNative(root, candidate string) bool {
	if root == candidate {
		return true
	}
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func nativeDenyPaths(value policy.Policy, view string) ([]string, error) {
	if value.Read.Mode != policy.ModeEnforce {
		return nil, nil
	}
	snapshot, err := manifest.Build(view)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0)
	for _, entry := range snapshot.Entries {
		candidate := entry.Path
		if value.Read.Explain(candidate).Decision == "deny" || value.Read.Explain("/"+candidate).Decision == "deny" {
			paths = append(paths, filepath.Join(view, filepath.FromSlash(entry.Path)))
		}
	}
	if value.Read.PII.Mode == policy.ModeEnforce {
		findings, err := pii.ScanPath(view)
		if err != nil {
			return nil, err
		}
		for _, finding := range findings {
			paths = append(paths, finding.Path)
		}
	}
	return uniqueStrings(paths), nil
}

func nativeDiff(base, after manifest.Manifest) ([]diff.Change, error) {
	return diff.Compare(base, after), nil
}

func handleNativeStatus(args []string) {
	flags := flag.NewFlagSet("rewind native status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "native run record JSON path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind native status --record PATH")
	}
	record, err := platform.ReadNativeRecord(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	_ = json.NewEncoder(os.Stdout).Encode(record)
}

func handleNativeEvents(args []string) {
	flags := flag.NewFlagSet("rewind native events", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "native run record JSON path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind native events --record PATH")
	}
	record, err := platform.ReadNativeRecord(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	data, err := os.ReadFile(record.EventsPath)
	if err != nil {
		fatal(fmt.Sprintf("read native events: %v", err))
	}
	_, _ = os.Stdout.Write(data)
}

func handleNativeDiff(args []string) {
	flags := flag.NewFlagSet("rewind native diff", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "native run record JSON path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind native diff --record PATH")
	}
	record, err := platform.ReadNativeRecord(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	_ = json.NewEncoder(os.Stdout).Encode(record.Changes)
}

func handleNativeRollback(args []string) {
	flags := flag.NewFlagSet("rewind native rollback", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "native run record JSON path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind native rollback --record PATH")
	}
	record, err := platform.ReadNativeRecord(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	if record.State == "committed" {
		fatal("native rollback cannot undo a committed destination; restore from version control or a separate snapshot")
	}
	if err := platform.DiscardMacOSRuntime(record.RuntimeRoot, record.Workspace); err != nil {
		fatal(err.Error())
	}
	if err := appendNativeEvent(record.EventsPath, platform.NativeEvent{Operation: "rollback", Decision: "rollback", Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		fatal(fmt.Sprintf("write native rollback event: %v", err))
	}
	record.State = "rolled_back"
	if err := writeNativeRecordWithHistory(*recordPath, record); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("macOS native run rolled back: run_id=%s\n", record.RunID)
}

func handleNativeCommit(args []string) {
	flags := flag.NewFlagSet("rewind native commit", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "native run record JSON path")
	confirm := flags.Bool("confirm", false, "explicitly apply the staged candidate")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" {
		fatal("usage: rewind native commit --record PATH --confirm")
	}
	if !*confirm {
		fatal("native commit is destructive to the destination; pass --confirm after reviewing diff")
	}
	record, err := platform.ReadNativeRecord(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	if record.State != "succeeded" {
		fatal(fmt.Sprintf("native commit requires a review run, got %s", record.State))
	}
	if err := appendNativeEvent(record.EventsPath, platform.NativeEvent{Operation: "commit_attempt", Decision: "pending", Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		fatal(fmt.Sprintf("write native commit event: %v", err))
	}
	changes, err := platform.AcceptMacOSRuntime(context.Background(), record.Workspace, record.RuntimeRoot, record.BaseManifest)
	if err != nil {
		fatal(err.Error())
	}
	if err := appendNativeEvent(record.EventsPath, platform.NativeEvent{Operation: "commit", Decision: "allow", Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		fatal(fmt.Sprintf("write native commit event: %v", err))
	}
	record.Changes = changes
	record.State = "committed"
	if err := writeNativeRecordWithHistory(*recordPath, record); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("macOS native run committed: run_id=%s changes=%d\n", record.RunID, len(changes))
}

func appendNativeEvent(path string, event platform.NativeEvent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(event)
}

func writeNativeRecordWithHistory(path string, record platform.NativeRecord) error {
	if err := platform.WriteNativeRecord(path, record); err != nil {
		return err
	}
	if record.HistoryPath == "" {
		return nil
	}
	return platform.PersistNativeHistoryAt(path, record)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func validateNativeReadRoots(workspace string, roots []string) error {
	workspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return fmt.Errorf("resolve macOS workspace for read-root validation: %w", err)
	}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		resolved, resolveErr := filepath.EvalSymlinks(root)
		if resolveErr != nil {
			continue
		}
		if withinNative(resolved, workspace) || withinNative(workspace, resolved) {
			return fmt.Errorf("macOS runtime read root overlaps workspace: %s", root)
		}
	}
	return nil
}
