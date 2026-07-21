//go:build darwin

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SeatbeltCommand returns a process command wrapped by macOS's Seatbelt
// launcher. It is a process/read boundary only; APFS rollback and
// EndpointSecurity telemetry must be supplied by the native helper before a
// macOS protected run can be advertised as ready.
func SeatbeltCommand(workspace string, command string, args ...string) (*exec.Cmd, func() error, error) {
	profile, err := SeatbeltProfileWithRoots(workspace, nil)
	if err != nil {
		return nil, nil, err
	}
	profilePath, err := writeDisposableProfile(profile)
	if err != nil {
		return nil, nil, fmt.Errorf("write Seatbelt profile: %w", err)
	}
	cleanup := func() error { return os.Remove(profilePath) }
	return exec.Command("sandbox-exec", append([]string{"-f", profilePath, command}, args...)...), cleanup, nil
}

// SeatbeltCommandOptions keeps the native launch boundary explicit.  The
// caller may set the staged workspace as cwd and pass a bounded set of
// runtime roots; no host write root is ever inferred from PATH.
type SeatbeltCommandOptions struct {
	Workspace    string
	Command      string
	Args         []string
	WorkingDir   string
	Environment  []string
	RuntimeRoots []string
	DenyPaths    []string
}

// SeatbeltCommandWithOptions is the production-shaped launcher used by the
// future APFS transaction adapter.  It is safe to call on a disposable
// workspace today, but it does not claim EndpointSecurity telemetry or
// rollback by itself.
func SeatbeltCommandWithOptions(options SeatbeltCommandOptions) (*exec.Cmd, func() error, error) {
	if strings.TrimSpace(options.Command) == "" {
		return nil, nil, fmt.Errorf("Seatbelt command is required")
	}
	profile, err := SeatbeltProfileWithRootsAndDenies(options.Workspace, options.RuntimeRoots, options.DenyPaths)
	if err != nil {
		return nil, nil, err
	}
	profilePath, err := writeDisposableProfile(profile)
	if err != nil {
		return nil, nil, fmt.Errorf("write Seatbelt profile: %w", err)
	}
	cleanup := func() error { return os.Remove(profilePath) }
	command := exec.Command("sandbox-exec", append([]string{"-f", profilePath, options.Command}, options.Args...)...)
	if options.WorkingDir != "" {
		workingDir, err := filepath.Abs(options.WorkingDir)
		if err != nil {
			_ = cleanup()
			return nil, nil, fmt.Errorf("resolve Seatbelt working directory: %w", err)
		}
		command.Dir = workingDir
	}
	if options.Environment != nil {
		command.Env = append([]string(nil), options.Environment...)
	}
	return command, cleanup, nil
}

// SeatbeltProfileWithRoots allows system libraries and an explicitly bounded
// command runtime to be read while keeping writes confined to workspace.
// Sensitive paths are staged out of the view by the native runner before
// launch; Seatbelt remains the process/read/write boundary on this host.
func SeatbeltProfileWithRoots(workspace string, roots []string) (string, error) {
	return SeatbeltProfileWithRootsAndDenies(workspace, roots, nil)
}

// SeatbeltProfileWithRootsAndDenies records exact path denials in the profile
// for future signed-helper integrations. The production native runner also
// stages these paths out of the disposable view before launch. Denies are
// absolute, expanded paths; callers must resolve user glob patterns against a
// disposable manifest.
func SeatbeltProfileWithRootsAndDenies(workspace string, roots, denies []string) (string, error) {
	base, err := SeatbeltProfile(workspace)
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSuffix(base, "\n"), "\n")
	insert := 2 // immediately after deny default, before process/workspace rules
	readRoots := []string{"/usr", "/bin", "/sbin", "/System/Library", "/Library/Frameworks", "/private/etc", "/private/var/db", "/private/var/run", "/opt/homebrew", "/usr/local", "/dev"}
	readRoots = append(readRoots, roots...)
	seen := make(map[string]struct{}, len(readRoots))
	for _, root := range readRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			return "", fmt.Errorf("resolve Seatbelt runtime root: %w", err)
		}
		abs = filepath.Clean(abs)
		if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
			abs = resolved
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		quoted := strings.ReplaceAll(strings.ReplaceAll(abs, "\\", "\\\\"), "\"", "\\\"")
		lines = append(lines[:insert], append([]string{fmt.Sprintf("(allow file-read* (subpath \"%s\"))", quoted)}, lines[insert:]...)...)
		insert++
	}
	// Keep explicit deny metadata in the profile for helper implementations.
	// The runner does not rely on these rules for sensitive-data enforcement.
	for _, deny := range denies {
		deny = strings.TrimSpace(deny)
		if deny == "" {
			continue
		}
		abs, err := filepath.Abs(deny)
		if err != nil {
			return "", fmt.Errorf("resolve Seatbelt deny path: %w", err)
		}
		abs = filepath.Clean(abs)
		if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
			abs = resolved
		}
		quoted := strings.ReplaceAll(strings.ReplaceAll(abs, "\\", "\\\\"), "\"", "\\\"")
		lines = append(lines,
			fmt.Sprintf("(deny file-read* (literal \"%s\"))", quoted),
			fmt.Sprintf("(deny file-read-data (literal \"%s\"))", quoted),
		)
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func writeDisposableProfile(profile string) (string, error) {
	file, err := os.CreateTemp("", "rewind-seatbelt-*.sb")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err := file.WriteString(profile); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}
