package supervisor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/rewindbpf/rewind/internal/history"
)

func TestHandlerHealthAndReadOnlyActionBoundary(t *testing.T) {
	server := Server{History: history.Open(filepath.Join(t.TempDir(), "history.json"))}
	for _, test := range []struct {
		path   string
		status int
		want   string
	}{{"/health", http.StatusOK, "ready"}, {"/v1/actions", http.StatusNotImplemented, "refused"}} {
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

func TestValidateUnixSocketPath(t *testing.T) {
	if err := ValidateUnixSocketPath(""); err == nil {
		t.Fatal("empty socket path should fail")
	}
	if err := ValidateUnixSocketPath(filepath.Join(t.TempDir(), "rewind.sock")); err != nil {
		t.Fatal(err)
	}
}
