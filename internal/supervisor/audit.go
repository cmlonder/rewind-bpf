package supervisor

import (
	"bufio"
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

func readAudit(path string, limit int) ([]AuditEntry, error) {
	if path == "" {
		return []AuditEntry{}, nil
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return []AuditEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open supervisor audit: %w", err)
	}
	defer file.Close()
	entries := make([]AuditEntry, 0, limit)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry AuditEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
		if len(entries) > limit {
			entries = entries[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read supervisor audit: %w", err)
	}
	return entries, nil
}
