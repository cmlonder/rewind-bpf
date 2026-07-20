package bundle

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	encryptedVersion  = 1
	maxEncryptedBytes = 128 << 20
)

// EncryptedEnvelope is an authenticated-at-rest wrapper for an evidence
// archive. It contains no workspace data outside the AEAD ciphertext.
type EncryptedEnvelope struct {
	Version       int    `json:"version"`
	Algorithm     string `json:"algorithm"`
	Nonce         string `json:"nonce"`
	Ciphertext    string `json:"ciphertext"`
	PlaintextSHA  string `json:"plaintext_sha256"`
	CiphertextSHA string `json:"ciphertext_sha256"`
}

func EncryptFile(inputPath, outputPath string, key []byte) (EncryptedEnvelope, error) {
	plaintext, err := readBounded(inputPath, maxEncryptedBytes)
	if err != nil {
		return EncryptedEnvelope{}, fmt.Errorf("encrypt bundle: %w", err)
	}
	envelope, err := EncryptBytes(plaintext, key)
	if err != nil {
		return EncryptedEnvelope{}, err
	}
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return EncryptedEnvelope{}, fmt.Errorf("encode encrypted bundle: %w", err)
	}
	if err := atomicWrite(outputPath, append(data, '\n'), 0o600); err != nil {
		return EncryptedEnvelope{}, fmt.Errorf("write encrypted bundle: %w", err)
	}
	return envelope, nil
}

func DecryptFile(inputPath, outputPath string, key []byte) error {
	data, err := readBounded(inputPath, maxEncryptedBytes+1)
	if err != nil {
		return fmt.Errorf("decrypt bundle: %w", err)
	}
	plaintext, err := DecryptBytes(data, key)
	if err != nil {
		return err
	}
	if err := atomicWrite(outputPath, plaintext, 0o600); err != nil {
		return fmt.Errorf("write decrypted bundle: %w", err)
	}
	return nil
}

func EncryptBytes(plaintext, key []byte) (EncryptedEnvelope, error) {
	if len(key) != 32 {
		return EncryptedEnvelope{}, fmt.Errorf("encrypt bundle: key must be exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return EncryptedEnvelope{}, fmt.Errorf("encrypt bundle: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedEnvelope{}, fmt.Errorf("encrypt bundle: gcm: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return EncryptedEnvelope{}, fmt.Errorf("encrypt bundle: nonce: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, nil)
	plainDigest := sha256.Sum256(plaintext)
	cipherDigest := sha256.Sum256(ciphertext)
	return EncryptedEnvelope{
		Version:       encryptedVersion,
		Algorithm:     "AES-256-GCM",
		Nonce:         base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext:    base64.RawStdEncoding.EncodeToString(ciphertext),
		PlaintextSHA:  fmt.Sprintf("%x", plainDigest[:]),
		CiphertextSHA: fmt.Sprintf("%x", cipherDigest[:]),
	}, nil
}

func DecryptBytes(data, key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("decrypt bundle: key must be exactly 32 bytes")
	}
	var envelope EncryptedEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decrypt bundle: decode envelope: %w", err)
	}
	if envelope.Version != encryptedVersion || envelope.Algorithm != "AES-256-GCM" {
		return nil, fmt.Errorf("decrypt bundle: unsupported envelope")
	}
	nonce, err := base64.RawStdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decrypt bundle: decode nonce: %w", err)
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt bundle: decode ciphertext: %w", err)
	}
	digest := sha256.Sum256(ciphertext)
	if fmt.Sprintf("%x", digest[:]) != envelope.CiphertextSHA {
		return nil, fmt.Errorf("decrypt bundle: ciphertext digest mismatch")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("decrypt bundle: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("decrypt bundle: gcm: %w", err)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt bundle: authentication failed")
	}
	plainDigest := sha256.Sum256(plaintext)
	if fmt.Sprintf("%x", plainDigest[:]) != envelope.PlaintextSHA {
		return nil, fmt.Errorf("decrypt bundle: plaintext digest mismatch")
	}
	return plaintext, nil
}

func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || int64(len(data)) > limit {
		return nil, fmt.Errorf("file must be between 1 and %d bytes", limit)
	}
	return data, nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
