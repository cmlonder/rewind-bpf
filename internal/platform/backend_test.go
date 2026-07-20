package platform

import (
	"context"
	"testing"
)

func TestUnavailableBackendRefusesPrepare(t *testing.T) {
	backend := UnavailableBackend{Report: Capability{Platform: "darwin", Backend: "seatbelt-apfs", Reasons: []string{"not tested"}}}
	if _, err := backend.Prepare(context.Background(), "/tmp/project"); err == nil {
		t.Fatal("unavailable backend unexpectedly prepared")
	}
	if backend.Capabilities().Backend != "seatbelt-apfs" {
		t.Fatal("capability report changed")
	}
}
