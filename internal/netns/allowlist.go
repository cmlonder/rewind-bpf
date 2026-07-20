package netns

import (
	"fmt"
	"net"
	"strings"
)

// AllowlistPlan is a reviewable veth/NAT setup plan. It is intentionally a
// command plan, not an implicit privileged mutator: a supervisor or VM broker
// must approve and execute it. This keeps the ordinary run path fail-closed.
type AllowlistPlan struct {
	Domains  []string `json:"domains"`
	Commands []string `json:"commands"`
}

func BuildAllowlistPlan(domains []string) (AllowlistPlan, error) {
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
	}
	plan.Commands = []string{
		"ip link add rewind-host type veth peer name rewind-agent",
		"ip addr add 10.231.0.1/30 dev rewind-host",
		"ip addr add 10.231.0.2/30 dev rewind-agent",
		"iptables -t nat -A POSTROUTING -s 10.231.0.0/30 -j MASQUERADE",
		"iptables -A FORWARD -s 10.231.0.2/32 -j REWIND_ALLOWLIST",
	}
	return plan, nil
}
