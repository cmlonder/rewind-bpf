package platform

import (
	"runtime"
	"testing"
)

func TestProbeNeverAdvertisesUnsupportedNativeBackends(t *testing.T) {
	report := Probe()
	if report.Platform != runtime.GOOS {
		t.Fatalf("platform mismatch: %+v", report)
	}
	if runtime.GOOS != "linux" && report.Supported {
		t.Fatalf("native backend should remain disabled on %s: %+v", runtime.GOOS, report)
	}
}

func TestUnsupportedCapabilityFailsClosed(t *testing.T) {
	if err := (Capability{Platform: "darwin", Backend: "seatbelt-apfs"}).ValidateForRun(); err == nil {
		t.Fatal("unsupported backend should fail closed")
	}
}
