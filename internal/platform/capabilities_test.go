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

func TestStatusMatrixKeepsNativeTargetsManualGated(t *testing.T) {
	status, err := StatusMatrix("")
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 3 {
		t.Fatalf("status length = %d", len(status))
	}
	for _, item := range status {
		if !item.CodeComplete {
			t.Fatalf("platform is missing code-complete contract: %+v", item)
		}
		if item.Platform == "linux" {
			continue
		}
		if item.EnforcementReady || !item.ManualGate {
			t.Fatalf("native platform was not kept behind manual gate: %+v", item)
		}
	}
}
