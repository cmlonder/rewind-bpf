//go:build !darwin

package platform

import (
	"context"
	"fmt"
)

func ApplyNativeSupervisorAction(context.Context, string, string, string) (NativeActionResult, error) {
	return NativeActionResult{}, fmt.Errorf("native supervisor actions are unavailable on this platform")
}
