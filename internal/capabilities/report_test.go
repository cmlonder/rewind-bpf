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
