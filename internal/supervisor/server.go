package supervisor

import (
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rewindbpf/rewind/internal/controlplane"
	"github.com/rewindbpf/rewind/internal/credentials"
	"github.com/rewindbpf/rewind/internal/history"
	"github.com/rewindbpf/rewind/internal/lifecycle"
	"github.com/rewindbpf/rewind/internal/platform"
	"github.com/rewindbpf/rewind/internal/policybundle"
	"github.com/rewindbpf/rewind/internal/registry"
	"github.com/rewindbpf/rewind/internal/runstore"
	"github.com/rewindbpf/rewind/internal/session"
)

type ActionFunc func(Request) (Response, error)

type Server struct {
	History   history.Store
	AuthToken string
	Actions   ActionFunc
	AuditPath string
	AuditMu   *sync.Mutex
	// RequireAuth protects read endpoints when the handler is exposed over
	// loopback HTTP. Unix-socket mode keeps the historical read-only behavior
	// because the socket itself is mode 0600.
	RequireAuth       bool
	CORSOrigin        string
	Config            *controlplane.Store
	TrustedPolicyKeys []ed25519.PublicKey
	CredentialBroker  credentials.Broker
	Sessions          session.LeaseStore
	SessionPath       string
	ActionChallenges  *actionChallengeStore
	Registry          *registry.Client
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	challenges := s.ActionChallenges
	if challenges == nil {
		challenges = &actionChallengeStore{}
	}
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, Response{OK: true, State: "ready", Message: "local supervisor; authenticated actions"})
	})
	mux.HandleFunc("/v1/capabilities", func(w http.ResponseWriter, r *http.Request) {
		if s.RequireAuth && !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
			return
		}
		writeJSON(w, http.StatusOK, platform.Probe())
	})
	mux.HandleFunc("/v1/history", func(w http.ResponseWriter, r *http.Request) {
		if s.RequireAuth && !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
			return
		}
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, Response{Message: "history is read-only over this endpoint"})
			return
		}
		entries, err := s.History.List()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, Response{Message: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, entries)
	})
	mux.HandleFunc("/v1/history/prune", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "history pruning requires POST"})
			return
		}
		var request HistoryPruneRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&request); err != nil || request.Keep < 0 {
			response := Response{OK: false, State: "refused", Message: "keep must be a non-negative integer"}
			s.recordAudit(Request{Action: "history_prune"}, response, nil)
			writeJSON(w, http.StatusBadRequest, response)
			return
		}
		removed, err := s.History.PruneKeepLatest(request.Keep)
		if err != nil {
			response := Response{OK: false, State: "refused", Message: err.Error()}
			s.recordAudit(Request{Action: "history_prune"}, response, err)
			writeJSON(w, http.StatusConflict, response)
			return
		}
		response := Response{OK: true, State: "pruned", Message: fmt.Sprintf("removed=%d keep=%d", removed, request.Keep)}
		s.recordAudit(Request{Action: "history_prune"}, response, nil)
		writeJSON(w, http.StatusOK, response)
	})
	mux.HandleFunc("/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
			return
		}
		if s.SessionPath == "" {
			writeJSON(w, http.StatusNotImplemented, Response{OK: false, State: "refused", Message: "session leases are unavailable"})
			return
		}
		if r.Method == http.MethodGet {
			leases, err := s.Sessions.List()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, Response{OK: false, State: "refused", Message: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, leases)
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "sessions require GET or POST"})
			return
		}
		var request SessionRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, Response{OK: false, State: "refused", Message: "invalid session request"})
			return
		}
		lease, err := s.Sessions.Apply(session.Request{Action: request.Action, RunID: request.RunID, Owner: request.Owner, TTL: request.TTL}, time.Now())
		if err != nil {
			response := Response{OK: false, State: "refused", Message: err.Error()}
			s.recordAudit(Request{Action: "session_" + request.Action, RunID: request.RunID}, response, err)
			writeJSON(w, http.StatusConflict, response)
			return
		}
		response := Response{OK: true, State: request.Action, Message: lease.ID}
		s.recordAudit(Request{Action: "session_" + request.Action, RunID: request.RunID}, response, nil)
		writeJSON(w, http.StatusOK, lease)
	})
	mux.HandleFunc("/v1/policies", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if s.RequireAuth && !s.authorized(r) {
				writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
				return
			}
			snapshot, err := s.configSnapshot()
			if err != nil {
				writeJSON(w, http.StatusNotImplemented, Response{OK: false, State: "refused", Message: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, snapshot.Policies)
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "policies require GET or POST"})
			return
		}
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
			return
		}
		var value controlplane.PolicyPackage
		if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
			writeJSON(w, http.StatusBadRequest, Response{OK: false, State: "refused", Message: "invalid policy package"})
			return
		}
		if err := s.createPolicy(value); err != nil {
			s.recordAudit(Request{Action: "policy_create", Policy: value.Name + "@" + value.Version}, Response{OK: false, State: "refused", Message: err.Error()}, err)
			writeJSON(w, http.StatusConflict, Response{OK: false, State: "refused", Message: err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, Response{OK: true, State: "created", Message: value.Name + "@" + value.Version})
	})
	mux.HandleFunc("/v1/workspaces", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if s.RequireAuth && !s.authorized(r) {
				writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
				return
			}
			snapshot, err := s.configSnapshot()
			if err != nil {
				writeJSON(w, http.StatusNotImplemented, Response{OK: false, State: "refused", Message: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, snapshot.Workspaces)
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "workspaces require GET or POST"})
			return
		}
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
			return
		}
		var value controlplane.Workspace
		if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
			writeJSON(w, http.StatusBadRequest, Response{OK: false, State: "refused", Message: "invalid workspace assignment"})
			return
		}
		if err := s.assignWorkspace(value); err != nil {
			s.recordAudit(Request{Action: "workspace_assign", Workspace: value.Name, Policy: value.Policy}, Response{OK: false, State: "refused", Message: err.Error()}, err)
			writeJSON(w, http.StatusConflict, Response{OK: false, State: "refused", Message: err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, Response{OK: true, State: "assigned", Message: value.Name})
	})
	mux.HandleFunc("/v1/policy-bundles", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if !s.authorized(r) {
				writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
				return
			}
			snapshot, err := s.configSnapshot()
			if err != nil {
				writeJSON(w, http.StatusNotImplemented, Response{OK: false, State: "refused", Message: err.Error()})
				return
			}
			bundles := make([]policybundle.Signed, 0, len(snapshot.Policies))
			for _, value := range snapshot.Policies {
				if value.SignedBundle != nil {
					bundles = append(bundles, *value.SignedBundle)
				}
			}
			writeJSON(w, http.StatusOK, bundles)
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "policy bundles require POST"})
			return
		}
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
			return
		}
		var signed policybundle.Signed
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&signed); err != nil {
			response := Response{OK: false, State: "refused", Message: "invalid signed policy bundle"}
			s.recordAudit(Request{Action: "policy_bundle_import"}, response, err)
			writeJSON(w, http.StatusBadRequest, response)
			return
		}
		if err := s.importSignedPolicy(signed); err != nil {
			response := Response{OK: false, State: "refused", Message: err.Error()}
			s.recordAudit(Request{Action: "policy_bundle_import"}, response, err)
			writeJSON(w, http.StatusConflict, response)
			return
		}
		bundle, _ := policybundle.Verify(signed)
		response := Response{OK: true, State: "created", Message: bundle.Name + "@" + bundle.Version}
		s.recordAudit(Request{Action: "policy_bundle_import", Policy: bundle.Name + "@" + bundle.Version}, response, nil)
		writeJSON(w, http.StatusCreated, response)
	})
	mux.HandleFunc("/v1/credential-leases", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
			return
		}
		if r.Method == http.MethodGet {
			state := "unavailable"
			if s.CredentialBroker != nil {
				state = "available"
			}
			writeJSON(w, http.StatusOK, map[string]any{"available": s.CredentialBroker != nil, "state": state, "secret_exposed": false})
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "credential leases require POST"})
			return
		}
		if s.CredentialBroker == nil {
			response := Response{OK: false, State: "refused", Message: "credential broker is unavailable"}
			s.recordAudit(Request{Action: "credential_lease"}, response, nil)
			writeJSON(w, http.StatusNotImplemented, response)
			return
		}
		var request CredentialLeaseRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&request); err != nil || strings.TrimSpace(request.Ref) == "" {
			response := Response{OK: false, State: "refused", Message: "invalid credential lease request"}
			s.recordAudit(Request{Action: "credential_lease"}, response, nil)
			writeJSON(w, http.StatusBadRequest, response)
			return
		}
		lease, err := s.CredentialBroker.Issue(credentials.Request{Ref: request.Ref, Scopes: append([]string(nil), request.Scopes...)})
		if err != nil {
			response := Response{OK: false, State: "refused", Message: err.Error()}
			s.recordAudit(Request{Action: "credential_lease"}, response, err)
			writeJSON(w, http.StatusConflict, response)
			return
		}
		// Lease intentionally contains only opaque metadata. The broker keeps
		// any secret bytes in its runtime-only store.
		response := Response{OK: true, State: "issued", Message: lease.ID}
		s.recordAudit(Request{Action: "credential_lease"}, response, nil)
		writeJSON(w, http.StatusCreated, lease)
	})
	mux.HandleFunc("/v1/audit", s.auditLog)
	mux.HandleFunc("/v1/run", s.runDetail)
	mux.HandleFunc("/v1/events", s.events)
	mux.HandleFunc("/v1/action-challenges", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "action challenges require POST"})
			return
		}
		var request ActionChallengeRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, Response{OK: false, State: "refused", Message: "invalid action challenge request"})
			return
		}
		if err := Validate(Request{Action: request.Action, RunID: request.RunID}); err != nil || (request.Action != "rollback" && request.Action != "recover" && request.Action != "commit") {
			writeJSON(w, http.StatusBadRequest, Response{OK: false, State: "refused", Message: "challenge requires rollback, recover, or commit"})
			return
		}
		challenge, err := challenges.issue(request.Action, request.RunID, time.Now())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, Response{OK: false, State: "refused", Message: err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, challenge)
	})
	mux.HandleFunc("/v1/registry/policies", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
			return
		}
		if s.Registry == nil {
			writeJSON(w, http.StatusNotImplemented, Response{OK: false, State: "unavailable", Message: "trusted registry client is not configured"})
			return
		}
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "registry listing requires GET"})
			return
		}
		entries, err := s.Registry.List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusBadGateway, Response{OK: false, State: "refused", Message: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, entries)
	})
	mux.HandleFunc("/v1/registry/policies/fetch", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
			return
		}
		if s.Registry == nil {
			writeJSON(w, http.StatusNotImplemented, Response{OK: false, State: "unavailable", Message: "trusted registry client is not configured"})
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "registry fetch requires POST"})
			return
		}
		var request RegistryFetchRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&request); err != nil || strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.Version) == "" {
			writeJSON(w, http.StatusBadRequest, Response{OK: false, State: "refused", Message: "registry name and version are required"})
			return
		}
		bundle, err := s.Registry.Fetch(r.Context(), request.Name, request.Version)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, Response{OK: false, State: "refused", Message: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, bundle)
	})
	mux.HandleFunc("/v1/registry/policies/revoke", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
			return
		}
		if s.Registry == nil {
			writeJSON(w, http.StatusNotImplemented, Response{OK: false, State: "unavailable", Message: "trusted registry client is not configured"})
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "registry revoke requires POST"})
			return
		}
		var request RegistryFetchRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&request); err != nil || strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.Version) == "" {
			writeJSON(w, http.StatusBadRequest, Response{OK: false, State: "refused", Message: "registry name and version are required"})
			return
		}
		if err := s.Registry.Revoke(r.Context(), request.Name, request.Version); err != nil {
			writeJSON(w, http.StatusBadGateway, Response{OK: false, State: "refused", Message: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, Response{OK: true, State: "revoked", Message: request.Name + "@" + request.Version})
	})
	mux.HandleFunc("/v1/actions", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "actions require POST"})
			return
		}
		var request Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, Response{OK: false, Message: "invalid action request"})
			return
		}
		if err := Validate(request); err != nil {
			writeJSON(w, http.StatusBadRequest, Response{OK: false, Message: err.Error()})
			return
		}
		if request.Action != "status" {
			if err := challenges.consume(request.ActionToken, request.Action, request.RunID, time.Now()); err != nil {
				response := Response{OK: false, State: "refused", Message: err.Error()}
				s.recordAudit(Request{Action: request.Action, RunID: request.RunID}, response, err)
				writeJSON(w, http.StatusConflict, response)
				return
			}
		}
		if s.Actions == nil {
			response := Response{OK: false, State: "refused", Message: "runtime action handler is not connected"}
			s.recordAudit(request, response, nil)
			writeJSON(w, http.StatusNotImplemented, response)
			return
		}
		response, err := s.Actions(request)
		if err != nil {
			refused := Response{OK: false, State: "refused", Message: err.Error()}
			s.recordAudit(request, refused, err)
			writeJSON(w, http.StatusConflict, refused)
			return
		}
		s.recordAudit(request, response, nil)
		writeJSON(w, http.StatusOK, response)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, Response{OK: false, Message: "not found"})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(s.CORSOrigin) != "" {
			w.Header().Set("Access-Control-Allow-Origin", s.CORSOrigin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func (s Server) configSnapshot() (controlplane.Snapshot, error) {
	if s.Config == nil || !s.Config.Enabled() {
		return controlplane.Snapshot{}, fmt.Errorf("control-plane config store is disabled")
	}
	return s.Config.Snapshot()
}

func (s Server) createPolicy(value controlplane.PolicyPackage) error {
	if s.Config == nil || !s.Config.Enabled() {
		return fmt.Errorf("control-plane config store is disabled")
	}
	if err := s.Config.CreatePolicy(value); err != nil {
		return err
	}
	s.recordAudit(Request{Action: "policy_create", Policy: value.Name + "@" + value.Version}, Response{OK: true, State: "created", Message: value.Name + "@" + value.Version}, nil)
	return nil
}

func (s Server) assignWorkspace(value controlplane.Workspace) error {
	if s.Config == nil || !s.Config.Enabled() {
		return fmt.Errorf("control-plane config store is disabled")
	}
	if err := s.Config.AssignWorkspace(value); err != nil {
		return err
	}
	s.recordAudit(Request{Action: "workspace_assign", Workspace: value.Name, Policy: value.Policy}, Response{OK: true, State: "assigned", Message: value.Name}, nil)
	return nil
}

func (s Server) importSignedPolicy(signed policybundle.Signed) error {
	if s.Config == nil || !s.Config.Enabled() {
		return fmt.Errorf("control-plane config store is disabled")
	}
	if _, err := policybundle.Verify(signed, s.TrustedPolicyKeys...); err != nil {
		return err
	}
	return s.Config.CreateSignedPolicy(signed)
}

func (s Server) recordAudit(request Request, response Response, actionErr error) {
	if strings.TrimSpace(s.AuditPath) == "" {
		return
	}
	entry := AuditEntry{
		Timestamp: time.Now().UTC(),
		Action:    request.Action,
		RunID:     request.RunID,
		State:     response.State,
		OK:        response.OK,
		Message:   response.Message,
	}
	if actionErr != nil {
		entry.Error = actionErr.Error()
	}
	mu := s.AuditMu
	if mu == nil {
		mu = &sync.Mutex{}
	}
	mu.Lock()
	defer mu.Unlock()
	_ = appendAudit(s.AuditPath, entry)
}

func (s Server) auditLog(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "audit is read-only over this endpoint"})
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			writeJSON(w, http.StatusBadRequest, Response{OK: false, Message: "audit limit must be a positive integer"})
			return
		}
		if value > 500 {
			value = 500
		}
		limit = value
	}
	entries, err := readAudit(s.AuditPath, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, Response{OK: false, Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s Server) authorized(r *http.Request) bool {
	if s.AuthToken == "" {
		return false
	}
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(value, "Bearer ") {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
	return len(token) == len(s.AuthToken) && subtle.ConstantTimeCompare([]byte(token), []byte(s.AuthToken)) == 1
}

func (s Server) events(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "events require GET"})
		return
	}
	runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, Response{OK: false, Message: "run_id is required"})
		return
	}
	entries, err := s.History.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, Response{OK: false, Message: err.Error()})
		return
	}
	var recordPath string
	for _, entry := range entries {
		if entry.RunID == runID {
			recordPath = entry.RecordPath
			break
		}
	}
	if recordPath == "" {
		writeJSON(w, http.StatusNotFound, Response{OK: false, Message: "run not found in supervisor history; start it with the same --history path"})
		return
	}
	if native, ok, detectErr := platform.NativeRecordForSupervisor(recordPath, runID); detectErr != nil {
		writeJSON(w, http.StatusConflict, Response{OK: false, State: "refused", Message: detectErr.Error()})
		return
	} else if ok {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher, flushOK := w.(http.Flusher)
		if !flushOK {
			writeJSON(w, http.StatusInternalServerError, Response{OK: false, Message: "streaming is unavailable"})
			return
		}
		follow, err := parseFollow(r.URL.Query().Get("follow"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, Response{OK: false, Message: err.Error()})
			return
		}
		if !follow {
			streamNativeEventSnapshot(w, flusher, native)
			return
		}
		streamNativeEventFollow(r.Context(), w, flusher, recordPath, native)
		return
	}
	record, err := runstore.Read(recordPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, Response{OK: false, Message: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, Response{OK: false, Message: "streaming is unavailable"})
		return
	}
	follow, err := parseFollow(r.URL.Query().Get("follow"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, Response{OK: false, Message: err.Error()})
		return
	}
	if !follow {
		streamEventSnapshot(w, flusher, record)
		return
	}
	streamEventFollow(r.Context(), w, flusher, runID, recordPath, record)
}

