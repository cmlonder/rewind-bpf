//go:build darwin

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeatbeltProfileWithRootsKeepsWorkspaceWriteBoundary(t *testing.T) {
	profile, err := SeatbeltProfileWithRoots("/tmp/rewind workspace", []string{"/opt/rewind/runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(profile, `(allow file-read* (subpath "/usr"))`) || !strings.Contains(profile, `(allow file-read* (subpath "/opt/rewind/runtime"))`) {
		t.Fatalf("runtime roots missing from profile: %q", profile)
	}
	if !strings.Contains(profile, `(allow file-write* (subpath "/tmp/rewind workspace"))`) {
		t.Fatalf("workspace write boundary missing: %q", profile)
	}
}

func TestSeatbeltCommandWithOptionsBuildsScopedCommand(t *testing.T) {
	workspace := t.TempDir()
	command, cleanup, err := SeatbeltCommandWithOptions(SeatbeltCommandOptions{
		Workspace:    workspace,
		Command:      "/bin/sh",
		Args:         []string{"-c", "printf 'sandbox-ok\\n' > marker.txt"},
		WorkingDir:   workspace,
		Environment:  []string{"PATH=/usr/bin:/bin"},
		RuntimeRoots: []string{"/usr/bin", "/bin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if len(command.Args) < 5 || command.Args[1] != "-f" || command.Args[3] != "/bin/sh" {
		t.Fatalf("unexpected sandbox command: %v", command.Args)
	}
	profile, err := os.ReadFile(command.Args[2])
	if err != nil {
		t.Fatal(err)
	}
	profileText := string(profile)
	canonicalWorkspace, _ := filepath.EvalSymlinks(workspace)
	if !strings.Contains(profileText, `(allow file-write* (subpath "`+canonicalWorkspace+`"))`) {
		t.Fatalf("workspace write scope missing: %s", profileText)
	}
	if command.Dir != workspace {
		t.Fatalf("working directory = %q", command.Dir)
	}
}

func TestSeatbeltCommandWithOptionsRunsSyntheticWrite(t *testing.T) {
	workspace := t.TempDir()
	command, cleanup, err := SeatbeltCommandWithOptions(SeatbeltCommandOptions{
		Workspace:   workspace,
		Command:     "/bin/sh",
		Args:        []string{"-c", "printf 'sandbox-ok\\n' > marker.txt"},
		WorkingDir:  workspace,
		Environment: []string{"PATH=/usr/bin:/bin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("sandbox write failed: %v (%s)", err, output)
	}
	data, err := os.ReadFile(workspace + "/marker.txt")
	if err != nil || string(data) != "sandbox-ok\n" {
		t.Fatalf("marker=%q err=%v", data, err)
	}
}
