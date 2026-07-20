package platform

import (
	"context"
	"runtime"
	"testing"
)

func TestMacOSPlanRefusesOtherOperatingSystems(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("non-darwin contract case")
	}
	if _, err := PlanForWorkspace(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected non-darwin refusal")
	}
}
