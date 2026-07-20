// Package agent defines the launch identity contract shared by the CLI and
// control plane. Adapters record an agent kind without rewriting its command.
package agent

import (
	"fmt"
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
	Kind        Kind   `json:"kind"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

var specs = []Spec{
	{Kind: Generic, DisplayName: "Generic command", Description: "Any executable launched under the protected runtime."},
	{Kind: Codex, DisplayName: "Codex", Description: "Codex-compatible command; the operator command remains unchanged."},
	{Kind: OpenHands, DisplayName: "OpenHands", Description: "OpenHands-compatible command; the operator command remains unchanged."},
	{Kind: ClaudeCode, DisplayName: "Claude Code", Description: "Claude Code-compatible command; the operator command remains unchanged."},
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
