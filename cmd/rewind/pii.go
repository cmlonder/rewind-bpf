package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rewindbpf/rewind/internal/pii"
)

func handlePII(args []string) {
	if len(args) < 1 || args[0] != "scan" {
		fatal("usage: rewind pii scan --path PATH [--output PATH --redact-output PATH]")
	}
	flags := flag.NewFlagSet("pii scan", flag.ContinueOnError)
	path := flags.String("path", "", "file or workspace directory to scan")
	output := flags.String("output", "", "optional JSON findings output")
	redactOutput := flags.String("redact-output", "", "optional redacted copy output (single file only)")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || strings.TrimSpace(*path) == "" {
		fatal("usage: rewind pii scan --path PATH [--output PATH --redact-output PATH]")
	}
	findings, err := pii.ScanPath(filepath.Clean(*path))
	if err != nil {
		fatal(err.Error())
	}
	data, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		fatal(fmt.Sprintf("encode PII findings: %v", err))
	}
	if strings.TrimSpace(*output) != "" {
		if err := os.WriteFile(filepath.Clean(*output), append(data, '\n'), 0o600); err != nil {
			fatal(fmt.Sprintf("write PII findings: %v", err))
		}
	} else {
		fmt.Println(string(data))
	}
	if strings.TrimSpace(*redactOutput) != "" {
		info, err := os.Stat(*path)
		if err != nil || info.IsDir() {
			fatal("--redact-output requires a single input file")
		}
		contents, err := os.ReadFile(*path)
		if err != nil {
			fatal(fmt.Sprintf("read PII input: %v", err))
		}
		if err := os.WriteFile(filepath.Clean(*redactOutput), pii.RedactBytes(contents), 0o600); err != nil {
			fatal(fmt.Sprintf("write redacted output: %v", err))
		}
	}
	fmt.Fprintf(os.Stderr, "PII scan complete: findings=%d\n", len(findings))
}
