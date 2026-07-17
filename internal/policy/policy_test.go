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

func TestReadPolicyModesAndAllowOverride(t *testing.T) {
	read := ReadPolicy{Mode: ModeEnforce, Deny: []string{"**/.env"}, Allow: []string{"/workspace/.env"}}
	if got := read.Decision("/workspace/.env"); got != "allow" {
		t.Fatalf("allow override: got %q", got)
	}
	if got := read.Decision("/other/.env"); got != "deny" {
		t.Fatalf("deny decision: got %q", got)
	}
	read.Mode = ModeAudit
	if got := read.Decision("/other/.env"); got != "audit" {
		t.Fatalf("audit decision: got %q", got)
	}
}

func TestParseAndValidateYAML(t *testing.T) {
	value, err := Parse([]byte("read:\n  mode: enforce\n  deny:\n    - '**/*.pem'\nwrite:\n  mode: rollback\n  scope: workspace\n"))
	if err != nil {
		t.Fatal(err)
	}
	if value.Read.Mode != ModeEnforce || len(value.Read.Deny) != 1 {
		t.Fatalf("unexpected policy: %+v", value)
	}
}
