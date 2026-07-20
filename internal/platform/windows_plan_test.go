package platform

import (
	"context"
	"runtime"
	"testing"
)

func TestWindowsPlanRefusesOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows contract test")
	}
	if _, err := PlanForWindowsWorkspace(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected non-Windows refusal")
	}
}
