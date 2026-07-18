package ebpfload

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/rewindbpf/rewind/internal/policycompile"
	"github.com/rewindbpf/rewind/internal/telemetry"
)

// ReadSession owns one BPF-LSM read-enforcement program and its event reader.
// It is deliberately separate from the tracepoint Session so telemetry can be
// tested and deployed without silently enabling enforcement.
type ReadSession struct {
	collection *ebpf.Collection
	link       link.Link
	events     *telemetry.Reader
}

// LoadReadEnforcer loads and attaches the BPF-LSM file_open hook for one
// compiled policy. This is a privileged Linux operation and must only be
// called inside the disposable VM after checking that BPF LSM is active.
func LoadReadEnforcer(objectPath, runID string, targetPID uint32, rules policycompile.ReadRules) (*ReadSession, error) {
	if err := validateObjectPath(objectPath); err != nil {
		return nil, err
	}
	if strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("load read enforcer: run id cannot be empty")
	}
	if targetPID == 0 {
		return nil, fmt.Errorf("load read enforcer: target pid must be non-zero")
	}
	if rules.Mode == "off" {
		return nil, fmt.Errorf("load read enforcer: policy mode is off")
	}

	spec, err := ebpf.LoadCollectionSpec(objectPath)
	if err != nil {
		return nil, fmt.Errorf("load read enforcer: parse object: %w", err)
	}
	if err := spec.RewriteConstants(map[string]interface{}{"target_pid": targetPID}); err != nil {
		return nil, fmt.Errorf("load read enforcer: set target pid: %w", err)
	}
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("load read enforcer: remove memlock limit: %w", err)
	}
	collection, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, fmt.Errorf("load read enforcer: create collection: %w", err)
	}

	session := &ReadSession{collection: collection}
	cleanup := func(loadErr error) (*ReadSession, error) {
		_ = session.Close()
		return nil, loadErr
	}

	rulesMap := collection.Maps[readRuleMapName]
	if rulesMap == nil {
		return cleanup(fmt.Errorf("load read enforcer: object missing %s map", readRuleMapName))
	}
	if err := InstallReadRules(rulesMap, rules); err != nil {
		return cleanup(err)
	}

	program := collection.Programs["rewind_file_open"]
	if program == nil {
		return cleanup(fmt.Errorf("load read enforcer: object missing rewind_file_open program"))
	}
	attached, err := link.AttachLSM(link.LSMOptions{Program: program})
	if err != nil {
		return cleanup(fmt.Errorf("load read enforcer: attach BPF LSM file_open: %w", err))
	}
	session.link = attached

	eventsMap := collection.Maps["rewind_events"]
	if eventsMap == nil {
		return cleanup(fmt.Errorf("load read enforcer: object missing rewind_events map"))
	}
	events, err := telemetry.NewReader(eventsMap, runID)
	if err != nil {
		return cleanup(fmt.Errorf("load read enforcer: create event reader: %w", err))
	}
	session.events = events
	return session, nil
}

func (s *ReadSession) Events() *telemetry.Reader {
	if s == nil {
		return nil
	}
	return s.events
}

// Close detaches the LSM hook before closing the collection. It is safe to
// call more than once and does not touch any other BPF collection.
func (s *ReadSession) Close() error {
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
	if s.link != nil {
		if err := s.link.Close(); err != nil {
			closeErrs = append(closeErrs, err)
		}
		s.link = nil
	}
	if s.collection != nil {
		s.collection.Close()
		s.collection = nil
	}
	return errors.Join(closeErrs...)
}
