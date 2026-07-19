package credentials

import (
	"strings"
	"testing"

	"github.com/rewindbpf/rewind/internal/policy"
)

func TestRefusingBrokerNeverExposesSecrets(t *testing.T) {
	_, err := (RefusingBroker{}).Issue(Request{Ref: "github", Scopes: []string{"read:org"}})
	if err == nil || !strings.Contains(err.Error(), "refusing raw secret exposure") {
		t.Fatalf("expected refusal, got %v", err)
	}
}

func TestValidateRequestScopes(t *testing.T) {
	ref := policy.CredentialRef{Name: "github", Scopes: []string{"read:org"}}
	if err := ValidateRequest(ref, Request{Ref: "github", Scopes: []string{"read:org"}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequest(ref, Request{Ref: "github", Scopes: []string{"repo:write"}}); err == nil {
		t.Fatal("unexpectedly accepted unlisted scope")
	}
}
