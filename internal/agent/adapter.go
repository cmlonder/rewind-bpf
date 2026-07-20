// Package agent defines the launch identity contract shared by the CLI and
// control plane. Adapters record an agent kind without rewriting its command.
package agent

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Kind string

const (
	Generic    Kind = "generic"
	Codex      Kind = "codex"
	OpenHands  Kind = "openhands"
	ClaudeCode Kind = "claude-code"
)

type Spec struct {
	Kind         Kind     `json:"kind"`
	DisplayName  string   `json:"display_name"`
	Description  string   `json:"description"`
	Executables  []string `json:"executables,omitempty"`
	HookProtocol string   `json:"hook_protocol"`
}

type Launch struct {
	Spec        Spec
	Command     []string
	Environment []string
}

var specs = []Spec{
	{Kind: Generic, DisplayName: "Generic command", Description: "Any executable launched under the protected runtime.", HookProtocol: "rewind/v1"},
	{Kind: Codex, DisplayName: "Codex", Description: "Codex-compatible command; the operator command remains unchanged.", Executables: []string{"codex", "codex-cli"}, HookProtocol: "rewind/v1"},
	{Kind: OpenHands, DisplayName: "OpenHands", Description: "OpenHands-compatible command; the operator command remains unchanged.", Executables: []string{"openhands"}, HookProtocol: "rewind/v1"},
	{Kind: ClaudeCode, DisplayName: "Claude Code", Description: "Claude Code-compatible command; the operator command remains unchanged.", Executables: []string{"claude", "claude-code"}, HookProtocol: "rewind/v1"},
}

func List() []Spec { result := make([]Spec, len(specs)); copy(result, specs); return result }

func Resolve(value string) (Spec, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = string(Generic)
	}
	for _, spec := range specs {
		if string(spec.Kind) == value {
			return spec, nil
		}
	}
	return Spec{}, fmt.Errorf("unsupported agent adapter %q (want generic, codex, openhands, or claude-code)", value)
}

func ValidateCommand(spec Spec, command []string) error {
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return fmt.Errorf("agent adapter %s: command cannot be empty", spec.Kind)
	}
	return nil
}

// Prepare is the stable lifecycle seam for SDK-specific adapters. Current
// adapters are intentionally command-preserving: they add only an auditable
// identity marker and leave command arguments under operator control.
func Prepare(spec Spec, command []string) (Launch, error) {
	if err := ValidateCommand(spec, command); err != nil {
		return Launch{}, err
	}
	copyCommand := append([]string(nil), command...)
	return Launch{Spec: spec, Command: copyCommand, Environment: []string{
		"REWIND_AGENT_ADAPTER=" + string(spec.Kind),
		"REWIND_AGENT_HOOK_PROTOCOL=" + spec.HookProtocol,
		"REWIND_AGENT_LIFECYCLE=prepare,start,exit",
	}}, nil
}

func (s Spec) Recognizes(command string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(command)))
	for _, executable := range s.Executables {
		if base == executable {
			return true
		}
	}
	return s.Kind == Generic
}
