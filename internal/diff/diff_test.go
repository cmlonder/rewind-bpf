package diff

import (
	"testing"

	"github.com/rewindbpf/rewind/internal/manifest"
)

func TestCompareReportsCreatedModifiedDeleted(t *testing.T) {
	before := manifest.Manifest{Version: 1, Entries: []manifest.Entry{
		{Path: "deleted.txt", Type: "file", SHA256: "a"},
		{Path: "changed.txt", Type: "file", SHA256: "a"},
	}}
	after := manifest.Manifest{Version: 1, Entries: []manifest.Entry{
		{Path: "changed.txt", Type: "file", SHA256: "b"},
		{Path: "created.txt", Type: "file", SHA256: "c"},
	}}
	changes := Compare(before, after)
	if len(changes) != 3 {
		t.Fatalf("changes = %+v", changes)
	}
	if changes[0].Path != "changed.txt" || changes[1].Path != "created.txt" || changes[2].Path != "deleted.txt" {
		t.Fatalf("changes are not sorted: %+v", changes)
	}
}
