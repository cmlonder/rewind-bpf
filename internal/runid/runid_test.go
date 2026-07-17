package runid

import (
	"strings"
	"testing"
)

func TestNewHasStablePrefixAndIsUniqueEnough(t *testing.T) {
	first, err := New()
	if err != nil {
		t.Fatal(err)
	}
	second, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first, "run_") || !strings.HasPrefix(second, "run_") {
		t.Fatalf("invalid IDs: %q %q", first, second)
	}
	if first == second {
		t.Fatalf("IDs unexpectedly equal: %q", first)
	}
}
