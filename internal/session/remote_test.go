package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteStoreApplyAndList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test" {
			t.Errorf("missing auth")
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("[]"))
			return
		}
		_, _ = w.Write([]byte(`{"id":"lease-1","run_id":"run-1","owner":"test"}`))
	}))
	defer server.Close()
	store := RemoteStore{Endpoint: server.URL, Bearer: "test", Attempts: 1}
	lease, err := store.Apply(context.Background(), Request{Action: "acquire", RunID: "run-1", Owner: "test"})
	if err != nil || lease.ID != "lease-1" {
		t.Fatalf("lease=%+v err=%v", lease, err)
	}
	if _, err := store.List(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteStoreRejectsExternalHTTP(t *testing.T) {
	_, err := (RemoteStore{Endpoint: "http://example.com"}).List(context.Background())
	if err == nil {
		t.Fatal("expected HTTPS refusal")
	}
}
