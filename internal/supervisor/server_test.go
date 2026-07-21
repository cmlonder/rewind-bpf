package supervisor

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rewindbpf/rewind/internal/controlplane"
	"github.com/rewindbpf/rewind/internal/credentials"
	"github.com/rewindbpf/rewind/internal/history"
	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/platform"
	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/policybundle"
	"github.com/rewindbpf/rewind/internal/registry"
	"github.com/rewindbpf/rewind/internal/runstore"
	"github.com/rewindbpf/rewind/internal/session"
)

type testCredentialBroker struct{}

func (testCredentialBroker) Issue(request credentials.Request) (credentials.Lease, error) {
	return credentials.Lease{ID: "lease-test", Ref: request.Ref, Scopes: request.Scopes, SecretExposed: false}, nil
}

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

func TestCredentialLeaseEndpointRequiresAuthAndNeverReturnsSecret(t *testing.T) {
	server := Server{AuthToken: "secret", RequireAuth: true, CredentialBroker: testCredentialBroker{}}
	unauthorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/v1/credential-leases", strings.NewReader(`{"ref":"github"}`)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorized.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/credential-leases", strings.NewReader(`{"ref":"github","scopes":["read:org"]}`))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), "lease-test") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"secret":"`) || !strings.Contains(response.Body.String(), `"secret_exposed":false`) {
		t.Fatalf("credential response leaked or omitted safety marker: %s", response.Body.String())
	}
	statusRequest := httptest.NewRequest(http.MethodGet, "/v1/credential-leases", nil)
	statusRequest.Header.Set("Authorization", "Bearer secret")
	statusResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK || !strings.Contains(statusResponse.Body.String(), `"available":true`) || !strings.Contains(statusResponse.Body.String(), `"secret_exposed":false`) {
		t.Fatalf("credential status=%d body=%s", statusResponse.Code, statusResponse.Body.String())
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

func TestHistoryPruneEndpointRequiresAuthAndKeepsNewestEntries(t *testing.T) {
	dir := t.TempDir()
	store := history.Open(filepath.Join(dir, "history.json"))
	for _, id := range []string{"old", "new"} {
		if err := store.Upsert(history.Entry{RunID: id, State: "succeeded", UpdatedAt: time.Now().Add(map[string]time.Duration{"old": -time.Hour, "new": time.Hour}[id])}); err != nil {
			t.Fatal(err)
		}
	}
	server := Server{AuthToken: "secret", History: store}
	request := httptest.NewRequest(http.MethodPost, "/v1/history/prune", strings.NewReader(`{"keep":1}`))
	request.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "removed=1") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	entries, err := store.List()
	if err != nil || len(entries) != 1 || entries[0].RunID != "new" {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
}

func TestRunDetailEndpointReturnsNativeFilesystemChanges(t *testing.T) {
	dir := t.TempDir()
	recordPath := filepath.Join(dir, "run.json")
	historyPath := filepath.Join(dir, "history.json")
	runID := "run_native_detail"
	view := filepath.Join(dir, "view")
	if err := os.MkdirAll(filepath.Join(view, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(view, "src", "marker.txt"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base, err := manifest.Build(view)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(view, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(view, "generated.txt"), []byte("created-by-agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record := platform.NativeRecord{
		RunID: runID, Platform: "darwin", Backend: "apfs-seatbelt", Workspace: filepath.Join(dir, "workspace"),
		View: view, BaseManifest: base, State: "succeeded",
	}
	if err := platform.WriteNativeRecord(recordPath, record); err != nil {
		t.Fatal(err)
	}
	if err := history.Open(historyPath).Upsert(history.Entry{RunID: runID, Backend: record.Backend, State: record.State, Workspace: record.Workspace, RecordPath: recordPath}); err != nil {
		t.Fatal(err)
	}
	server := Server{History: history.Open(historyPath), AuthToken: "secret", RequireAuth: true}
	request := httptest.NewRequest(http.MethodGet, "/v1/run?run_id="+runID, nil)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"change_count":3`) || !strings.Contains(body, `"path":"generated.txt"`) || !strings.Contains(body, `"staged_bytes":26`) {
		t.Fatalf("native detail omitted changes: %s", body)
	}
}

func TestSessionEndpointSupportsReconnectAndTakeover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	server := Server{AuthToken: "secret", SessionPath: path, Sessions: session.Open(path)}
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"action":"acquire","run_id":"run-1","owner":"browser-a","ttl_seconds":60}`))
	request.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"owner":"browser-a"`) {
		t.Fatalf("acquire status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	takeover := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"action":"takeover","run_id":"run-1","owner":"browser-b","ttl_seconds":60}`))
	takeover.Header.Set("Authorization", "Bearer secret")
	taken := httptest.NewRecorder()
	server.Handler().ServeHTTP(taken, takeover)
	if taken.Code != http.StatusOK || !strings.Contains(taken.Body.String(), `"owner":"browser-b"`) {
		t.Fatalf("takeover status=%d body=%s", taken.Code, taken.Body.String())
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
	listRequest := httptest.NewRequest(http.MethodGet, "/v1/policy-bundles", nil)
	listRequest.Header.Set("Authorization", "Bearer secret")
	listed := httptest.NewRecorder()
	server.Handler().ServeHTTP(listed, listRequest)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), signed.KeyID) {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
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

func TestSignedPolicyBundleImportHonorsTrustedSignerAllowList(t *testing.T) {
	dir := t.TempDir()
	trusted, trustedPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, untrustedPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := policybundle.Sign(policybundle.Bundle{Name: "untrusted-agent", Version: "1.0.0", Policy: policy.Policy{Read: policy.ReadPolicy{Mode: policy.ModeAudit}}}, untrustedPrivate)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(signed)
	server := Server{AuthToken: "secret", Config: controlplane.Open(filepath.Join(dir, "config.json")), TrustedPolicyKeys: []ed25519.PublicKey{trusted}, AuditPath: filepath.Join(dir, "audit.jsonl"), AuditMu: &sync.Mutex{}}
	request := httptest.NewRequest(http.MethodPost, "/v1/policy-bundles", strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer secret")
	refused := httptest.NewRecorder()
	server.Handler().ServeHTTP(refused, request)
	if refused.Code != http.StatusConflict || !strings.Contains(refused.Body.String(), "not trusted") {
		t.Fatalf("status=%d body=%s", refused.Code, refused.Body.String())
	}
	trustedBundle, err := policybundle.Sign(policybundle.Bundle{Name: "trusted-agent", Version: "1.0.0", Policy: policy.Policy{Read: policy.ReadPolicy{Mode: policy.ModeAudit}}}, trustedPrivate)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(trustedBundle)
	request = httptest.NewRequest(http.MethodPost, "/v1/policy-bundles", strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer secret")
	created := httptest.NewRecorder()
	server.Handler().ServeHTTP(created, request)
	if created.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", created.Code, created.Body.String())
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

func TestMutatingActionRequiresOneTimeServerChallenge(t *testing.T) {
	server := Server{AuthToken: "secret", Actions: func(request Request) (Response, error) {
		return Response{OK: true, State: request.Action}, nil
	}}
	handler := server.Handler()
	challengeRequest := httptest.NewRequest(http.MethodPost, "/v1/action-challenges", strings.NewReader(`{"action":"rollback","run_id":"run-1"}`))
	challengeRequest.Header.Set("Authorization", "Bearer secret")
	challengeResponse := httptest.NewRecorder()
	handler.ServeHTTP(challengeResponse, challengeRequest)
	if challengeResponse.Code != http.StatusCreated {
		t.Fatalf("challenge status=%d body=%s", challengeResponse.Code, challengeResponse.Body.String())
	}
	var challenge ActionChallengeResponse
	if err := json.Unmarshal(challengeResponse.Body.Bytes(), &challenge); err != nil || challenge.Token == "" {
		t.Fatalf("challenge=%+v err=%v", challenge, err)
	}

	request := func(token string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"action":"rollback","run_id":"run-1","action_token":%q}`, token)
		req := httptest.NewRequest(http.MethodPost, "/v1/actions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	if response := request(challenge.Token); response.Code != http.StatusOK {
		t.Fatalf("first action status=%d body=%s", response.Code, response.Body.String())
	}
	if response := request(challenge.Token); response.Code != http.StatusConflict {
		t.Fatalf("replay status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestConnectedRegistryIsBearerBoundAndVerifiedBeforeUIResponse(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := policybundle.Sign(policybundle.Bundle{Name: "strict-agent", Version: "1.0.0", Policy: policy.Policy{Read: policy.ReadPolicy{Mode: policy.ModeAudit}}}, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/registry/policies":
			_ = json.NewEncoder(w).Encode([]registry.Entry{{Name: "strict-agent", Version: "1.0.0"}})
		case "/v1/registry/policies/strict-agent/1.0.0":
			_ = json.NewEncoder(w).Encode(signed)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	server := Server{AuthToken: "secret", Registry: &registry.Client{Endpoint: remote.URL, HTTP: remote.Client(), TrustedKeys: []ed25519.PublicKey{publicKey}}}
	handler := server.Handler()
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/registry/policies", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized registry status=%d", unauthorized.Code)
	}
	authorized := httptest.NewRecorder()
	list := httptest.NewRequest(http.MethodGet, "/v1/registry/policies", nil)
	list.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(authorized, list)
	if authorized.Code != http.StatusOK || !strings.Contains(authorized.Body.String(), "strict-agent") {
		t.Fatalf("registry list status=%d body=%s", authorized.Code, authorized.Body.String())
	}
	fetch := httptest.NewRecorder()
	fetchRequest := httptest.NewRequest(http.MethodPost, "/v1/registry/policies/fetch", strings.NewReader(`{"name":"strict-agent","version":"1.0.0"}`))
	fetchRequest.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(fetch, fetchRequest)
	if fetch.Code != http.StatusOK || !strings.Contains(fetch.Body.String(), `"name":"strict-agent"`) {
		t.Fatalf("registry fetch status=%d body=%s", fetch.Code, fetch.Body.String())
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
