package policybundle

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/rewindbpf/rewind/internal/policy"
)

func TestSignAndVerify(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	want := Bundle{Name: "strict-agent", Version: "1.0.0", Policy: policy.Policy{Read: policy.ReadPolicy{Mode: policy.ModeEnforce}}}
	signed, err := Sign(want, private)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Verify(signed, public)
	if err != nil || got.Name != want.Name {
		t.Fatalf("verified=%+v err=%v", got, err)
	}
	signed.Payload += "x"
	if _, err := Verify(signed, public); err == nil {
		t.Fatal("tampered payload unexpectedly verified")
	}
}

func TestVerifyRejectsUntrustedSigner(t *testing.T) {
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	signed, err := Sign(Bundle{Name: "demo", Version: "1.0.0", Policy: policy.Policy{Read: policy.ReadPolicy{Mode: policy.ModeAudit}}}, private)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(signed, other); err == nil {
		t.Fatal("untrusted signer unexpectedly accepted")
	}
}
