// Package pii provides a deterministic, content-only audit scanner. It is an
// optional review tool, not a replacement for Landlock path enforcement: scan
// results never grant access and matched values are never returned.
package pii

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	if strings.IndexByte(string(data), 0) >= 0 {
		return nil
	}
	var findings []Finding
	for lineNumber, line := range strings.Split(string(data), "\n") {
		for _, current := range rules {
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

func RedactBytes(data []byte) []byte {
	value := string(data)
	for _, current := range rules {
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
