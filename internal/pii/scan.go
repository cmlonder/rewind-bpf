// Package pii provides a deterministic, content-only audit scanner. It is an
// optional review tool, not a replacement for Landlock path enforcement: scan
// results never grant access and matched values are never returned.
package pii

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const MaxFileBytes = 8 << 20

type Finding struct {
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	ValueHash   string `json:"value_hash"`
	Replacement string `json:"replacement"`
}

type rule struct {
	kind        string
	pattern     *regexp.Regexp
	replacement string
}

// RuleConfig lets operators add deterministic, redacted project-specific
// patterns without changing the built-in scanner. Pattern values are regular
// expressions and must never be used to return matched content.
type RuleConfig struct {
	Kind        string `json:"kind" yaml:"kind"`
	Pattern     string `json:"pattern" yaml:"pattern"`
	Replacement string `json:"replacement" yaml:"replacement"`
}

type Scanner struct{ rules []rule }

func NewScanner(custom []RuleConfig) (Scanner, error) {
	compiled := append([]rule(nil), rules...)
	for _, config := range custom {
		if strings.TrimSpace(config.Kind) == "" || strings.TrimSpace(config.Pattern) == "" || strings.TrimSpace(config.Replacement) == "" {
			return Scanner{}, fmt.Errorf("PII custom rule requires kind, pattern, and replacement")
		}
		pattern, err := regexp.Compile(config.Pattern)
		if err != nil {
			return Scanner{}, fmt.Errorf("PII custom rule %s: %w", config.Kind, err)
		}
		compiled = append(compiled, rule{kind: config.Kind, pattern: pattern, replacement: config.Replacement})
	}
	return Scanner{rules: compiled}, nil
}

var rules = []rule{
	{kind: "email", pattern: regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`), replacement: "[REDACTED:email]"},
	{kind: "phone", pattern: regexp.MustCompile(`(?:\+\d[\d .()\-]{7,}\d|\(\d{3}\)[\d .\-]{7,}\d|\b\d{10,11}\b)`), replacement: "[REDACTED:phone]"},
	{kind: "ssn", pattern: regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`), replacement: "[REDACTED:ssn]"},
	{kind: "credit_card", pattern: regexp.MustCompile(`\b(?:\d[ -]?){13,19}\b`), replacement: "[REDACTED:credit_card]"},
	{kind: "api_token", pattern: regexp.MustCompile(`\b(?:sk|ghp|github_pat|xox[baprs])_[A-Za-z0-9_\-]{12,}\b`), replacement: "[REDACTED:api_token]"},
}

func ScanPath(path string) ([]Finding, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("scan PII: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("scan PII: refusing symlink %s", path)
	}
	if info.IsDir() {
		return scanDir(path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scan PII: read %s: %w", path, err)
	}
	if len(data) > MaxFileBytes {
		return nil, fmt.Errorf("scan PII: %s exceeds %d-byte limit", path, MaxFileBytes)
	}
	return ScanBytes(path, data), nil
}

func ScanBytes(path string, data []byte) []Finding {
	return scanBytes(path, data, rules)
}

func (s Scanner) ScanBytes(path string, data []byte) []Finding {
	active := s.rules
	if len(active) == 0 {
		active = rules
	}
	return scanBytes(path, data, active)
}

func scanBytes(path string, data []byte, active []rule) []Finding {
	if strings.IndexByte(string(data), 0) >= 0 {
		return nil
	}
	var findings []Finding
	for lineNumber, line := range strings.Split(string(data), "\n") {
		for _, current := range active {
			for _, match := range current.pattern.FindAllStringIndex(line, -1) {
				value := line[match[0]:match[1]]
				digest := sha256.Sum256([]byte(value))
				findings = append(findings, Finding{Path: path, Kind: current.kind, Line: lineNumber + 1, Column: match[0] + 1, ValueHash: hex.EncodeToString(digest[:8]), Replacement: current.replacement})
			}
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Column < findings[j].Column
	})
	return findings
}

// ScanReader consumes at most maxBytes and avoids loading an unbounded stream
// into memory. It is intended for event payloads and remote restore previews.
func (s Scanner) ScanReader(path string, reader io.Reader, maxBytes int64) ([]Finding, error) {
	if maxBytes <= 0 || maxBytes > MaxFileBytes {
		maxBytes = MaxFileBytes
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("scan PII: stream exceeds %d-byte limit", maxBytes)
	}
	return s.ScanBytes(path, data), nil
}

func RedactBytes(data []byte) []byte {
	return redactBytes(data, rules)
}

func (s Scanner) RedactBytes(data []byte) []byte {
	active := s.rules
	if len(active) == 0 {
		active = rules
	}
	return redactBytes(data, active)
}

func redactBytes(data []byte, active []rule) []byte {
	value := string(data)
	for _, current := range active {
		value = current.pattern.ReplaceAllString(value, current.replacement)
	}
	return []byte(value)
}

func scanDir(root string) ([]Finding, error) {
	var findings []Finding
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == ".rewind") {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > MaxFileBytes {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		findings = append(findings, ScanBytes(path, data)...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan PII tree: %w", err)
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Path < findings[j].Path })
	return findings, nil
}