func parseFollow(value string) (bool, error) {
	if strings.TrimSpace(value) == "" {
		return false, nil
	}
	follow, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("follow must be true or false")
	}
	return follow, nil
}

func streamEventSnapshot(w http.ResponseWriter, flusher http.Flusher, record runstore.Record) {
	for _, path := range runstore.EventLogPaths(record) {
		file, openErr := os.Open(path)
		if openErr != nil {
			continue
		}
		decoder := json.NewDecoder(file)
		for {
			var event json.RawMessage
			if decodeErr := decoder.Decode(&event); decodeErr != nil {
				break
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		}
		_ = file.Close()
	}
}

func streamNativeEventSnapshot(w http.ResponseWriter, flusher http.Flusher, record platform.NativeRecord) {
	position := int64(0)
	streamNativeEventDelta(w, flusher, record.EventsPath, &position)
}

func streamNativeEventFollow(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, recordPath string, record platform.NativeRecord) {
	position := int64(0)
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		current := record
		if refreshed, err := platform.ReadNativeRecord(recordPath); err == nil {
			current = refreshed
		}
		wrote := streamNativeEventDelta(w, flusher, current.EventsPath, &position)
		if nativeRecordTerminal(current.State) {
			return
		}
		if wrote {
			if !deadline.Stop() {
				select {
				case <-deadline.C:
				default:
				}
			}
			deadline.Reset(30 * time.Second)
		}
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			return
		case <-ticker.C:
		}
	}
}

