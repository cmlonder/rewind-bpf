package policy

import "testing"

func TestMatchSupportsRecursivePatterns(t *testing.T) {
	tests := []struct {
		pattern, candidate string
		want               bool
	}{
		{"**/.env", ".env", true},
		{"**/.env", "/workspace/.env", true},
		{"**/*.pem", "/home/demo/key.pem", true},
		{"/home/*/.ssh/**", "/home/demo/.ssh/config", true},
		{"/home/*/.ssh/**", "/home/demo/.ssh", true},
		{"/workspace/**", "/workspace/src/main.go", true},
		{"**/.env", "/workspace/.env.example", false},
	}
	for _, test := range tests {
		if got := Match(test.pattern, test.candidate); got != test.want {
			t.Errorf("Match(%q, %q) = %v, want %v", test.pattern, test.candidate, got, test.want)
		}
	}
}

func TestReadPolicyModesAndDenyPrecedence(t *testing.T) {
	read := ReadPolicy{Mode: ModeEnforce, Deny: []string{"**/.env"}, Allow: []string{"/workspace/.env"}}
	if got := read.Decision("/workspace/.env"); got != "deny" {
		t.Fatalf("deny must beat allow: got %q", got)
	}
	if got := read.Decision("/other/.env"); got != "deny" {
		t.Fatalf("deny decision: got %q", got)
	}
	read.Mode = ModeAudit
	if got := read.Decision("/other/.env"); got != "audit" {
		t.Fatalf("audit decision: got %q", got)
	}
	explanation := read.Explain("/workspace/.env")
	if explanation.Rule != "deny" || explanation.MatchedPattern != "**/.env" || explanation.Decision != "audit" {
		t.Fatalf("unexpected explanation: %+v", explanation)
	}
}

func TestParseAndValidateYAML(t *testing.T) {
	value, err := Parse([]byte("read:\n  mode: enforce\n  deny:\n    - '**/*.pem'\nwrite:\n  mode: rollback\n  scope: workspace\nresources:\n  pids_max: '128'\n  memory_max: '268435456'\n  cpu_max: '50000 100000'\n"))
	if err != nil {
		t.Fatal(err)
	}
	if value.Read.Mode != ModeEnforce || len(value.Read.Deny) != 1 {
		t.Fatalf("unexpected policy: %+v", value)
	}
	if value.Resources.PIDsMax != "128" || value.Resources.CPUMax != "50000 100000" {
		t.Fatalf("unexpected resource policy: %+v", value.Resources)
	}
}

func TestResourcePolicyRejectsInvalidLimits(t *testing.T) {
	for _, input := range []string{"0", "-1", "bytes", ""} {
		value := Policy{Resources: ResourcePolicy{PIDsMax: input}}
		if input == "" {
			continue
		}
		if err := value.Validate(); err == nil {
			t.Fatalf("PIDsMax %q should fail validation", input)
		}
	}
	if err := (Policy{Resources: ResourcePolicy{CPUMax: "1000"}}).Validate(); err == nil {
		t.Fatal("short cpu.max should fail validation")
	}
}
