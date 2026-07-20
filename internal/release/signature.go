// Package release provides detached signatures for release metadata.
//
// The signature envelope carries the public key so an artifact can be
// self-verified. Callers that need publisher authenticity must also provide a
// pinned trusted key to Verify; an embedded key alone proves integrity, not
// that the signer is the expected organization.
package release

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const SignatureVersion = 1

// Signature is a detached Ed25519 signature over the exact payload bytes.
// Payload bytes are intentionally not embedded: the signed file remains the
// canonical artifact and can be checked with ordinary SHA-256 tooling first.
type Signature struct {
	Version       int    `json:"version"`
	KeyID         string `json:"key_id"`
	PublicKey     string `json:"public_key"`
	PayloadSHA256 string `json:"payload_sha256"`
	Signature     string `json:"signature"`
}

func Sign(payload []byte, private ed25519.PrivateKey) (Signature, error) {
	if len(private) != ed25519.PrivateKeySize {
		return Signature{}, fmt.Errorf("release private key has invalid length")
	}
	public, ok := private.Public().(ed25519.PublicKey)
	if !ok || len(public) != ed25519.PublicKeySize {
		return Signature{}, fmt.Errorf("release private key has invalid public key")
	}
	digest := sha256.Sum256(payload)
	return Signature{
		Version:       SignatureVersion,
		KeyID:         KeyID(public),
		PublicKey:     base64.StdEncoding.EncodeToString(public),
		PayloadSHA256: hex.EncodeToString(digest[:]),
		Signature:     base64.StdEncoding.EncodeToString(ed25519.Sign(private, payload)),
	}, nil
}

// Verify authenticates a signature envelope and its payload. When trusted is
// non-empty, the embedded key must match one of those pinned keys.
func Verify(payload []byte, signed Signature, trusted ...ed25519.PublicKey) error {
	if signed.Version != SignatureVersion {
		return fmt.Errorf("unsupported release signature version %d", signed.Version)
	}
	public, err := decode(signed.PublicKey, ed25519.PublicKeySize)
	if err != nil {
		return fmt.Errorf("decode release public key: %w", err)
	}
	if signed.KeyID != KeyID(ed25519.PublicKey(public)) {
		return fmt.Errorf("release key id mismatch")
	}
	digest := sha256.Sum256(payload)
	expectedDigest := hex.EncodeToString(digest[:])
	if len(signed.PayloadSHA256) != sha256.Size*2 || subtle.ConstantTimeCompare([]byte(signed.PayloadSHA256), []byte(expectedDigest)) != 1 {
		return fmt.Errorf("release payload checksum mismatch")
	}
	signature, err := decode(signed.Signature, ed25519.SignatureSize)
	if err != nil {
		return fmt.Errorf("decode release signature: %w", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(public), payload, signature) {
		return fmt.Errorf("release signature verification failed")
	}
	if len(trusted) > 0 {
		trustedMatch := false
		for _, candidate := range trusted {
			if len(candidate) == ed25519.PublicKeySize && subtle.ConstantTimeCompare(candidate, public) == 1 {
				trustedMatch = true
				break
			}
		}
		if !trustedMatch {
			return fmt.Errorf("release signer is not trusted")
		}
	}
	return nil
}

// VerifyAny accepts a rotating trust set. An empty set preserves the embedded
// signer check; a non-empty set succeeds when any configured key matches.
func VerifyAny(payload []byte, signed Signature, trusted []ed25519.PublicKey) error {
	if len(trusted) == 0 {
		return Verify(payload, signed)
	}
	var last error
	for _, key := range trusted {
		if err := Verify(payload, signed, key); err == nil {
			return nil
		} else {
			last = err
		}
	}
	if last == nil {
		last = fmt.Errorf("release signer is not trusted")
	}
	return last
}

func KeyID(public ed25519.PublicKey) string {
	digest := sha256.Sum256(public)
	return hex.EncodeToString(digest[:8])
}

func decode(value string, length int) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != length {
		return nil, fmt.Errorf("expected %d bytes", length)
	}
	return decoded, nil
}
