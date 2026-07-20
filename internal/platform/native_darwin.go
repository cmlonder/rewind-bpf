//go:build darwin

package platform

import (
	"fmt"
	"os"
	"os/exec"
)

// SeatbeltCommand returns a process command wrapped by macOS's Seatbelt
// launcher. It is a process/read boundary only; APFS rollback and
// EndpointSecurity telemetry must be supplied by the native helper before a
// macOS protected run can be advertised as ready.
func SeatbeltCommand(workspace string, command string, args ...string) (*exec.Cmd, func() error, error) {
	profile, err := SeatbeltProfile(workspace)
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
