package protectedrun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rewindbpf/rewind/internal/landlock"
)

func TestWritePlanRoundTripsWithPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	want := landlock.Plan{Root: "/tmp/run/merged", AllowedFiles: []string{"/tmp/run/merged/a"}}
	if err := writePlan(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("plan mode = %o, want 600", got)
	}
	var got landlock.Plan
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(file).Decode(&got); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if got.Root != want.Root || len(got.AllowedFiles) != 1 || got.AllowedFiles[0] != want.AllowedFiles[0] {
		t.Fatalf("plan = %+v, want %+v", got, want)
	}
}

func TestExecStarterRequiresHelperAndCommand(t *testing.T) {
	starter := ExecStarter{}
	if _, err := starter.Start(nil, []string{"agent"}, "/tmp", nil); err == nil {
		t.Fatal("expected helper path validation error")
	}
	starter.HelperPath = "/bin/true"
	if _, err := starter.Start(nil, nil, "/tmp", nil); err == nil {
		t.Fatal("expected command validation error")
	}
}
