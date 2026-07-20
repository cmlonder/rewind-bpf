// Package policybundle provides signed, portable policy package envelopes.
// Signing authenticates policy metadata; it does not grant a package runtime
// authority. The runtime must still validate capabilities before assignment.
package policybundle

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/rewindbpf/rewind/internal/policy"
)

type Bundle struct {
	Name        string        `json:"name"`
	Version     string        `json:"version"`
	Description string        `json:"description,omitempty"`
	Policy      policy.Policy `json:"policy"`
}

type Signed struct {
	Version   int    `json:"version"`
	KeyID     string `json:"key_id"`
	PublicKey string `json:"public_key"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

func Sign(bundle Bundle, private ed25519.PrivateKey) (Signed, error) {
	if err := validateBundle(bundle); err != nil {
		return Signed{}, err
	}
	if len(private) != ed25519.PrivateKeySize {
		return Signed{}, fmt.Errorf("policy bundle private key has invalid length")
	}
	payload, err := json.Marshal(bundle)
	if err != nil {
		return Signed{}, fmt.Errorf("encode policy bundle: %w", err)
	}
	public := private.Public().(ed25519.PublicKey)
	return Signed{Version: 1, KeyID: keyID(public), PublicKey: base64.StdEncoding.EncodeToString(public), Payload: base64.StdEncoding.EncodeToString(payload), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, payload))}, nil
}

func Verify(signed Signed, trusted ...ed25519.PublicKey) (Bundle, error) {
	if signed.Version != 1 {
		return Bundle{}, fmt.Errorf("unsupported signed policy version %d", signed.Version)
	}
	public, err := decodeKey(signed.PublicKey, ed25519.PublicKeySize)
	if err != nil {
		return Bundle{}, fmt.Errorf("decode policy public key: %w", err)
	}
	if signed.KeyID != keyID(public) {
		return Bundle{}, fmt.Errorf("policy key id mismatch")
	}
	payload, err := base64.StdEncoding.DecodeString(signed.Payload)
	if err != nil {
		return Bundle{}, fmt.Errorf("decode policy payload: %w", err)
	}
	signature, err := decodeKey(signed.Signature, ed25519.SignatureSize)
	if err != nil {
		return Bundle{}, fmt.Errorf("decode policy signature: %w", err)
	}
	if !ed25519.Verify(public, payload, signature) {
		return Bundle{}, fmt.Errorf("policy signature verification failed")
	}
	if len(trusted) > 0 {
		trustedMatch := false
		for _, candidate := range trusted {
			if ed25519.PublicKey(candidate).Equal(ed25519.PublicKey(public)) {
				trustedMatch = true
				break
			}
		}
		if !trustedMatch {
			return Bundle{}, fmt.Errorf("policy signer is not trusted")
		}
	}
	var bundle Bundle
	if err := json.Unmarshal(payload, &bundle); err != nil {
		return Bundle{}, fmt.Errorf("decode policy bundle: %w", err)
	}
	if err := validateBundle(bundle); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func validateBundle(bundle Bundle) error {
	if bundle.Name == "" || bundle.Version == "" {
		return fmt.Errorf("policy bundle name and version are required")
	}
	if err := bundle.Policy.Validate(); err != nil {
		return fmt.Errorf("policy bundle policy: %w", err)
	}
	return nil
}

func keyID(public ed25519.PublicKey) string {
	digest := sha256.Sum256(public)
	return fmt.Sprintf("%x", digest[:8])
}

func decodeKey(value string, length int) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != length {
		return nil, fmt.Errorf("expected %d bytes", length)
	}
	return decoded, nil
}
