package bundle

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecryptRoundTripAndTamperRefusal(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "bundle.tar.gz")
	encrypted := filepath.Join(dir, "bundle.enc")
	decrypted := filepath.Join(dir, "roundtrip.tar.gz")
	key := bytes.Repeat([]byte{0x42}, 32)
	original := []byte("signed evidence bytes")
	if err := os.WriteFile(input, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := EncryptFile(input, encrypted, key); err != nil {
		t.Fatal(err)
	}
	if err := DecryptFile(encrypted, decrypted, key); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(decrypted)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("round trip=%q err=%v", got, err)
	}
	data, err := os.ReadFile(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-2] ^= 1
	if _, err := DecryptBytes(data, key); err == nil {
		t.Fatal("tampered envelope decrypted")
	}
}

func TestEncryptRejectsWrongKeyLength(t *testing.T) {
	if _, err := EncryptBytes([]byte("payload"), []byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
}
