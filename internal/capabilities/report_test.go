package capabilities

import "testing"

func TestContainsWordDoesNotMatchSubstring(t *testing.T) {
	if containsWord("lockdown,capability,landlock_extra", "landlock") {
		t.Fatal("unexpected substring match")
	}
	if !containsWord("lockdown,landlock,yama", "landlock") {
		t.Fatal("expected exact word match")
	}
}

func TestReportJSONIsStableEnoughForInspection(t *testing.T) {
	report := Report{OS: "linux", Arch: "arm64", CgroupV2: true}
	data, err := report.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected JSON report")
	}
}

func TestValidateRejectsRawNetworkEnforcementWithoutSeccomp(t *testing.T) {
	report := Report{FuseOverlayFS: true, CgroupV2: true, Seccomp: false}
	if err := report.ValidateForProtectedRun("fuse", false, true); err == nil {
		t.Fatal("expected raw-network enforcement to fail without seccomp")
	}
}

func TestValidateAcceptsRawNetworkEnforcementWithSeccomp(t *testing.T) {
	report := Report{FuseOverlayFS: true, CgroupV2: true, Seccomp: true}
	if err := report.ValidateForProtectedRun("fuse", false, true); err != nil {
		t.Fatal(err)
	}
}
