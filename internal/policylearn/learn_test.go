package policylearn

import (
	"strings"
	"testing"

	"github.com/rewindbpf/rewind/internal/policy"
)

func TestLearnFiltersSecretsVirtualAndBroadPaths(t *testing.T) {
	input := strings.NewReader(`{"operation":"openat","path":"/workspace/src/main.go"}
{"operation":"openat","path":"/workspace/src/main.go"}
{"operation":"read","path":"/workspace/.env"}
{"operation":"openat","path":"/sys/devices/system/cpu/possible"}
{"operation":"openat","path":"/etc"}
{"operation":"write","path":"/workspace/generated.txt"}
`)
	report, err := Learn(input, 128)
	if err != nil {
		t.Fatal(err)
	}
	if report.ReadEvents != 5 {
		t.Fatalf("read events = %d, want 5", report.ReadEvents)
	}
	if len(report.Candidates) != 1 || report.Candidates[0].Path != "/workspace/src/main.go" || report.Candidates[0].Count != 2 {
		t.Fatalf("unexpected candidates: %+v", report.Candidates)
	}
	for _, reason := range []string{"secret_like", "virtual_path", "broad_path"} {
		if report.Skipped[reason] == 0 {
			t.Fatalf("missing skip reason %q: %+v", reason, report.Skipped)
		}
	}
}

func TestRenderIsAuditOnlyAndParsesAsPolicy(t *testing.T) {
	data, err := Render(Report{Candidates: []Candidate{{Path: "/workspace/src/main.go"}}})
	if err != nil {
		t.Fatal(err)
	}
	value, err := policy.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if value.Read.Mode != policy.ModeAudit || value.Read.Allow[0] != "/workspace/src/main.go" {
		t.Fatalf("unexpected rendered policy: %+v", value)
	}
}

func TestLearnRejectsMalformedJSONL(t *testing.T) {
	if _, err := Learn(strings.NewReader("not-json\n"), 1); err == nil {
		t.Fatal("expected malformed JSON error")
	}
}
