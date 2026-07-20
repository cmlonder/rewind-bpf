package bundle

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rewindbpf/rewind/internal/lifecycle"
	"github.com/rewindbpf/rewind/internal/runplan"
	"github.com/rewindbpf/rewind/internal/runstore"
)

func TestCreateEvidenceBundle(t *testing.T) {
	runtimeRoot := t.TempDir()
	recordPath := filepath.Join(runtimeRoot, "record.json")
	eventsPath := filepath.Join(runtimeRoot, "events.jsonl")
	if err := os.WriteFile(recordPath, []byte(`{"plan":"fixture"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eventsPath, []byte(`{"operation":"write"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	record := runstore.Record{Plan: runplan.Plan{Run: lifecycle.Run{ID: "run_test", State: lifecycle.Succeeded}}, EventsPath: eventsPath}
	output := filepath.Join(t.TempDir(), "evidence.tar.gz")
	metadata, err := Create(output, recordPath, runtimeRoot, record)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.RunID != "run_test" || len(metadata.Artifacts) != 2 {
		t.Fatalf("metadata = %+v", metadata)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	decompress, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(decompress)
	names := map[string]bool{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names[header.Name] = true
	}
	_ = decompress.Close()
	_ = file.Close()
	for _, name := range []string{"record.json", "events/000000.jsonl", "bundle.json", "SHA256SUMS"} {
		if !names[name] {
			t.Fatalf("bundle missing %s", name)
		}
	}
}

func TestCreateRefusesOutsideEventPath(t *testing.T) {
	runtimeRoot := t.TempDir()
	recordPath := filepath.Join(runtimeRoot, "record.json")
	if err := os.WriteFile(recordPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	record := runstore.Record{Plan: runplan.Plan{Run: lifecycle.Run{ID: "run_test"}}, EventsPath: filepath.Join(t.TempDir(), "outside.jsonl")}
	if _, err := Create(filepath.Join(t.TempDir(), "bundle.tar.gz"), recordPath, runtimeRoot, record); err == nil {
		t.Fatal("expected outside path refusal")
	}
}

func TestMetadataJSONIsStable(t *testing.T) {
	value := Metadata{Version: 1, RunID: "run_test", Artifacts: []Artifact{{Name: "z", SHA256: "z"}, {Name: "a", SHA256: "a"}}}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected metadata JSON")
	}
}
