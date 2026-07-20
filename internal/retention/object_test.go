package retention

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
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
