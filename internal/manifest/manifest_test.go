package manifest

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildAndVerify(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	expected, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(root, expected); err != nil {
		t.Fatalf("verify unchanged tree: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Verify(root, expected); err == nil {
		t.Fatal("expected verification to fail after content change")
	}

	var output bytes.Buffer
	if err := WriteJSON(&output, expected); err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadJSON(&output)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Entries) != len(expected.Entries) {
		t.Fatalf("decoded entry count: got %d, want %d", len(decoded.Entries), len(expected.Entries))
	}
}
