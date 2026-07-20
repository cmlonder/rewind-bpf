package netns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

// Boundary is the lifecycle hook used by protectedrun. Prepare runs while
// the child is paused at the start gate, so the peer can be moved into the
// child's already-created network namespace before the agent execs. Cleanup
// is idempotent-friendly and is called on both success and rollback paths.
type Boundary interface {
	Prepare(context.Context, uint32) error
	Cleanup(context.Context) error
}

// Broker owns the privileged network edge for one run. The command runner is
// injectable for tests; production uses OSCommandRunner and therefore remains
// an explicit root-gated operation.
type Broker struct {
	Plan               AllowlistPlan
	Runner             CommandRunner
	RequireRoot        bool
	previousForwarding string
}

func (b *Broker) Prepare(ctx context.Context, pid uint32) error {
	if b == nil {
		return fmt.Errorf("namespace broker is nil")
	}
	if pid == 0 {
		return fmt.Errorf("namespace broker child pid is zero")
	}
	if b.Runner == nil {
		b.Runner = OSCommandRunner{}
	}
	if b.requireRoot() {
		value, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
		if err != nil {
			return fmt.Errorf("read IPv4 forwarding state: %w", err)
		}
		b.previousForwarding = strings.TrimSpace(string(value))
	}
	if err := b.Plan.install(ctx, b.Runner, b.requireRoot()); err != nil {
		_ = b.Cleanup(ctx)
		return err
	}
	commands := [][]string{
		{"ip", "link", "set", "rewind-agent", "netns", fmt.Sprint(pid)},
		{"nsenter", "-t", fmt.Sprint(pid), "-n", "--", "ip", "link", "set", "lo", "up"},
		{"nsenter", "-t", fmt.Sprint(pid), "-n", "--", "ip", "addr", "replace", "10.231.0.2/30", "dev", "rewind-agent"},
		{"nsenter", "-t", fmt.Sprint(pid), "-n", "--", "ip", "link", "set", "rewind-agent", "up"},
		{"nsenter", "-t", fmt.Sprint(pid), "-n", "--", "ip", "route", "replace", "default", "via", "10.231.0.1", "dev", "rewind-agent"},
	}
	for _, command := range commands {
		if err := b.Runner.Run(ctx, command[0], command[1:]...); err != nil {
			_ = b.Plan.Cleanup(ctx, b.Runner)
			return fmt.Errorf("namespace broker command %s: %w", strings.Join(command, " "), err)
		}
	}
	return nil
}

func (b *Broker) Cleanup(ctx context.Context) error {
	if b == nil {
		return nil
	}
	if b.Runner == nil {
		b.Runner = OSCommandRunner{}
	}
	err := b.Plan.cleanup(ctx, b.Runner, b.requireRoot())
	if b.previousForwarding != "" {
		if restoreErr := b.Runner.Run(ctx, "sysctl", "-w", "net.ipv4.ip_forward="+b.previousForwarding); err == nil {
			err = restoreErr
		} else if restoreErr != nil {
			err = fmt.Errorf("%v; restore IPv4 forwarding: %w", err, restoreErr)
		}
		b.previousForwarding = ""
	}
	return err
}

func (b *Broker) requireRoot() bool {
	// Production callers must set RequireRoot=true. Tests and VM command-plan
	// harnesses explicitly set it to false to validate sequencing without
	// mutation.
	return b == nil || b.RequireRoot
}

// AllowlistPlan is a reviewable veth/NAT setup plan. It is intentionally a
// command plan, not an implicit privileged mutator: a supervisor or VM broker
// must approve and execute it. This keeps the ordinary run path fail-closed.
type AllowlistPlan struct {
	Domains     []string `json:"domains"`
	ResolvedIPs []string `json:"resolved_ips,omitempty"`
	ResolverIPs []string `json:"resolver_ips,omitempty"`
	Commands    []string `json:"commands"`
}

// CommandRunner is injectable so the privileged broker can be tested without
// touching the host network namespace.
type CommandRunner interface {
	Run(context.Context, string, ...string) error
}

type OSCommandRunner struct{}

func (OSCommandRunner) Run(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("%w: %s", err, message)
		}
	}
	return err
}

// ResolveDomains performs DNS resolution before any privileged mutation. The
// resulting IPv4 set is what the broker installs into ipset, so post-start
// traffic is matched by address rather than by an untrusted hostname string.
func ResolveDomains(ctx context.Context, domains []string) (map[string][]net.IP, error) {
	if len(domains) == 0 {
		return nil, fmt.Errorf("namespace allowlist requires at least one domain")
	}
	result := make(map[string][]net.IP, len(domains))
	for _, raw := range domains {
		host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
		if host == "" || net.ParseIP(host) != nil || strings.ContainsAny(host, "/ :") {
			return nil, fmt.Errorf("invalid namespace allowlist domain %q", raw)
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if err != nil {
			return nil, fmt.Errorf("resolve namespace allowlist domain %q: %w", host, err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("resolve namespace allowlist domain %q: no IPv4 addresses", host)
		}
		result[host] = ips
	}
	return result, nil
}

// ResolveNameservers reads the host resolver configuration without changing
// it. Resolver addresses are added to the same destination ipset so the
// child can perform DNS lookups, while the resolved application addresses
// remain the only destinations allowed for ordinary egress.
func ResolveNameservers() ([]net.IP, error) {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil, fmt.Errorf("read resolver configuration: %w", err)
	}
	var result []net.IP
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != "nameserver" {
			continue
		}
		ip := net.ParseIP(fields[1])
		if ip == nil || ip.To4() == nil || ip.IsLoopback() {
			continue
		}
		result = append(result, ip.To4())
	}
	return result, nil
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
	var errs []error
	for _, command := range commands {
		if err := runner.Run(ctx, command[0], command[1:]...); err != nil {
			if isAlreadyGone(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("namespace allowlist cleanup command %s: %w", strings.Join(command, " "), err))
		}
	}
	return errors.Join(errs...)
}

func isAlreadyGone(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"cannot find device",
		"no chain/target/match by that name",
		"does not exist",
		"name does not exist",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
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
		{"sysctl", "-w", "net.ipv4.ip_forward=1"},
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
	for _, ip := range p.ResolverIPs {
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
	return BuildAllowlistPlanWithIPsAndResolvers(domains, resolved, nil)
}

func BuildAllowlistPlanWithIPsAndResolvers(domains []string, resolved map[string][]net.IP, resolvers []net.IP) (AllowlistPlan, error) {
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
	for _, ip := range resolvers {
		if ip == nil || ip.To4() == nil {
			return AllowlistPlan{}, fmt.Errorf("invalid IPv4 resolver address")
		}
		plan.ResolverIPs = append(plan.ResolverIPs, ip.To4().String())
	}
	plan.Commands = []string{
		"ip link add rewind-host type veth peer name rewind-agent",
		"ip addr add 10.231.0.1/30 dev rewind-host",
		"ip addr add 10.231.0.2/30 dev rewind-agent",
		"ip link set rewind-host up",
		"ip link set rewind-agent up",
		"sysctl -w net.ipv4.ip_forward=1",
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
	for _, ip := range plan.ResolverIPs {
		plan.Commands = append(plan.Commands, "ipset add REWIND_ALLOWLIST4 "+ip+" -exist")
	}
	return plan, nil
}