func streamNativeEventDelta(w http.ResponseWriter, flusher http.Flusher, path string, position *int64) bool {
	if path == "" {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	if _, err := file.Seek(*position, io.SeekStart); err != nil {
		return false
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return false
	}
	lastNewline := strings.LastIndexByte(string(data), '\n')
	if lastNewline < 0 {
		return false
	}
	complete := string(data[:lastNewline+1])
	wrote := false
	for _, raw := range strings.Split(complete, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || !json.Valid([]byte(line)) {
			continue
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
		wrote = true
	}
	*position += int64(lastNewline + 1)
	return wrote
}

func nativeRecordTerminal(state string) bool {
	switch state {
	case "succeeded", "failed", "discarded", "committed", "rolled_back":
		return true
	default:
		return false
	}
}

// streamEventFollow replays any existing journal bytes and then tails the
// active journal until the run reaches a terminal state or the client closes
// the request. A bounded idle timeout prevents a forgotten browser tab from
// holding a supervisor connection forever; reconnecting resumes from the
// latest persisted snapshot.
func streamEventFollow(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, runID, recordPath string, record runstore.Record) {
	positions := make(map[string]int64)
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		current := record
		if refreshed, err := runstore.Read(recordPath); err == nil {
			current = refreshed
		}
		wrote := streamEventDelta(w, flusher, current, positions)
		if current.Plan.Run.ID == runID && isTerminal(current.Plan.Run.State) {
			return
		}
		if wrote {
			if !deadline.Stop() {
				select {
				case <-deadline.C:
				default:
				}
			}
			deadline.Reset(30 * time.Second)
		}
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			return
		case <-ticker.C:
		}
	}
}

