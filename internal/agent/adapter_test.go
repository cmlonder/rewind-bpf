package agent

import "testing"

func TestResolveKnownAdapters(t *testing.T) {
	for _, name := range []string{"generic", "codex", "openhands", "claude-code"} {
		if _, err := Resolve(name); err != nil {
			t.Fatalf("Resolve(%q): %v", name, err)
		}
	}
}

func TestResolveRejectsUnknownAdapter(t *testing.T) {
	if _, err := Resolve("unknown"); err == nil {
		t.Fatal("expected unknown adapter error")
	}
}

func TestValidateCommandRejectsEmptyVector(t *testing.T) {
	spec, _ := Resolve("generic")
	if err := ValidateCommand(spec, nil); err == nil {
		t.Fatal("expected empty command error")
	}
}
