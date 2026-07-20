package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rewindbpf/rewind/internal/netpolicy"
	"github.com/rewindbpf/rewind/internal/overlay"
	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/runplan"
	"github.com/rewindbpf/rewind/internal/telemetry"
)

func TestAgentOwnerUsesSudoIdentityWhenRoot(t *testing.T) {
	original := os.Geteuid()
	if original == 0 {
		t.Skip("test must run unprivileged on the development host")
	}
	owner, err := agentOwner()
	if err != nil {
		t.Fatal(err)
	}
	want := overlay.Owner{UID: os.Getuid(), GID: os.Getgid()}
	if owner != want {
		t.Fatalf("owner = %+v, want %+v", owner, want)
	}
}

func TestValidateOnSuccess(t *testing.T) {
	for _, value := range []string{"discard", "review"} {
		if err := validateOnSuccess(value); err != nil {
			t.Fatalf("validateOnSuccess(%q): %v", value, err)
		}
	}
	if err := validateOnSuccess("commit"); err == nil {
		t.Fatal("validateOnSuccess(commit) unexpectedly succeeded")
	}
}

func TestRecordNetworkAppendsHashChainedEvent(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "events-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	adapter := &telemetryAdapter{
		runID:  "run_network_test",
		pid:    42,
		writer: &telemetry.JournalWriter{Destination: file},
	}
	if err := adapter.RecordNetwork("example.invalid", netpolicy.Deny); err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	var journal struct {
		RunID     string `json:"run_id"`
		PID       uint32 `json:"pid"`
		Operation string `json:"operation"`
		Path      string `json:"path"`
		Decision  string `json:"decision"`
		Sequence  uint64 `json:"sequence"`
		Hash      string `json:"hash"`
	}
	if err := json.NewDecoder(bufio.NewReader(file)).Decode(&journal); err != nil {
		t.Fatal(err)
	}
	if journal.RunID != "run_network_test" || journal.PID != 42 || journal.Operation != "network_connect" || journal.Path != "example.invalid" || journal.Decision != "deny" {
		t.Fatalf("unexpected network journal event: %+v", journal)
	}
	if journal.Sequence != 1 || journal.Hash == "" {
		t.Fatalf("journal sequence/hash = %d/%q", journal.Sequence, journal.Hash)
	}
}

func TestScanPIIAfterRunFindsCreatedFileWithoutRawValue(t *testing.T) {
	merged := t.TempDir()
	path := filepath.Join(merged, "generated.txt")
	if err := os.WriteFile(path, []byte("token=ghp_1234567890abcdef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := &runplan.Plan{Layout: overlay.Layout{Merged: merged}, PIIMode: policy.ModeAudit}
	if err := scanPIIAfterRun(plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.PIIFindings) != 1 || plan.PIIFindings[0].ValueHash == "" {
		t.Fatalf("findings=%+v", plan.PIIFindings)
	}
}
