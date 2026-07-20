package credentials

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestCommandProviderManagedLeaseIsOneShot(t *testing.T) {
	provider := CommandProvider{Path: "/bin/sh", Args: []string{"-c", "test \"$REWIND_CREDENTIAL_REF\" = github; printf 'token-value'"}}
	clock := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	broker := &ManagedBroker{Provider: provider, TTL: time.Minute, Now: func() time.Time { return clock }}
	lease, err := broker.Issue(Request{Ref: "github", Scopes: []string{"read:org"}})
	if err != nil {
		t.Fatal(err)
	}
	if lease.SecretExposed || lease.ID == "" || !lease.ExpiresAt.Equal(clock.Add(time.Minute)) {
		t.Fatalf("lease = %+v", lease)
	}
	reader, err := broker.Open(lease.ID)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || string(secret) != "token-value" {
		t.Fatalf("secret = %q err=%v", secret, err)
	}
	if _, err := broker.Open(lease.ID); err == nil {
		t.Fatal("lease was reusable")
	}
}

func TestManagedBrokerExpiresAndRevokes(t *testing.T) {
	clock := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	broker := &ManagedBroker{Provider: CommandProvider{Path: "/bin/sh", Args: []string{"-c", "printf token"}}, TTL: time.Second, Now: func() time.Time { return clock }}
	lease, err := broker.Issue(Request{Ref: "service"})
	if err != nil {
		t.Fatal(err)
	}
	broker.Revoke(lease.ID)
	if _, err := broker.Open(lease.ID); err == nil {
		t.Fatal("revoked lease opened")
	}
	lease, err = broker.Issue(Request{Ref: "service"})
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(2 * time.Second)
	if _, err := broker.Open(lease.ID); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("unexpected expiry result: %v", err)
	}
}

func TestCommandProviderContextIsHonored(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (CommandProvider{Path: "/bin/sh", Args: []string{"-c", "printf token"}}).Fetch(ctx, Request{Ref: "x"})
	if err == nil {
		t.Fatal("expected canceled provider")
	}
}
