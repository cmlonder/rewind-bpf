package supervisor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rewindbpf/rewind/internal/history"
	"github.com/rewindbpf/rewind/internal/runstore"
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

func TestAuthenticatedActionHandlerReceivesValidatedRequest(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "actions.jsonl")
	server := Server{
		AuthToken: "secret",
		AuditPath: auditPath,
		AuditMu:   &sync.Mutex{},
		Actions: func(request Request) (Response, error) {
			if request.Action != "status" || request.RunID != "run_1" {
				t.Fatalf("request = %+v", request)
			}
			return Response{OK: true, State: "succeeded"}, nil
		},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/actions", strings.NewReader(`{"action":"status","run_id":"run_1"}`))
	request.Header.Set("Authorization", "Bearer secret")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	audit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(audit), `"action":"status"`) || !strings.Contains(string(audit), `"run_id":"run_1"`) {
		t.Fatalf("audit=%s", audit)
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

func TestFollowParsingAndEventDelta(t *testing.T) {
	if follow, err := parseFollow(""); err != nil || follow {
		t.Fatalf("empty follow = %v, %v", follow, err)
	}
	if follow, err := parseFollow("true"); err != nil || !follow {
		t.Fatalf("true follow = %v, %v", follow, err)
	}
	if _, err := parseFollow("sometimes"); err == nil {
		t.Fatal("invalid follow value should fail")
	}
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"operation":"write"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	positions := map[string]int64{}
	if !streamEventDelta(response, response, runstore.Record{EventsPath: path}, positions) {
		t.Fatal("event delta did not emit")
	}
	if !strings.Contains(response.Body.String(), `data: {"operation":"write"}`) {
		t.Fatalf("body=%q", response.Body.String())
	}
	if positions[path] == 0 {
		t.Fatal("event position was not advanced")
	}
	if err := os.WriteFile(path, []byte(`{"operation":"write"}`+"\n"+`{"operation":"partial"`), 0o600); err != nil {
		t.Fatal(err)
	}
	positions = map[string]int64{}
	first := httptest.NewRecorder()
	if !streamEventDelta(first, first, runstore.Record{EventsPath: path}, positions) {
		t.Fatal("complete prefix did not emit")
	}
	if strings.Contains(first.Body.String(), "partial") {
		t.Fatalf("partial event was emitted: %q", first.Body.String())
	}
	if err := os.WriteFile(path, []byte(`{"operation":"write"}`+"\n"+`{"operation":"partial"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second := httptest.NewRecorder()
	if !streamEventDelta(second, second, runstore.Record{EventsPath: path}, positions) {
		t.Fatal("completed suffix did not emit")
	}
	if !strings.Contains(second.Body.String(), "partial") {
		t.Fatalf("suffix body=%q", second.Body.String())
	}
}
