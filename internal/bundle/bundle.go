// Package bundle creates portable, checksum-indexed evidence archives.
// Workspace contents are deliberately excluded: the bundle contains only the
// run record and event logs that were already persisted under the runtime root.
package bundle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rewindbpf/rewind/internal/runstore"
)

type Artifact struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type Metadata struct {
	Version   int                    `json:"version"`
	RunID     string                 `json:"run_id"`
	State     string                 `json:"state"`
	CreatedAt time.Time              `json:"created_at"`
	Events    runstore.EventEvidence `json:"events"`
	Artifacts []Artifact             `json:"artifacts"`
}

type source struct {
	name string
	path string
}

// Create writes an atomic .tar.gz evidence bundle. The record and event paths
// must be inside runtimeRoot; this prevents a record containing a surprising
// absolute path from turning a retention operation into file exfiltration.
func Create(outputPath, recordPath, runtimeRoot string, record runstore.Record) (Metadata, error) {
	if record.Plan.Run.ID == "" {
		return Metadata{}, fmt.Errorf("create evidence bundle: missing run id")
	}
	runtimeRoot, err := filepath.Abs(runtimeRoot)
	if err != nil {
		return Metadata{}, fmt.Errorf("resolve runtime root: %w", err)
	}
	recordPath, err = filepath.Abs(recordPath)
	if err != nil {
		return Metadata{}, fmt.Errorf("resolve record path: %w", err)
	}
	if !within(runtimeRoot, recordPath) {
		return Metadata{}, fmt.Errorf("record path is outside runtime root")
	}
	sources := []source{{name: "record.json", path: recordPath}}
	for index, path := range runstore.EventLogPaths(record) {
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return Metadata{}, fmt.Errorf("resolve event path: %w", err)
		}
		if !within(runtimeRoot, abs) {
			return Metadata{}, fmt.Errorf("event path is outside runtime root: %s", path)
		}
		name := fmt.Sprintf("events/%06d.jsonl", index)
		sources = append(sources, source{name: name, path: abs})
	}
	for _, item := range sources {
		if _, err := os.Stat(item.path); err != nil {
			return Metadata{}, fmt.Errorf("stat bundle artifact %s: %w", item.name, err)
		}
	}
	artifacts := make([]Artifact, 0, len(sources))
	for _, item := range sources {
		artifact, err := describe(item)
		if err != nil {
			return Metadata{}, err
		}
		artifacts = append(artifacts, artifact)
	}
	metadata := Metadata{
		Version:   1,
		RunID:     record.Plan.Run.ID,
		State:     string(record.Plan.Run.State),
		CreatedAt: record.Plan.Run.CreatedAt,
		Events:    record.Events,
		Artifacts: artifacts,
	}
	if err := writeArchive(outputPath, metadata, sources); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

func describe(item source) (Artifact, error) {
	file, err := os.Open(item.path)
	if err != nil {
		return Artifact{}, fmt.Errorf("open bundle artifact %s: %w", item.name, err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return Artifact{}, fmt.Errorf("stat bundle artifact %s: %w", item.name, err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return Artifact{}, fmt.Errorf("hash bundle artifact %s: %w", item.name, err)
	}
	return Artifact{Name: item.name, Bytes: stat.Size(), SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func writeArchive(outputPath string, metadata Metadata, sources []source) error {
	abs, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("resolve bundle output: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return fmt.Errorf("create bundle output directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".rewind-bundle-*")
	if err != nil {
		return fmt.Errorf("create bundle temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod bundle temporary file: %w", err)
	}
	gz := gzip.NewWriter(tmp)
	tarWriter := tar.NewWriter(gz)
	for _, item := range sources {
		if err := addFile(tarWriter, item.name, item.path); err != nil {
			_ = tarWriter.Close()
			_ = gz.Close()
			_ = tmp.Close()
			return err
		}
	}
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode bundle metadata: %w", err)
	}
	metadataJSON = append(metadataJSON, '\n')
	if err := addBytes(tarWriter, "bundle.json", metadataJSON); err != nil {
		return err
	}
	var checksums strings.Builder
	for _, artifact := range metadata.Artifacts {
		fmt.Fprintf(&checksums, "%s  %s\n", artifact.SHA256, artifact.Name)
	}
	if err := addBytes(tarWriter, "SHA256SUMS", []byte(checksums.String())); err != nil {
		return err
	}
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("close bundle tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("close bundle gzip: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close bundle output: %w", err)
	}
	if err := os.Rename(tmpPath, abs); err != nil {
		return fmt.Errorf("replace bundle output: %w", err)
	}
	return nil
}

func addFile(writer *tar.Writer, name, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open bundle artifact %s: %w", name, err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat bundle artifact %s: %w", name, err)
	}
	header := &tar.Header{Name: name, Mode: 0o600, Size: stat.Size(), ModTime: time.Unix(0, 0).UTC()}
	if err := writer.WriteHeader(header); err != nil {
		return fmt.Errorf("write bundle header %s: %w", name, err)
	}
	if _, err := io.Copy(writer, file); err != nil {
		return fmt.Errorf("write bundle artifact %s: %w", name, err)
	}
	return nil
}

func addBytes(writer *tar.Writer, name string, data []byte) error {
	header := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), ModTime: time.Unix(0, 0).UTC()}
	if err := writer.WriteHeader(header); err != nil {
		return fmt.Errorf("write bundle header %s: %w", name, err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write bundle artifact %s: %w", name, err)
	}
	return nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func SortedArtifacts(value Metadata) []Artifact {
	artifacts := append([]Artifact(nil), value.Artifacts...)
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Name < artifacts[j].Name })
	return artifacts
}
