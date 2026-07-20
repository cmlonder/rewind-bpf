// Package credentials defines a capability-only broker boundary. Raw secret
// values intentionally have no representation in this package.
package credentials

import (
	"fmt"
	"strings"
	"time"

	"github.com/rewindbpf/rewind/internal/policy"
)

type Request struct {
	Ref    string
	Scopes []string
}

type Lease struct {
	ID            string    `json:"id"`
	Ref           string    `json:"ref"`
	Scopes        []string  `json:"scopes,omitempty"`
	ExpiresAt     time.Time `json:"expires_at"`
	SecretExposed bool      `json:"secret_exposed"`
}

type Broker interface {
	Issue(Request) (Lease, error)
}

// RefusingBroker is the safe default until a platform-specific secret broker
// is configured. It prevents accidental fallback to environment injection.
type RefusingBroker struct{}

func (RefusingBroker) Issue(Request) (Lease, error) {
	return Lease{}, fmt.Errorf("credential broker backend unavailable; refusing raw secret exposure")
}

func ValidateRequest(ref policy.CredentialRef, request Request) error {
	if strings.TrimSpace(request.Ref) == "" || strings.TrimSpace(ref.Name) == "" || request.Ref != ref.Name {
		return fmt.Errorf("credential reference mismatch")
	}
	allowed := make(map[string]struct{}, len(ref.Scopes))
	for _, scope := range ref.Scopes {
		allowed[strings.TrimSpace(scope)] = struct{}{}
	}
	for _, scope := range request.Scopes {
		if _, ok := allowed[strings.TrimSpace(scope)]; !ok {
			return fmt.Errorf("credential scope %q is not allowed for %s", scope, ref.Name)
		}
	}
	return nil
}
