package release

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestSignVerifyAndTrustPin(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("artifact checksums\n")
	signed, err := Sign(payload, private)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(payload, signed); err != nil {
		t.Fatalf("embedded key verification failed: %v", err)
	}
	if err := Verify(payload, signed, public); err != nil {
		t.Fatalf("trusted key verification failed: %v", err)
	}
	other, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(payload, signed, other); err == nil {
		t.Fatal("expected untrusted signer refusal")
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := Sign([]byte("original"), private)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify([]byte("tampered"), signed); err == nil {
		t.Fatal("expected payload tampering refusal")
	}
	signed.PayloadSHA256 = "not-a-digest"
	if err := Verify([]byte("original"), signed); err == nil {
		t.Fatal("expected malformed checksum refusal")
	}
}

func TestSignRejectsInvalidPrivateKey(t *testing.T) {
	if _, err := Sign(nil, make(ed25519.PrivateKey, ed25519.PrivateKeySize-1)); err == nil {
		t.Fatal("expected invalid key refusal")
	}
}
