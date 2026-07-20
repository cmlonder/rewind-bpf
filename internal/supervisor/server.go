package supervisor

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rewindbpf/rewind/internal/history"
	"github.com/rewindbpf/rewind/internal/lifecycle"
	"github.com/rewindbpf/rewind/internal/platform"
	"github.com/rewindbpf/rewind/internal/runstore"
)

type ActionFunc func(Request) (Response, error)

type Server struct {
	History   history.Store
	AuthToken string
	Actions   ActionFunc
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, Response{OK: true, State: "ready", Message: "local supervisor; authenticated actions"})
	})
	mux.HandleFunc("/v1/capabilities", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, platform.Probe()) })
	mux.HandleFunc("/v1/history", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/v1/events", s.events)
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
		if s.Actions == nil {
			writeJSON(w, http.StatusNotImplemented, Response{OK: false, State: "refused", Message: "runtime action handler is not connected"})
			return
		}
		response, err := s.Actions(request)
		if err != nil {
			writeJSON(w, http.StatusConflict, Response{OK: false, State: "refused", Message: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, Response{OK: false, Message: "not found"})
	})
	return mux
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
		writeJSON(w, http.StatusNotFound, Response{OK: false, Message: "run not found in history"})
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.Handler().ServeHTTP(w, r) }
