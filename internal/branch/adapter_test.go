package branch

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rewindbpf/rewind/internal/manifest"
)

func TestApplyToCleanCheckedOutBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.email", "test@example.invalid")
	runGit(t, root, "config", "user.name", "Rewind Test")
	writeFile(t, filepath.Join(root, "marker.txt"), "original\n")
	runGit(t, root, "add", "--all")
	runGit(t, root, "commit", "-m", "initial")

	candidate := t.TempDir()
	copyTree(t, root, candidate)
	writeFile(t, filepath.Join(candidate, "marker.txt"), "accepted\n")
	writeFile(t, filepath.Join(candidate, "generated.txt"), "from-agent\n")
	base, err := manifest.Build(root)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := manifest.Build(root)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := manifest.Build(candidate)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Apply(base, destination, merged, candidate, root, "main", "accept agent result", true)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Applied || report.CommitID == "" {
		t.Fatalf("unexpected report: %+v", report)
	}
	if got := readFile(t, filepath.Join(root, "marker.txt")); got != "accepted\n" {
		t.Fatalf("marker = %q", got)
	}
	if got := readFile(t, filepath.Join(root, "generated.txt")); got != "from-agent\n" {
		t.Fatalf("generated = %q", got)
	}
}

func TestApplyRefusesDirtyOrWrongBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.email", "test@example.invalid")
	runGit(t, root, "config", "user.name", "Rewind Test")
	writeFile(t, filepath.Join(root, "marker.txt"), "original\n")
	runGit(t, root, "add", "--all")
	runGit(t, root, "commit", "-m", "initial")
	candidate := t.TempDir()
	writeFile(t, filepath.Join(candidate, "marker.txt"), "accepted\n")
	base, _ := manifest.Build(root)
	merged, _ := manifest.Build(candidate)
	if _, err := Apply(base, base, merged, candidate, root, "feature", "", false); err == nil {
		t.Fatal("expected wrong branch refusal")
	}
	writeFile(t, filepath.Join(root, "dirty.txt"), "do not overwrite\n")
	if _, err := Apply(base, base, merged, candidate, root, "main", "", false); err == nil {
		t.Fatal("expected dirty worktree refusal")
	}
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output)
}

func writeFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func copyTree(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(destination, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		t.Fatal(err)
	}
}
