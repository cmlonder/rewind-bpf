package main

import (
	"os"
	"testing"
)

func TestParseIDEnv(t *testing.T) {
	t.Setenv("REWIND_TEST_ID", "1000")
	if got, err := parseIDEnv("REWIND_TEST_ID"); err != nil || got != 1000 {
		t.Fatalf("parseIDEnv = %d, %v", got, err)
	}
	t.Setenv("REWIND_TEST_ID", "0")
	if _, err := parseIDEnv("REWIND_TEST_ID"); err == nil {
		t.Fatal("expected zero ID rejection")
	}
	os.Unsetenv("REWIND_TEST_ID")
	if _, err := parseIDEnv("REWIND_TEST_ID"); err == nil {
		t.Fatal("expected missing ID rejection")
	}
}
