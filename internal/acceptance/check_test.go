package acceptance

import (
	"testing"

	"github.com/rewindbpf/rewind/internal/manifest"
)

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
