package export

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rewindbpf/rewind/internal/manifest"
)

func TestBuildAndWriteBundle(t *testing.T) {
	before := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{Path: "marker.txt", Type: "file", SHA256: "before"}}}
	after := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{Path: "marker.txt", Type: "file", SHA256: "after"}, {Path: "new.txt", Type: "file"}}}
	bundle, err := Build("run_test", before, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Changes) != 2 {
		t.Fatalf("changes = %+v", bundle.Changes)
	}
	path := filepath.Join(t.TempDir(), "nested", "bundle.json")
	if err := Write(path, bundle); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}
