package netpolicy

import (
	"testing"

	"github.com/rewindbpf/rewind/internal/policy"
)

func TestExplainModesAndDomainBoundaries(t *testing.T) {
	plan, err := Compile(policy.NetworkPolicy{Mode: policy.ModeEnforce, AllowDomains: []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Explain("api.example.com"); got != Allow {
		t.Fatalf("subdomain should be allowed, got %s", got)
	}
	if got := plan.Explain("example.com.evil.test"); got != Deny {
		t.Fatalf("suffix confusion should be denied, got %s", got)
	}
	plan.Mode = policy.ModeAudit
	if got := plan.Explain("anything.invalid"); got != Audit {
		t.Fatalf("audit mode should produce audit, got %s", got)
	}
}