func streamEventDelta(w http.ResponseWriter, flusher http.Flusher, record runstore.Record, positions map[string]int64) bool {
	wrote := false
	for _, path := range runstore.EventLogPaths(record) {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		position := positions[path]
		if _, err := file.Seek(position, io.SeekStart); err != nil {
			_ = file.Close()
			continue
		}
		data, readErr := io.ReadAll(file)
		if readErr != nil {
			_ = file.Close()
			continue
		}
		lastNewline := strings.LastIndexByte(string(data), '\n')
		if lastNewline >= 0 {
			complete := string(data[:lastNewline+1])
			for _, raw := range strings.Split(complete, "\n") {
				line := strings.TrimSpace(raw)
				if line == "" || !json.Valid([]byte(line)) {
					continue
				}
				_, _ = fmt.Fprintf(w, "data: %s\n\n", line)
				flusher.Flush()
				wrote = true
			}
			positions[path] = position + int64(lastNewline+1)
		}
		_ = file.Close()
	}
	return wrote
}

func isTerminal(state lifecycle.State) bool {
	switch state {
	case lifecycle.Succeeded, lifecycle.Failed, lifecycle.Committed, lifecycle.RolledBack:
		return true
	default:
		return false
	}
}

// ValidateUnixSocketPath refuses broad or ambiguous paths before a future
// daemon creates a socket. The caller remains responsible for filesystem
// permissions and authentication policy.
func ValidateUnixSocketPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("supervisor socket path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve supervisor socket: %w", err)
	}
	if abs == string(filepath.Separator) || filepath.Base(abs) == "." || filepath.Base(abs) == ".." {
		return fmt.Errorf("unsafe supervisor socket path")
	}
	return nil
}

// ValidateHTTPListenAddress limits the browser bridge to loopback. The
// supervisor carries an authenticated action endpoint and must never be
// accidentally exposed on a LAN or public interface.
func ValidateHTTPListenAddress(address string) error {
	value := strings.TrimSpace(address)
	if value == "" {
		return fmt.Errorf("supervisor HTTP listen address is required")
	}
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("parse supervisor HTTP listen address: %w", err)
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("supervisor HTTP listener must bind to loopback")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.Handler().ServeHTTP(w, r) }
