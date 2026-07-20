// Package ebpfload owns the lifecycle of the RewindBPF telemetry collection.
// It does not start agents, mount filesystems, or evaluate user policies.
package ebpfload

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/rewindbpf/rewind/internal/telemetry"
)

type tracepointBinding struct {
	program string
	group   string
	name    string
}

var tracepointBindings = []tracepointBinding{
	{program: "trace_execve", group: "syscalls", name: "sys_enter_execve"},
	{program: "trace_openat", group: "syscalls", name: "sys_enter_openat"},
	{program: "trace_write", group: "syscalls", name: "sys_enter_write"},
	{program: "trace_pwrite64", group: "syscalls", name: "sys_enter_pwrite64"},
	{program: "trace_unlinkat", group: "syscalls", name: "sys_enter_unlinkat"},
	{program: "trace_renameat2", group: "syscalls", name: "sys_enter_renameat2"},
	{program: "trace_truncate", group: "syscalls", name: "sys_enter_truncate"},
	{program: "trace_socket", group: "syscalls", name: "sys_enter_socket"},
	{program: "trace_process_exit", group: "sched", name: "sched_process_exit"},
}

// Session owns loaded programs, tracepoint links, and the userspace event
// reader for one protected run.
type Session struct {
	collection *ebpf.Collection
	links      []link.Link
	events     *telemetry.Reader
	dropped    *ebpf.Map
}

// Load parses and loads a telemetry object, scopes it to targetPID, and
// attaches the declared tracepoints. This is a privileged Linux operation and
// must only be called inside the disposable VM.
func Load(objectPath, runID string, targetPID uint32) (*Session, error) {
	if err := validateObjectPath(objectPath); err != nil {
		return nil, err
	}
	if strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("load eBPF sensors: run id cannot be empty")
	}
	if targetPID == 0 {
		return nil, fmt.Errorf("load eBPF sensors: target pid must be non-zero")
	}

	spec, err := ebpf.LoadCollectionSpec(objectPath)
	if err != nil {
		return nil, fmt.Errorf("load eBPF sensors: parse object: %w", err)
	}
	if err := spec.RewriteConstants(map[string]interface{}{"target_pid": targetPID}); err != nil {
		return nil, fmt.Errorf("load eBPF sensors: set target pid: %w", err)
	}
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("load eBPF sensors: remove memlock limit: %w", err)
	}
	collection, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, fmt.Errorf("load eBPF sensors: create collection: %w", err)
	}

	session := &Session{collection: collection}
	cleanup := func(loadErr error) (*Session, error) {
		_ = session.Close()
		return nil, loadErr
	}
	for _, binding := range tracepointBindings {
		program := collection.Programs[binding.program]
		if program == nil {
			return cleanup(fmt.Errorf("load eBPF sensors: object missing program %s", binding.program))
		}
		attached, err := link.Tracepoint(binding.group, binding.name, program, nil)
		if err != nil {
			return cleanup(fmt.Errorf("load eBPF sensors: attach %s: %w", binding.name, err))
		}
		session.links = append(session.links, attached)
	}

	eventsMap := collection.Maps["rewind_events"]
	if eventsMap == nil {
		return cleanup(fmt.Errorf("load eBPF sensors: object missing rewind_events map"))
	}
	events, err := telemetry.NewReader(eventsMap, runID)
	if err != nil {
		return cleanup(fmt.Errorf("load eBPF sensors: create event reader: %w", err))
	}
	session.events = events
	session.dropped = collection.Maps["rewind_dropped"]
	if session.dropped == nil {
		return cleanup(fmt.Errorf("load eBPF sensors: object missing rewind_dropped map"))
	}
	return session, nil
}

func (s *Session) Events() *telemetry.Reader {
	if s == nil {
		return nil
	}
	return s.events
}

// Dropped returns the aggregate per-CPU ring-buffer reserve failures.
func (s *Session) Dropped() (uint64, error) {
	if s == nil || s.dropped == nil {
		return 0, fmt.Errorf("read dropped events: map is not initialized")
	}
	key := uint32(0)
	var values []uint64
	if err := s.dropped.Lookup(&key, &values); err != nil {
		return 0, fmt.Errorf("read dropped events: lookup: %w", err)
	}
	var total uint64
	for _, value := range values {
		total += value
	}
	return total, nil
}

// Close detaches tracepoints before closing the collection. It is safe to
// call more than once and intentionally does not unload anything outside this
// collection.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	var closeErrs []error
	if s.events != nil {
		if err := s.events.Close(); err != nil {
			closeErrs = append(closeErrs, err)
		}
		s.events = nil
	}
	for _, attached := range s.links {
		if attached != nil {
			if err := attached.Close(); err != nil {
				closeErrs = append(closeErrs, err)
			}
		}
	}
	s.links = nil
	if s.collection != nil {
		s.collection.Close()
		s.collection = nil
	}
	return errors.Join(closeErrs...)
}

func validateObjectPath(objectPath string) error {
	if strings.TrimSpace(objectPath) == "" {
		return fmt.Errorf("load eBPF sensors: object path cannot be empty")
	}
	info, err := os.Stat(objectPath)
	if err != nil {
		return fmt.Errorf("load eBPF sensors: stat object %s: %w", objectPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("load eBPF sensors: object path is a directory: %s", objectPath)
	}
	return nil
}
