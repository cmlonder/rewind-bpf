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
	if len(plan.Commands) != 13 || plan.Domains[1] != "example.com" {
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
	if len(runner.calls) != 13 {
		t.Fatalf("calls=%d want 13: %v", len(runner.calls), runner.calls)
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

func TestBrokerMovesPeerAndConfiguresChildNamespace(t *testing.T) {
	plan, err := BuildAllowlistPlanWithIPs([]string{"api.example.com"}, map[string][]net.IP{"api.example.com": {net.ParseIP("203.0.113.10")}})
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	broker := &Broker{Plan: plan, Runner: runner, RequireRoot: false}
	if err := broker.Prepare(context.Background(), 4242); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.calls, "\n")
	for _, expected := range []string{
		"ip link set rewind-agent netns 4242",
		"nsenter -t 4242 -n -- ip link set lo up",
		"nsenter -t 4242 -n -- ip addr replace 10.231.0.2/30 dev rewind-agent",
		"nsenter -t 4242 -n -- ip route replace default via 10.231.0.1 dev rewind-agent",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in calls:\n%s", expected, joined)
		}
	}
	if err := broker.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(runner.calls, "\n"), "ip link del rewind-host") {
		t.Fatal("broker cleanup did not remove owned host veth")
	}
}

func TestBrokerRefreshAtomicallySwapsResolvedSet(t *testing.T) {
	plan, err := BuildAllowlistPlanWithIPsAndResolvers([]string{"api.example.com"}, map[string][]net.IP{"api.example.com": {net.ParseIP("203.0.113.10")}}, []net.IP{net.ParseIP("192.0.2.53")})
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	broker := &Broker{
		Plan:   plan,
		Runner: runner,
		ResolveDomains: func(context.Context, []string) (map[string][]net.IP, error) {
			return map[string][]net.IP{"api.example.com": {net.ParseIP("203.0.113.20"), net.ParseIP("203.0.113.20")}}, nil
		},
		ResolveNameservers: func() ([]net.IP, error) { return []net.IP{net.ParseIP("192.0.2.54")}, nil },
	}
	if err := broker.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.calls, "\n")
	for _, expected := range []string{
		"ipset create REWIND_ALLOWLIST4_NEXT hash:ip family inet -exist",
		"ipset add REWIND_ALLOWLIST4_NEXT 192.0.2.54 -exist",
		"ipset add REWIND_ALLOWLIST4_NEXT 203.0.113.20 -exist",
		"ipset swap REWIND_ALLOWLIST4 REWIND_ALLOWLIST4_NEXT",
		"ipset destroy REWIND_ALLOWLIST4_NEXT",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in calls:\n%s", expected, joined)
		}
	}
	if len(broker.Plan.ResolvedIPs) != 1 || broker.Plan.ResolvedIPs[0] != "203.0.113.20" || broker.Plan.ResolverIPs[0] != "192.0.2.54" {
		t.Fatalf("plan not refreshed: %+v", broker.Plan)
	}
}

func TestBrokerRefreshLeavesPlanUntouchedWhenResolutionFails(t *testing.T) {
	plan, err := BuildAllowlistPlanWithIPsAndResolvers([]string{"api.example.com"}, map[string][]net.IP{"api.example.com": {net.ParseIP("203.0.113.10")}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	broker := &Broker{Plan: plan, Runner: runner, ResolveDomains: func(context.Context, []string) (map[string][]net.IP, error) {
		return nil, fmt.Errorf("resolver offline")
	}}
	if err := broker.Refresh(context.Background()); err == nil || !strings.Contains(err.Error(), "resolver offline") {
		t.Fatalf("err=%v", err)
	}
	if len(runner.calls) != 0 || broker.Plan.ResolvedIPs[0] != "203.0.113.10" {
		t.Fatalf("refresh mutated state after resolution failure: calls=%v plan=%+v", runner.calls, broker.Plan)
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

func TestAllowlistCleanupContinuesAfterIndividualFailure(t *testing.T) {
	plan, _ := BuildAllowlistPlan([]string{"api.example.com"})
	runner := &failingRunner{at: 1}
	if err := plan.cleanup(context.Background(), runner, false); err == nil {
		t.Fatal("expected cleanup error")
	}
	if runner.calls != 6 {
		t.Fatalf("cleanup stopped after first error: calls=%d", runner.calls)
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
