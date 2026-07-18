//go:build linux

package overlay

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// This test is opt-in because it invokes mount/umount. It owns all paths
// below t.TempDir and never touches a project, home directory, or host mount.
func TestOverlaySyntheticMountRollback(t *testing.T) {
	if os.Getenv("REWIND_OVERLAY_INTEGRATION") != "1" {
		t.Skip("set REWIND_OVERLAY_INTEGRATION=1 inside the disposable VM")
	}
	if os.Geteuid() != 0 {
		t.Skip("OverlayFS integration requires root or CAP_SYS_ADMIN in the VM")
	}

	layout, err := NewLayout(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Prepare(); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(layout.Lower, "marker.txt")
	if err := os.WriteFile(marker, []byte("lower-layer-original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := Manager{}
	if err := manager.Mount(context.Background(), layout); err != nil {
		t.Fatalf("mount synthetic overlay: %v", err)
	}
	mounted := true
	defer func() {
		if mounted {
			_ = manager.Unmount(context.Background(), layout)
		}
	}()

	mergedMarker := filepath.Join(layout.Merged, "marker.txt")
	if data, err := os.ReadFile(mergedMarker); err != nil || string(data) != "lower-layer-original\n" {
		t.Fatalf("initial merged marker = %q, err=%v", data, err)
	}
	if err := os.WriteFile(mergedMarker, []byte("upper-layer-change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "lower-layer-original\n" {
		t.Fatalf("lower marker changed before rollback = %q, err=%v", data, err)
	}

	if err := manager.Rollback(context.Background(), layout); err != nil {
		t.Fatal(err)
	}
	mounted = false
	if data, err := os.ReadFile(marker); err != nil || string(data) != "lower-layer-original\n" {
		t.Fatalf("lower marker changed after rollback = %q, err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(layout.Upper, "marker.txt")); !os.IsNotExist(err) {
		t.Fatalf("upper marker still exists or unexpected error: %v", err)
	}
}
