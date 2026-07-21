// Package credentials defines a capability-only broker boundary. Raw secret
// values intentionally have no representation in this package.
package credentials

import (
	"fmt"
	"io"
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

// ScopedConsumer receives a lease's secret through a short-lived reader. The
// broker consumes the lease exactly once and clears its in-memory copy when
// the callback returns. Callers should use this boundary instead of exporting
// a secret through argv, environment variables, or workspace files.
type ScopedConsumer func(io.Reader) error

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

// Use opens a managed lease for one callback and closes/clears it
// deterministically. It is intentionally defined on ManagedBroker rather
// than Broker: refusing and metadata-only brokers never expose a secret.
func (b *ManagedBroker) Use(id string, consumer ScopedConsumer) error {
	if consumer == nil {
		return fmt.Errorf("credential scoped consumer is required")
	}
	reader, err := b.Open(id)
	if err != nil {
		return err
	}
	defer reader.Close()
	return consumer(reader)
}
