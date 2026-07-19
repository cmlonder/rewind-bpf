// Package policylearn turns observed read events into a review-only policy
// suggestion. It deliberately refuses to learn secrets, virtual filesystems,
// or broad parent directories so the output cannot silently become an unsafe
// allowlist.
package policylearn

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultMaxPaths = 128

type Candidate struct {
	Path       string
	Count      int
	Operations []string
}

type Report struct {
	Candidates []Candidate
	Skipped    map[string]int
	ReadEvents int
}

type observedEvent struct {
	Operation string `json:"operation"`
	Path      string `json:"path"`
}

type suggestedPolicy struct {
	Read struct {
		Mode  string   `yaml:"mode"`
		Allow []string `yaml:"allow,omitempty"`
	} `yaml:"read"`
	Write struct {
		Mode  string `yaml:"mode"`
		Scope string `yaml:"scope"`
	} `yaml:"write"`
	Network struct {
		Mode string `yaml:"mode"`
	} `yaml:"network"`
}

// Learn reads JSONL telemetry and returns a deterministic, bounded report.
// Only openat/read observations are candidates for a read allowlist.
func Learn(r io.Reader, maxPaths int) (Report, error) {
	if r == nil {
		return Report{}, fmt.Errorf("learn policy: nil event stream")
	}
	if maxPaths <= 0 {
		maxPaths = DefaultMaxPaths
	}
	counts := make(map[string]*Candidate)
	skipped := make(map[string]int)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	line := 0
	readEvents := 0
	for scanner.Scan() {
		line++
		var value observedEvent
		if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
			return Report{}, fmt.Errorf("learn policy: decode line %d: %w", line, err)
		}
		if value.Operation != "openat" && value.Operation != "read" {
			continue
		}
		readEvents++
		candidate, reason := normalize(value.Path)
		if reason != "" {
			skipped[reason]++
			continue
		}
		entry := counts[candidate]
		if entry == nil {
			entry = &Candidate{Path: candidate}
			counts[candidate] = entry
		}
		entry.Count++
		if !contains(entry.Operations, value.Operation) {
			entry.Operations = append(entry.Operations, value.Operation)
			sort.Strings(entry.Operations)
		}
	}
	if err := scanner.Err(); err != nil {
		return Report{}, fmt.Errorf("learn policy: read events: %w", err)
	}
	all := make([]Candidate, 0, len(counts))
	for _, value := range counts {
		all = append(all, *value)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].Path < all[j].Path
	})
	if len(all) > maxPaths {
		skipped["path_limit"] += len(all) - maxPaths
		all = all[:maxPaths]
	}
	return Report{Candidates: all, Skipped: skipped, ReadEvents: readEvents}, nil
}

// Render produces YAML that defaults to audit mode. It is a suggestion for a
// human to review; the command never edits an existing policy or enables
// enforcement implicitly.
func Render(report Report) ([]byte, error) {
	value := suggestedPolicy{}
	value.Read.Mode = "audit"
	for _, candidate := range report.Candidates {
		value.Read.Allow = append(value.Read.Allow, candidate.Path)
	}
	value.Write.Mode = "rollback"
	value.Write.Scope = "workspace"
	value.Network.Mode = "audit"
	body, err := yaml.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("render learned policy: %w", err)
	}
	var out bytes.Buffer
	out.WriteString("# RewindBPF policy learn suggestion. Review every path before use.\n")
	out.WriteString("# This file defaults to audit; switch read.mode to enforce intentionally.\n")
	out.Write(body)
	if len(report.Skipped) > 0 {
		out.WriteString("# Skipped observations: ")
		keys := make([]string, 0, len(report.Skipped))
		for key := range report.Skipped {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if i > 0 {
				out.WriteString(", ")
			}
			fmt.Fprintf(&out, "%s=%d", key, report.Skipped[key])
		}
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func normalize(raw string) (string, string) {
	value := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if value == "" {
		return "", "empty_path"
	}
	value = path.Clean(value)
	if value == "." || value == "/" {
		return "", "broad_path"
	}
	trimmed := strings.TrimPrefix(value, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return "", "broad_path"
	}
	lower := strings.ToLower(value)
	for _, prefix := range []string{"/proc", "/sys", "/dev", "/run"} {
		if lower == prefix || strings.HasPrefix(lower, prefix+"/") {
			return "", "virtual_path"
		}
	}
	for _, part := range parts {
		if secretLike(part) {
			return "", "secret_like"
		}
	}
	return value, ""
}

func secretLike(value string) bool {
	lower := strings.ToLower(value)
	if lower == ".ssh" || lower == "credentials" || lower == "credential" || lower == "secrets" || lower == "secret" {
		return true
	}
	for _, suffix := range []string{".env", ".pem", ".key", ".p12", ".pfx", ".crt", ".secret", ".token"} {
		if lower == suffix || strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return strings.HasPrefix(lower, "id_rsa") || strings.HasPrefix(lower, "id_ed25519")
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

// WriteSuggestion writes a new suggestion without replacing an existing file.
// Use "-" for stdout.
func WriteSuggestion(path string, data []byte) error {
	if path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("learn policy: output already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("learn policy: check output: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("learn policy: write output: %w", err)
	}
	return nil
}
