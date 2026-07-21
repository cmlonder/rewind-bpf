//go:build !darwin

package platform

import (
	"context"
	"fmt"

	"github.com/rewindbpf/rewind/internal/diff"
	"github.com/rewindbpf/rewind/internal/manifest"
)

// MacOSBackend is present on other targets only so the CLI remains
// cross-compilable; it always refuses to prepare a native transaction.
type MacOSBackend struct{}

func NewMacOSBackend() MacOSBackend { return MacOSBackend{} }

func (MacOSBackend) Capabilities() Capability {
	return Capability{Platform: "darwin", Backend: "apfs-clone-seatbelt", Reasons: []string{"macOS backend requested on a non-darwin host"}}
}

func (b MacOSBackend) Prepare(context.Context, string) (Transaction, error) {
	return nil, fmt.Errorf("macOS native backend is unavailable on this operating system")
}

func DiscardMacOSRuntime(string, string) error {
	return fmt.Errorf("macOS native backend is unavailable on this operating system")
}

func AcceptMacOSRuntime(context.Context, string, string, manifest.Manifest) ([]diff.Change, error) {
	return nil, fmt.Errorf("macOS native backend is unavailable on this operating system")
}
