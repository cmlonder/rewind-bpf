package supervisor

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rewindbpf/rewind/internal/controlplane"
	"github.com/rewindbpf/rewind/internal/history"
	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/policybundle"
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

func TestHTTPBridgeRequiresAuthForReadEndpointsAndSetsCORS(t *testing.T) {
	server := Server{AuthToken: "secret", RequireAuth: true, CORSOrigin: "http://127.0.0.1:4173"}
	for _, path := range []string{"/v1/capabilities", "/v1/history"} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s status=%d", path, recorder.Code)
		}
		if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:4173" {
			t.Fatalf("%s CORS origin=%q", path, got)
		}
	}
	preflight := httptest.NewRecorder()
	server.Handler().ServeHTTP(preflight, httptest.NewRequest(http.MethodOptions, "/v1/actions", nil))
	if preflight.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d", preflight.Code)
	}
}

func TestConfigEndpointsRequireAuthAndPersistValidatedChanges(t *testing.T) {
	server := Server{AuthToken: "secret", RequireAuth: true, Config: controlplane.Open(filepath.Join(t.TempDir(), "config.json")), AuditPath: filepath.Join(t.TempDir(), "audit.jsonl"), AuditMu: &sync.Mutex{}}
	unauthorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/policies", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("policies status=%d", unauthorized.Code)
	}
	policyPayload := `{"name":"strict-agent","version":"1.0.0","description":"review first","policy":{"read":{"mode":"audit"},"write":{"mode":"rollback","scope":"workspace"},"network":{"mode":"off"}}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/policies", strings.NewReader(policyPayload))
	request.Header.Set("Authorization", "Bearer secret")
	created := httptest.NewRecorder()
	server.Handler().ServeHTTP(created, request)
	if created.Code != http.StatusCreated {
		t.Fatalf("create policy status=%d body=%s", created.Code, created.Body.String())
	}
	workspace := httptest.NewRequest(http.MethodPost, "/v1/workspaces", strings.NewReader(`{"name":"payments-api","path":"/workspaces/payments-api","policy":"strict-agent@1.0.0"}`))
	workspace.Header.Set("Authorization", "Bearer secret")
	assigned := httptest.NewRecorder()
	server.Handler().ServeHTTP(assigned, workspace)
	if assigned.Code != http.StatusCreated {
		t.Fatalf("assign workspace status=%d body=%s", assigned.Code, assigned.Body.String())
	}
	read := httptest.NewRequest(http.MethodGet, "/v1/workspaces", nil)
	read.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, read)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "payments-api") {
		t.Fatalf("workspace response status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRejectedConfigMutationIsAudited(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	server := Server{AuthToken: "secret", Config: controlplane.Open(filepath.Join(t.TempDir(), "config.json")), AuditPath: auditPath, AuditMu: &sync.Mutex{}}
	request := httptest.NewRequest(http.MethodPost, "/v1/policies", strings.NewReader(`{"name":"bad name","version":"1.0.0","policy":{"read":{"mode":"audit"}}}`))
	request.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	audit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(audit), `"action":"policy_create"`) || !strings.Contains(string(audit), `"ok":false`) {
		t.Fatalf("audit=%s", audit)
	}
}

func TestSignedPolicyBundleImportRequiresValidSignature(t *testing.T) {
	dir := t.TempDir()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := policybundle.Sign(policybundle.Bundle{Name: "signed-agent", Version: "1.0.0", Policy: policy.Policy{Read: policy.ReadPolicy{Mode: policy.ModeAudit}}}, private)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(signed)
	if err != nil {
		t.Fatal(err)
	}
	auditPath := filepath.Join(dir, "audit.jsonl")
	server := Server{AuthToken: "secret", Config: controlplane.Open(filepath.Join(dir, "config.json")), AuditPath: auditPath, AuditMu: &sync.Mutex{}}
	request := httptest.NewRequest(http.MethodPost, "/v1/policy-bundles", strings.NewReader(string(encoded)))
	request.Header.Set("Authorization", "Bearer secret")
	created := httptest.NewRecorder()
	server.Handler().ServeHTTP(created, request)
	if created.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", created.Code, created.Body.String())
	}
	signed.Signature = "tampered"
	encoded, _ = json.Marshal(signed)
	request = httptest.NewRequest(http.MethodPost, "/v1/policy-bundles", strings.NewReader(string(encoded)))
	request.Header.Set("Authorization", "Bearer secret")
	refused := httptest.NewRecorder()
	server.Handler().ServeHTTP(refused, request)
	if refused.Code != http.StatusConflict || !strings.Contains(refused.Body.String(), "signature") {
		t.Fatalf("status=%d body=%s", refused.Code, refused.Body.String())
	}
	audit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(audit), `"action":"policy_bundle_import"`) || !strings.Contains(string(audit), `"ok":false`) {
		t.Fatalf("audit=%s", audit)
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

func TestAuthenticatedAuditReadIsBounded(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "actions.jsonl")
	for i := 0; i < 3; i++ {
		if err := appendAudit(auditPath, AuditEntry{Action: "status", RunID: string(rune('a' + i))}); err != nil {
			t.Fatal(err)
		}
	}
	server := Server{AuthToken: "secret", AuditPath: auditPath}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/audit?limit=2", nil)
	request.Header.Set("Authorization", "Bearer secret")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var entries []AuditEntry
	if err := json.Unmarshal(recorder.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].RunID != "b" || entries[1].RunID != "c" {
		t.Fatalf("entries=%+v", entries)
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

func TestValidateHTTPListenAddressRequiresLoopback(t *testing.T) {
	for _, address := range []string{"", ":8787", "0.0.0.0:8787", "192.0.2.10:8787", "127.0.0.1"} {
		if err := ValidateHTTPListenAddress(address); err == nil {
			t.Fatalf("address %q should be rejected", address)
		}
	}
	for _, address := range []string{"127.0.0.1:8787", "localhost:8787", "[::1]:8787"} {
		if err := ValidateHTTPListenAddress(address); err != nil {
			t.Fatalf("address %q: %v", address, err)
		}
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
