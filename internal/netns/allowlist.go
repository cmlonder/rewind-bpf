package netns

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

// AllowlistPlan is a reviewable veth/NAT setup plan. It is intentionally a
// command plan, not an implicit privileged mutator: a supervisor or VM broker
// must approve and execute it. This keeps the ordinary run path fail-closed.
type AllowlistPlan struct {
	Domains     []string `json:"domains"`
	ResolvedIPs []string `json:"resolved_ips,omitempty"`
	Commands    []string `json:"commands"`
}

// CommandRunner is injectable so the privileged broker can be tested without
// touching the host network namespace.
type CommandRunner interface {
	Run(context.Context, string, ...string) error
}

type OSCommandRunner struct{}

func (OSCommandRunner) Run(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// Install is the reviewed privileged edge for the veth/NAT prototype. It
// refuses non-root callers and installs an explicit iptables chain; callers
// still need to move the peer into the agent namespace and configure DNS.
func (p AllowlistPlan) Install(ctx context.Context, runner CommandRunner) error {
	return p.install(ctx, runner, true)
}

// Cleanup removes only names owned by this plan. It is intentionally explicit
// and idempotent-friendly so a broker can run it from both normal and crash
// recovery paths without touching an operator's unrelated firewall rules.
func (p AllowlistPlan) Cleanup(ctx context.Context, runner CommandRunner) error {
	return p.cleanup(ctx, runner, true)
}

func (p AllowlistPlan) cleanup(ctx context.Context, runner CommandRunner, requireRoot bool) error {
	if requireRoot && os.Geteuid() != 0 {
		return fmt.Errorf("namespace allowlist cleanup requires root")
	}
	if runner == nil {
		return fmt.Errorf("namespace allowlist command runner is required")
	}
	commands := [][]string{
		{"iptables", "-D", "FORWARD", "-s", "10.231.0.2/32", "-j", "REWIND_ALLOWLIST"},
		{"iptables", "-t", "nat", "-D", "POSTROUTING", "-s", "10.231.0.0/30", "-j", "MASQUERADE"},
		{"iptables", "-F", "REWIND_ALLOWLIST"},
		{"iptables", "-X", "REWIND_ALLOWLIST"},
		{"ipset", "destroy", "REWIND_ALLOWLIST4"},
		{"ip", "link", "del", "rewind-host"},
	}
	for _, command := range commands {
		if err := runner.Run(ctx, command[0], command[1:]...); err != nil {
			return fmt.Errorf("namespace allowlist cleanup command %s: %w", strings.Join(command, " "), err)
		}
	}
	return nil
}

// install is split from Install so unit tests can validate the complete
// command sequence without requiring a real host network namespace.
func (p AllowlistPlan) install(ctx context.Context, runner CommandRunner, requireRoot bool) error {
	if requireRoot && os.Geteuid() != 0 {
		return fmt.Errorf("namespace allowlist install requires root")
	}
	if runner == nil {
		return fmt.Errorf("namespace allowlist command runner is required")
	}
	commands := [][]string{
		{"ip", "link", "add", "rewind-host", "type", "veth", "peer", "name", "rewind-agent"},
		{"ip", "addr", "add", "10.231.0.1/30", "dev", "rewind-host"},
		{"ip", "addr", "add", "10.231.0.2/30", "dev", "rewind-agent"},
		{"ip", "link", "set", "rewind-host", "up"},
		{"ip", "link", "set", "rewind-agent", "up"},
		{"iptables", "-N", "REWIND_ALLOWLIST"},
		{"ipset", "create", "REWIND_ALLOWLIST4", "hash:ip", "family", "inet", "-exist"},
		{"iptables", "-t", "nat", "-A", "POSTROUTING", "-s", "10.231.0.0/30", "-j", "MASQUERADE"},
		{"iptables", "-A", "FORWARD", "-s", "10.231.0.2/32", "-j", "REWIND_ALLOWLIST"},
		{"iptables", "-A", "REWIND_ALLOWLIST", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
		{"iptables", "-A", "REWIND_ALLOWLIST", "-m", "set", "--match-set", "REWIND_ALLOWLIST4", "dst", "-j", "ACCEPT"},
		{"iptables", "-A", "REWIND_ALLOWLIST", "-j", "REJECT"},
	}
	for _, ip := range p.ResolvedIPs {
		commands = append(commands, []string{"ipset", "add", "REWIND_ALLOWLIST4", ip, "-exist"})
	}
	for _, command := range commands {
		if err := runner.Run(ctx, command[0], command[1:]...); err != nil {
			return fmt.Errorf("namespace allowlist command %s: %w", strings.Join(command, " "), err)
		}
	}
	return nil
}

func BuildAllowlistPlan(domains []string) (AllowlistPlan, error) {
	return BuildAllowlistPlanWithIPs(domains, nil)
}

// BuildAllowlistPlanWithIPs keeps DNS resolution outside the privileged
// installer. A resolver/broker supplies validated addresses; an empty map is
// intentionally fail-closed (the REWIND_ALLOWLIST4 set stays empty).
func BuildAllowlistPlanWithIPs(domains []string, resolved map[string][]net.IP) (AllowlistPlan, error) {
	if len(domains) == 0 {
		return AllowlistPlan{}, fmt.Errorf("namespace allowlist requires at least one domain")
	}
	plan := AllowlistPlan{Domains: make([]string, 0, len(domains))}
	for _, raw := range domains {
		host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
		if host == "" || net.ParseIP(host) != nil || strings.ContainsAny(host, "/ :") {
			return AllowlistPlan{}, fmt.Errorf("invalid namespace allowlist domain %q", raw)
		}
		plan.Domains = append(plan.Domains, host)
		for _, ip := range resolved[host] {
			if ip == nil || ip.To4() == nil {
				return AllowlistPlan{}, fmt.Errorf("invalid IPv4 resolution for %q", host)
			}
			plan.ResolvedIPs = append(plan.ResolvedIPs, ip.String())
		}
	}
	plan.Commands = []string{
		"ip link add rewind-host type veth peer name rewind-agent",
		"ip addr add 10.231.0.1/30 dev rewind-host",
		"ip addr add 10.231.0.2/30 dev rewind-agent",
		"ip link set rewind-host up",
		"ip link set rewind-agent up",
		"iptables -N REWIND_ALLOWLIST",
		"ipset create REWIND_ALLOWLIST4 hash:ip family inet -exist",
		"iptables -t nat -A POSTROUTING -s 10.231.0.0/30 -j MASQUERADE",
		"iptables -A FORWARD -s 10.231.0.2/32 -j REWIND_ALLOWLIST",
		"iptables -A REWIND_ALLOWLIST -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT",
		"iptables -A REWIND_ALLOWLIST -m set --match-set REWIND_ALLOWLIST4 dst -j ACCEPT",
		"iptables -A REWIND_ALLOWLIST -j REJECT",
	}
	for _, ip := range plan.ResolvedIPs {
		plan.Commands = append(plan.Commands, "ipset add REWIND_ALLOWLIST4 "+ip+" -exist")
	}
	return plan, nil
}
