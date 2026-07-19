package main

import (
	"os"
	"testing"

	"github.com/rewindbpf/rewind/internal/overlay"
)

func TestAgentOwnerUsesSudoIdentityWhenRoot(t *testing.T) {
	original := os.Geteuid()
	if original == 0 {
		t.Skip("test must run unprivileged on the development host")
	}
	owner, err := agentOwner()
	if err != nil {
		t.Fatal(err)
	}
	want := overlay.Owner{UID: os.Getuid(), GID: os.Getgid()}
	if owner != want {
		t.Fatalf("owner = %+v, want %+v", owner, want)
	}
}

func TestValidateOnSuccess(t *testing.T) {
	for _, value := range []string{"discard", "review"} {
		if err := validateOnSuccess(value); err != nil {
			t.Fatalf("validateOnSuccess(%q): %v", value, err)
		}
	}
	if err := validateOnSuccess("commit"); err == nil {
		t.Fatal("validateOnSuccess(commit) unexpectedly succeeded")
	}
}
