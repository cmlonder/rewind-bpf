package retention

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObjectClientRetriesAndPinsDigest(t *testing.T) {
	var attempts int
	stored := []byte("evidence")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		if r.Method == http.MethodPut {
			stored, _ = io.ReadAll(r.Body)
		}
		digest := sha256.Sum256(stored)
		w.Header().Set("X-Rewind-Object-SHA256", hex.EncodeToString(digest[:]))
		_, _ = w.Write(stored)
	}))
	defer server.Close()
	client := Client{Endpoint: server.URL, Attempts: 3}
	if err := client.Put(context.Background(), "runs/demo", []byte("evidence")); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(context.Background(), "runs/demo"); err != nil {
		t.Fatal(err)
	}
	if attempts < 3 {
		t.Fatalf("attempts=%d, want retry for PUT and GET", attempts)
	}
}

func TestObjectClientRejectsExternalHTTP(t *testing.T) {
	client := Client{Endpoint: "http://example.test"}
	if err := client.Put(context.Background(), "x", []byte(strings.Repeat("a", 1))); err == nil {
		t.Fatal("expected HTTPS refusal")
	}
}

func TestObjectClientRestoresAtomicallyWithExpectedDigest(t *testing.T) {
	data := []byte("signed evidence archive")
	digest := sha256.Sum256(data)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Rewind-Object-SHA256", hex.EncodeToString(digest[:]))
		_, _ = w.Write(data)
	}))
	defer server.Close()
	directory := t.TempDir()
	output := filepath.Join(directory, "restore.tar.gz")
	client := Client{Endpoint: server.URL}
	if err := client.GetFile(context.Background(), "runs/demo", output, hex.EncodeToString(digest[:])); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil || string(got) != string(data) {
		t.Fatalf("restored=%q err=%v", got, err)
	}
	if err := client.GetFile(context.Background(), "runs/demo", output, strings.Repeat("0", 64)); err == nil {
		t.Fatal("expected digest refusal")
	}
	if got, _ := os.ReadFile(output); string(got) != string(data) {
		t.Fatalf("digest failure replaced existing output: %q", got)
	}
}
