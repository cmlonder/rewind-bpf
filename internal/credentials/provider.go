package credentials

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const maxProviderOutput = 64 << 10

// Provider is an external secret-manager boundary. The returned bytes remain
// inside the broker and are never part of a Lease or policy value.
type Provider interface {
	Fetch(context.Context, Request) ([]byte, error)
}

// CommandProvider adapts a local secret-manager CLI. Ref and scopes are passed
// as non-secret metadata in the child environment; the command's stdout is
// read with a hard cap and never logged.
type CommandProvider struct {
	Path    string
	Args    []string
	Timeout time.Duration
}

func (p CommandProvider) Fetch(parent context.Context, request Request) ([]byte, error) {
	if strings.TrimSpace(p.Path) == "" {
		return nil, fmt.Errorf("credential provider command is required")
	}
	if strings.TrimSpace(request.Ref) == "" {
		return nil, fmt.Errorf("credential provider reference is required")
	}
	ctx := parent
	var cancel context.CancelFunc
	if p.Timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, p.Timeout)
		defer cancel()
	}
	command := exec.CommandContext(ctx, p.Path, p.Args...)
	command.Env = append(os.Environ(),
		"REWIND_CREDENTIAL_REF="+request.Ref,
		"REWIND_CREDENTIAL_SCOPES="+strings.Join(request.Scopes, ","),
	)
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("credential provider command: %w", err)
	}
	if len(output) == 0 || len(output) > maxProviderOutput {
		return nil, fmt.Errorf("credential provider output must be between 1 and %d bytes", maxProviderOutput)
	}
	output = bytes.TrimSpace(output)
	if len(output) == 0 {
		return nil, fmt.Errorf("credential provider output is empty after trimming")
	}
	return output, nil
}

type managedLease struct {
	request Request
	secret  []byte
	expires time.Time
}

type leaseReader struct {
	data []byte
	off  int
}

func (r *leaseReader) Read(p []byte) (int, error) {
	if r == nil || len(r.data) == 0 || r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

func (r *leaseReader) Close() error {
	if r == nil {
		return nil
	}
	for i := range r.data {
		r.data[i] = 0
	}
	r.data = nil
	r.off = 0
	return nil
}

// ManagedBroker adds short-lived, one-shot handles around a Provider. Open is
// intentionally an explicit runtime-only operation; callers must decide how a
// secret is used without placing it in an agent environment or workspace.
type ManagedBroker struct {
	Provider Provider
	TTL      time.Duration
	Now      func() time.Time

	mu     sync.Mutex
	leases map[string]managedLease
}

func (b *ManagedBroker) Issue(request Request) (Lease, error) {
	if b == nil || b.Provider == nil {
		return Lease{}, fmt.Errorf("credential broker provider is unavailable")
	}
	ttl := b.TTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	now := time.Now
	if b.Now != nil {
		now = b.Now
	}
	secret, err := b.Provider.Fetch(context.Background(), request)
	if err != nil {
		return Lease{}, err
	}
	if len(secret) == 0 {
		return Lease{}, fmt.Errorf("credential provider returned an empty secret")
	}
	id, err := leaseID()
	if err != nil {
		return Lease{}, err
	}
	expires := now().Add(ttl)
	b.mu.Lock()
	if b.leases == nil {
		b.leases = make(map[string]managedLease)
	}
	b.leases[id] = managedLease{request: Request{Ref: request.Ref, Scopes: append([]string(nil), request.Scopes...)}, secret: append([]byte(nil), secret...), expires: expires}
	b.mu.Unlock()
	return Lease{ID: id, Ref: request.Ref, Scopes: append([]string(nil), request.Scopes...), ExpiresAt: expires, SecretExposed: false}, nil
}

// Open consumes a lease and returns its secret exactly once. The caller owns
// the returned reader and must close it promptly; expired or revoked leases
// are indistinguishable from unknown handles.
func (b *ManagedBroker) Open(id string) (io.ReadCloser, error) {
	if b == nil || strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("credential lease is invalid")
	}
	now := time.Now
	if b.Now != nil {
		now = b.Now
	}
	b.mu.Lock()
	lease, ok := b.leases[id]
	if ok {
		delete(b.leases, id)
	}
	b.mu.Unlock()
	if !ok || !now().Before(lease.expires) {
		return nil, fmt.Errorf("credential lease is expired or revoked")
	}
	return &leaseReader{data: lease.secret}, nil
}

func (b *ManagedBroker) Revoke(id string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	delete(b.leases, id)
	b.mu.Unlock()
}

func leaseID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate credential lease id: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}
