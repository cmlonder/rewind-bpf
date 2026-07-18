//go:build linux

package landlock

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/policycompile"
	"golang.org/x/sys/unix"
)

// This test is opt-in because it applies a process security policy. It only
// uses t.TempDir and a child process, never a mount or a host/project path.
func TestLandlockSyntheticReadEnforcement(t *testing.T) {
	if os.Getenv("REWIND_LANDLOCK_CHILD") == "1" {
		runLandlockChild(t)
		return
	}
	if os.Getenv("REWIND_LANDLOCK_INTEGRATION") != "1" {
		t.Skip("set REWIND_LANDLOCK_INTEGRATION=1 inside the disposable VM")
	}

	root := t.TempDir()
	public := filepath.Join(root, "public.txt")
	secret := filepath.Join(root, "synthetic-secret.txt")
	if err := os.WriteFile(public, []byte("public\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("not-for-agent\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestLandlockSyntheticReadEnforcement$")
	cmd.Env = append(os.Environ(),
		"REWIND_LANDLOCK_CHILD=1",
		"REWIND_LANDLOCK_ROOT="+root,
		"REWIND_LANDLOCK_PUBLIC="+public,
		"REWIND_LANDLOCK_SECRET="+secret,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Landlock child failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "landlock-ok") {
		t.Fatalf("unexpected child output: %s", output)
	}
}

func runLandlockChild(t *testing.T) {
	t.Helper()
	root := os.Getenv("REWIND_LANDLOCK_ROOT")
	public := os.Getenv("REWIND_LANDLOCK_PUBLIC")
	secret := os.Getenv("REWIND_LANDLOCK_SECRET")
	rules := policycompile.ReadRules{
		Mode:         policy.ModeEnforce,
		AllowedFiles: []string{public},
		AllowedDirs:  []string{root},
	}
	plan, err := BuildPlan(root, rules, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(public); err != nil || string(data) != "public\n" {
		t.Fatalf("allowed read failed: %v", err)
	}
	if _, err := os.ReadFile(secret); !errors.Is(err, unix.EACCES) {
		t.Fatalf("secret read error = %v, want EACCES", err)
	}
	fmt.Println("landlock-ok: allowed file readable; synthetic secret denied")
}
