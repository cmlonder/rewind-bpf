package platform

import (
	"strings"
	"testing"
)

func TestNativeContractsFailClosedAndDescribePrimitives(t *testing.T) {
	mac, err := BuildNativeContract("darwin", "/tmp/workspace")
	if err != nil || mac.ReadBoundary == "" || mac.Ready {
		t.Fatalf("mac=%+v err=%v", mac, err)
	}
	win, err := BuildNativeContract("windows", `C:\workspace`)
	if err != nil || !strings.Contains(win.ProcessBoundary, "Job Object") || win.Ready {
		t.Fatalf("windows=%+v err=%v", win, err)
	}
}

func TestSeatbeltProfileIsWorkspaceScoped(t *testing.T) {
	profile, err := SeatbeltProfile("/tmp/rewind workspace")
	if err != nil || !strings.Contains(profile, "(deny default)") || !strings.Contains(profile, "rewind workspace") {
		t.Fatalf("profile=%q err=%v", profile, err)
	}
}
