//go:build windows

package platform

import "testing"

func TestWindowsJobOptionsRequireCommand(t *testing.T) {
	if _, _, err := StartInWindowsJobWithOptions(WindowsJobOptions{}); err == nil {
		t.Fatal("empty Windows command unexpectedly accepted")
	}
}
