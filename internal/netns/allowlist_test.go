package netns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestBuildAllowlistPlanIsReviewable(t *testing.T) {
	plan, err := BuildAllowlistPlan([]string{"api.example.com", "example.com."})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 12 || plan.Domains[1] != "example.com" {
		t.Fatalf("plan=%+v", plan)
	}
}

type fakeRunner struct{ calls []string }

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, strings.Join(append([]string{name}, args...), " "))
	return nil
}

func TestAllowlistInstallIsCompleteWithoutTouchingHost(t *testing.T) {
	plan, err := BuildAllowlistPlan([]string{"api.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	if err := plan.install(context.Background(), runner, false); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.calls, "\n")
	for _, expected := range []string{"ip link add rewind-host", "10.231.0.2/30", "POSTROUTING", "REWIND_ALLOWLIST", "-j REJECT"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in calls:\n%s", expected, joined)
		}
	}
	if len(runner.calls) != 12 {
		t.Fatalf("calls=%d want 12: %v", len(runner.calls), runner.calls)
	}
}

func TestAllowlistPlanAcceptsOnlyBrokerResolvedIPs(t *testing.T) {
	plan, err := BuildAllowlistPlanWithIPs([]string{"api.example.com"}, map[string][]net.IP{"api.example.com": {net.ParseIP("203.0.113.10")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ResolvedIPs) != 1 || !strings.Contains(strings.Join(plan.Commands, "\n"), "ipset add REWIND_ALLOWLIST4 203.0.113.10") {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestAllowlistInstallPropagatesCommandFailure(t *testing.T) {
	runner := &failingRunner{at: 5}
	plan, _ := BuildAllowlistPlan([]string{"api.example.com"})
	if err := plan.install(context.Background(), runner, false); err == nil || !strings.Contains(err.Error(), "rewind-agent") {
		t.Fatalf("err=%v", err)
	}
}

func TestAllowlistCleanupIsScopedToOwnedNames(t *testing.T) {
	plan, _ := BuildAllowlistPlan([]string{"api.example.com"})
	runner := &fakeRunner{}
	if err := plan.cleanup(context.Background(), runner, false); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.calls, "\n")
	for _, expected := range []string{"FORWARD", "REWIND_ALLOWLIST", "REWIND_ALLOWLIST4", "ip link del rewind-host"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in %s", expected, joined)
		}
	}
	if len(runner.calls) != 6 {
		t.Fatalf("calls=%v", runner.calls)
	}
}

type failingRunner struct{ at, calls int }

func (f *failingRunner) Run(_ context.Context, name string, args ...string) error {
	f.calls++
	if f.calls == f.at {
		return fmt.Errorf("synthetic failure")
	}
	_ = name
	_ = args
	return nil
}

func TestBuildAllowlistPlanRejectsAmbiguousInput(t *testing.T) {
	if _, err := BuildAllowlistPlan([]string{"https://example.com"}); err == nil {
		t.Fatal("expected invalid domain")
	}
}
