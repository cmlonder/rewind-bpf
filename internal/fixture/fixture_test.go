package fixture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateIsDeterministicAndSynthetic(t *testing.T) {
	root := t.TempDir()
	if err := Create(root); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"workspace/src/main.go",
		"workspace/.env.example",
		"secrets/demo.env",
		"secrets/demo.pem",
		"data/pii/demo-user.json",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("fixture path %s: %v", path, err)
		}
	}
}
