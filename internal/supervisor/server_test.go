package supervisor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewindbpf/rewind/internal/history"
)

func TestHandlerHealthAndReadOnlyActionBoundary(t *testing.T) {
	server := Server{History: history.Open(filepath.Join(t.TempDir(), "history.json")), AuthToken: "secret"}
	for _, test := range []struct {
		path   string
		status int
		want   string
	}{{"/health", http.StatusOK, "ready"}, {"/v1/actions", http.StatusUnauthorized, "refused"}} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
		if recorder.Code != test.status {
			t.Fatalf("%s status=%d", test.path, recorder.Code)
		}
		var body Response
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.State != test.want {
			t.Fatalf("%s body=%+v", test.path, body)
		}
	}
}

func TestAuthenticatedActionWithoutRuntimeHandlerRefuses(t *testing.T) {
	server := Server{AuthToken: "secret"}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/actions", strings.NewReader(`{"action":"status"}`))
	request.Header.Set("Authorization", "Bearer secret")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestValidateUnixSocketPath(t *testing.T) {
	if err := ValidateUnixSocketPath(""); err == nil {
		t.Fatal("empty socket path should fail")
	}
	if err := ValidateUnixSocketPath(filepath.Join(t.TempDir(), "rewind.sock")); err != nil {
		t.Fatal(err)
	}
}
