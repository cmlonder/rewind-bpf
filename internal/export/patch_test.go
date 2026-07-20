package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewindbpf/rewind/internal/diff"
	"github.com/rewindbpf/rewind/internal/manifest"
)

func TestUnifiedPatchRendersTextFileChanges(t *testing.T) {
	beforeRoot := t.TempDir()
	afterRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(beforeRoot, "changed.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(afterRoot, "changed.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(afterRoot, "created.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{Path: "changed.txt", Type: "file", Mode: 0o644, Size: 7, SHA256: "before"}}}
	after := manifest.Manifest{Version: 1, Entries: []manifest.Entry{
		{Path: "changed.txt", Type: "file", Mode: 0o644, Size: 6, SHA256: "after"},
		{Path: "created.txt", Type: "file", Mode: 0o644, Size: 4, SHA256: "new"},
	}}
	bundle, err := Build("run_patch", before, after)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := UnifiedPatch(bundle, beforeRoot, afterRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"diff --git a/changed.txt b/changed.txt", "-before", "+after", "diff --git a/created.txt b/created.txt", "--- /dev/null", "+new"} {
		if !strings.Contains(patch, fragment) {
			t.Fatalf("patch missing %q:\n%s", fragment, patch)
		}
	}
}

func TestUnifiedPatchRefusesBinaryAndUnsafeChanges(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "binary.bin"), []byte{0, 1}, 0o644); err != nil {
		t.Fatal(err)
	}
	binaryBefore := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{Path: "binary.bin", Type: "file", Size: 2}}}
	binaryAfter := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{Path: "binary.bin", Type: "file", Size: 2, SHA256: "changed"}}}
	bundle, err := Build("run_binary", binaryBefore, binaryAfter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnifiedPatch(bundle, root, root); err == nil {
		t.Fatal("expected binary patch refusal")
	}
	unsafe := Bundle{Changes: []diff.Change{{Path: "../escape", Kind: diff.Created, After: &manifest.Entry{Path: "../escape", Type: "file"}}}}
	if _, err := UnifiedPatch(unsafe, root, root); err == nil {
		t.Fatal("expected unsafe path refusal")
	}
}
