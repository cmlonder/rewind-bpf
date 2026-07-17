package fixture

import (
	"fmt"
	"os"
	"path/filepath"
)

// Create writes a deterministic, synthetic fixture. It contains no real
// secrets or personal data and is intended for local unit/integration tests.
func Create(root string) error {
	directories := []string{
		"workspace/src",
		"workspace/docs",
		"secrets",
		"data/pii",
	}
	for _, directory := range directories {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			return fmt.Errorf("create fixture directory %s: %w", directory, err)
		}
	}

	files := map[string]struct {
		content string
		mode    os.FileMode
	}{
		"workspace/README.md":      {"# Synthetic agent workspace\n", 0o644},
		"workspace/src/main.go":    {"package main\n\nfunc main() {}\n", 0o644},
		"workspace/src/config.go":  {"package main\n\nconst Demo = true\n", 0o644},
		"workspace/.env.example":   {"DEMO_ONLY=true\n", 0o644},
		"secrets/demo.env":         {"DEMO_SECRET=not-a-real-secret\n", 0o600},
		"secrets/demo.pem":         {"-----BEGIN DEMO KEY-----\nnot-real\n-----END DEMO KEY-----\n", 0o600},
		"data/pii/demo-user.json":  {"{\"name\":\"Synthetic User\",\"id\":\"demo-001\"}\n", 0o600},
		"workspace/docs/notes.txt": {"Synthetic fixture for safe tests.\n", 0o644},
	}
	for relative, value := range files {
		if err := os.WriteFile(filepath.Join(root, relative), []byte(value.content), value.mode); err != nil {
			return fmt.Errorf("write fixture file %s: %w", relative, err)
		}
	}
	return nil
}
