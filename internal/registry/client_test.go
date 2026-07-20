package registry

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/policybundle"
)

func TestFetchVerifiesSignedEnvelope(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	signed, err := policybundle.Sign(policybundle.Bundle{Name: "safe", Version: "1", Policy: policy.Policy{Read: policy.ReadPolicy{Mode: policy.ModeOff}, Write: policy.WritePolicy{Mode: "rollback", Scope: "workspace"}, Network: policy.NetworkPolicy{Mode: policy.ModeAudit}}}, private)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/registry/policies/safe/1" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(signed)
	}))
	defer server.Close()
	client := Client{Endpoint: server.URL, TrustedKeys: []ed25519.PublicKey{public}}
	bundle, err := client.Fetch(context.Background(), "safe", "1")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Name != "safe" {
		t.Fatal(bundle)
	}
}

func TestRejectsNonHTTPSPublicEndpoint(t *testing.T) {
	if _, err := (Client{Endpoint: "http://registry.example"}).Fetch(context.Background(), "safe", "1"); err == nil {
		t.Fatal("expected HTTPS refusal")
	}
}

func TestFileRegistryPublishAndFetch(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	signed, err := policybundle.Sign(policybundle.Bundle{Name: "safe", Version: "1", Policy: policy.Policy{Read: policy.ReadPolicy{Mode: policy.ModeOff}, Write: policy.WritePolicy{Mode: "rollback", Scope: "workspace"}, Network: policy.NetworkPolicy{Mode: policy.ModeAudit}}}, private)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((Server{Store: FileStore{Root: t.TempDir()}, Bearer: "token"}).Handler())
	defer server.Close()
	client := Client{Endpoint: server.URL, Bearer: "token", TrustedKeys: []ed25519.PublicKey{public}}
	if err := client.Publish(context.Background(), signed); err != nil {
		t.Fatal(err)
	}
	bundle, err := client.Fetch(context.Background(), "safe", "1")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Version != "1" {
		t.Fatal(bundle)
	}
}
