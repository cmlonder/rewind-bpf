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

func TestPreparePreservesCommandAndAddsIdentityMarker(t *testing.T) {
	spec, _ := Resolve("codex")
	launch, err := Prepare(spec, []string{"codex", "--task", "review"})
	if err != nil {
		t.Fatal(err)
	}
	if len(launch.Command) != 3 || launch.Command[1] != "--task" || launch.Environment[0] != "REWIND_AGENT_ADAPTER=codex" {
		t.Fatalf("launch=%+v", launch)
	}
}
