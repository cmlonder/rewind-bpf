package bundle

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPublishRequiresHTTPSOutsideLoopback(t *testing.T) {
	path := writeArchiveFixture(t, []byte("gzip-placeholder"))
	_, err := Publish(context.Background(), path, "http://example.test/review", "", "", false)
	if err == nil || !strings.Contains(err.Error(), "HTTPS is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPublishPostsArchiveDigestSignatureAndBearer(t *testing.T) {
	payload := []byte("gzip-placeholder")
	path := writeArchiveFixture(t, payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/gzip" {
			t.Errorf("request = %s %s content-type=%q", r.Method, r.URL, r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Rewind-Bundle-Signature") != "signed" {
			t.Errorf("signature = %q", r.Header.Get("X-Rewind-Bundle-Signature"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if string(body) != string(payload) {
			t.Errorf("body = %q", body)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("queued"))
	}))
	defer server.Close()
	got, err := Publish(context.Background(), path, server.URL, "test-token", "signed", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "queued" {
		t.Fatalf("receipt = %q", got)
	}
}

func writeArchiveFixture(t *testing.T, data []byte) string {
	t.Helper()
	path := t.TempDir() + "/evidence.tar.gz"
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
