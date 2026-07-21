package supervisor

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/rewindbpf/rewind/internal/diff"
	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/platform"
	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/runstore"
)

// RunDetail is the read-only, browser-safe projection of a persisted run.
// Native records contain manifests and filesystem paths that must stay inside
// the supervisor; the dashboard only receives the reviewed change set and
// bounded evidence metadata.
type RunDetail struct {
	RunID        string        `json:"run_id"`
	State        string        `json:"state"`
	Backend      string        `json:"backend,omitempty"`
	Workspace    string        `json:"workspace,omitempty"`
	Command      []string      `json:"command,omitempty"`
	ExitCode     int           `json:"exit_code,omitempty"`
	Changes      []diff.Change `json:"changes,omitempty"`
	ChangeCount  int           `json:"change_count"`
	StagedBytes  int64         `json:"staged_bytes"`
	EventCount   uint64        `json:"event_count"`
	EventBytes   uint64        `json:"event_bytes"`
	EvidenceDone bool          `json:"evidence_complete"`
}

func (s Server) runDetail(w http.ResponseWriter, r *http.Request) {
	if s.RequireAuth && !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, Response{OK: false, State: "refused", Message: "bearer authentication required"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, Response{OK: false, Message: "run detail is read-only over this endpoint"})
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
		writeJSON(w, http.StatusNotFound, Response{OK: false, Message: "run not found in supervisor history"})
		return
	}
	if native, ok, detectErr := platform.NativeRecordForSupervisor(recordPath, runID); detectErr != nil {
		writeJSON(w, http.StatusConflict, Response{OK: false, State: "refused", Message: detectErr.Error()})
		return
	} else if ok {
		changes := append([]diff.Change(nil), native.Changes...)
		// A native record is finalized when the protected command exits. While
		// the shell is still running, derive a read-only live diff from the
		// disposable APFS view so the dashboard reflects `rm`/write activity
		// before the operator closes the session.
		if len(changes) == 0 && native.View != "" {
			if after, buildErr := manifest.Build(native.View); buildErr == nil {
				changes = diff.Compare(native.BaseManifest, after)
			}
		}
		// Read-policy enforcement temporarily hides sensitive paths from the
		// staged view. A missing hidden path is expected, not an agent delete.
		// Filter both new records (which carry exact hidden paths) and older
		// records by consulting their deny patterns.
		changes = filterPolicyHiddenChanges(changes, native)
		writeJSON(w, http.StatusOK, RunDetail{
			RunID: native.RunID, State: native.State, Backend: native.Backend,
			Workspace: native.Workspace, Command: append([]string(nil), native.Command...),
			ExitCode: native.ExitCode, Changes: changes, ChangeCount: len(changes),
			StagedBytes: stagedBytes(changes), EventCount: uint64(len(native.Events)),
			EvidenceDone: native.State != "running",
		})
		return
	}
	record, err := runstore.Read(recordPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, Response{OK: false, Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, RunDetail{
		RunID: record.Plan.Run.ID, State: string(record.Plan.Run.State), Backend: string(record.Plan.OverlayBackend),
		Workspace: record.Plan.Layout.Lower, ChangeCount: 0, EventCount: record.Events.Count,
		EventBytes: record.Events.Bytes, EvidenceDone: record.Events.Complete,
	})
}

func filterPolicyHiddenChanges(changes []diff.Change, native platform.NativeRecord) []diff.Change {
	if len(changes) == 0 {
		return changes
	}
	hidden := make([]string, 0, len(native.HiddenPaths))
	for _, path := range native.HiddenPaths {
		if native.View == "" {
			continue
		}
		rel, err := filepath.Rel(native.View, filepath.Clean(path))
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			hidden = append(hidden, filepath.ToSlash(rel))
		}
	}
	var readDeny func(string) bool
	if native.PolicyPath != "" {
		if value, err := policy.Load(native.PolicyPath); err == nil && value.Read.Mode == policy.ModeEnforce {
			readDeny = func(path string) bool {
				return value.Read.Explain(path).Decision == "deny" || value.Read.Explain("/"+path).Decision == "deny"
			}
		}
	}
	if len(hidden) == 0 && readDeny == nil {
		return changes
	}
	filtered := make([]diff.Change, 0, len(changes))
	for _, change := range changes {
		path := filepath.ToSlash(filepath.Clean(change.Path))
		isHidden := false
		for _, root := range hidden {
			if path == root || strings.HasPrefix(path, root+"/") {
				isHidden = true
				break
			}
		}
		if !isHidden && readDeny != nil {
			isHidden = readDeny(path)
		}
		if !isHidden {
			filtered = append(filtered, change)
		}
	}
	return filtered
}

func stagedBytes(changes []diff.Change) int64 {
	var total int64
	for _, change := range changes {
		if change.After != nil && change.After.Type == "file" {
			total += change.After.Size
		} else if change.Before != nil && change.Before.Type == "file" {
			total += change.Before.Size
		}
	}
	return total
}
