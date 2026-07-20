package credentials

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// NativeProvider reads one opaque value from the platform's local secret
// manager. It is deliberately opt-in: the default broker remains refusing.
// Ref and scopes are passed as argv metadata, never interpolated into a shell
// command, and command output is bounded and never logged.
type NativeProvider struct {
	Path    string
	Service string
	Timeout time.Duration
}

func (p NativeProvider) Fetch(parent context.Context, request Request) ([]byte, error) {
	if strings.TrimSpace(request.Ref) == "" {
		return nil, fmt.Errorf("native credential reference is required")
	}
	path, args, err := p.command(request.Ref)
	if err != nil {
		return nil, err
	}
	ctx := parent
	var cancel context.CancelFunc
	if p.Timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, p.Timeout)
		defer cancel()
	}
	output, err := exec.CommandContext(ctx, path, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("native credential provider: %w", err)
	}
	output = bytes.TrimSpace(output)
	if len(output) == 0 || len(output) > maxProviderOutput {
		return nil, fmt.Errorf("native credential provider output must be between 1 and %d bytes", maxProviderOutput)
	}
	return output, nil
}

func (p NativeProvider) command(ref string) (string, []string, error) {
	path := strings.TrimSpace(p.Path)
	if path == "" {
		switch runtime.GOOS {
		case "darwin":
			path = "security"
		case "linux":
			path = "secret-tool"
		default:
			return "", nil, fmt.Errorf("native credential provider is unsupported on %s", runtime.GOOS)
		}
	}
	service := strings.TrimSpace(p.Service)
	if service == "" {
		service = "rewind"
	}
	switch runtime.GOOS {
	case "darwin":
		return path, []string{"find-generic-password", "-s", service, "-a", ref, "-w"}, nil
	case "linux":
		return path, []string{"lookup", "service", service, "ref", ref}, nil
	default:
		if p.Path != "" {
			return path, []string{ref}, nil
		}
		return "", nil, fmt.Errorf("native credential provider is unsupported on %s", runtime.GOOS)
	}
}
