package supervisor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/rewindbpf/rewind/internal/history"
	"github.com/rewindbpf/rewind/internal/platform"
)

type Server struct{ History history.Store }

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, Response{OK: true, State: "ready", Message: "read-only local supervisor"})
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
	mux.HandleFunc("/v1/actions", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotImplemented, Response{OK: false, State: "refused", Message: "action transport requires authenticated supervisor integration"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, Response{OK: false, Message: "not found"})
	})
	return mux
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
