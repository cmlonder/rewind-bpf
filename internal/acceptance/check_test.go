package acceptance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rewindbpf/rewind/internal/manifest"
)

func TestApplyConflictCheckedCandidate(t *testing.T) {
	root := t.TempDir()
	candidateRoot := filepath.Join(root, "candidate")
	destinationRoot := filepath.Join(root, "destination")
	if err := os.MkdirAll(candidateRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidateRoot, "new.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := manifest.Manifest{Version: 1}
	candidate, err := manifest.Build(candidateRoot)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := manifest.Build(destinationRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(base, destination, candidate, candidateRoot, destinationRoot); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destinationRoot, "new.txt"))
	if err != nil || string(data) != "candidate\n" {
		t.Fatalf("applied data=%q err=%v", data, err)
	}
}

func TestCheckAllowsUnrelatedDestinationEdits(t *testing.T) {
	base := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{Path: "a.txt", Type: "file", SHA256: "a"}, {Path: "b.txt", Type: "file", SHA256: "b"}}}
	candidate := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{Path: "a.txt", Type: "file", SHA256: "candidate"}, {Path: "b.txt", Type: "file", SHA256: "b"}}}
	destination := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{Path: "a.txt", Type: "file", SHA256: "a"}, {Path: "b.txt", Type: "file", SHA256: "destination"}}}
	report := Check(base, destination, candidate)
	if !report.CanApply || len(report.Conflicts) != 0 {
		t.Fatalf("unexpected conflict report: %+v", report)
	}
}

func TestCheckRejectsSamePathDrift(t *testing.T) {
	base := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{Path: "a.txt", Type: "file", SHA256: "a"}}}
	candidate := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{Path: "a.txt", Type: "file", SHA256: "candidate"}}}
	destination := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{Path: "a.txt", Type: "file", SHA256: "destination"}}}
	report := Check(base, destination, candidate)
	if report.CanApply || len(report.Conflicts) != 1 || report.Conflicts[0] != "a.txt" {
		t.Fatalf("expected a.txt conflict: %+v", report)
	}
}
