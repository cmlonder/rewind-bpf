package export

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitPatchRendersBinaryAndStableRelativePaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for native patch rendering")
	}
	before := t.TempDir()
	after := t.TempDir()
	if err := os.WriteFile(filepath.Join(before, "binary.bin"), []byte{0, 1, 2}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(after, "binary.bin"), []byte{0, 3, 4}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(after, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch, err := GitPatch(before, after)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"diff --git a/binary.bin b/binary.bin", "GIT binary patch", "diff --git a/new.txt b/new.txt"} {
		if !strings.Contains(patch, fragment) {
			t.Fatalf("patch missing %q:\n%s", fragment, patch)
		}
	}
	if strings.Contains(patch, before) || strings.Contains(patch, after) {
		t.Fatalf("patch leaked absolute roots:\n%s", patch)
	}
}
