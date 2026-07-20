package supervisor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AuditEntry is a redacted record of an authenticated supervisor action. It
// intentionally excludes bearer tokens, policy contents, and file contents.
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	RunID     string    `json:"run_id,omitempty"`
	State     string    `json:"state,omitempty"`
	OK        bool      `json:"ok"`
	Message   string    `json:"message,omitempty"`
	Error     string    `json:"error,omitempty"`
}

func appendAudit(path string, entry AuditEntry) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create supervisor audit directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open supervisor audit: %w", err)
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(entry); err != nil {
		return fmt.Errorf("write supervisor audit: %w", err)
	}
	return nil
}
