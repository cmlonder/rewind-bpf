package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rewindbpf/rewind/internal/policybundle"
)

// FileStore is a small reference registry for a single operator or a
// disposable VM. It stores only signed envelopes and uses an atomic rename;
// production deployments can replace it with an object-store implementation.
type FileStore struct{ Root string }

func (s FileStore) path(name, version string) (string, error) {
	if strings.TrimSpace(s.Root) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" || filepath.Base(name) != name || filepath.Base(version) != version {
		return "", fmt.Errorf("invalid registry key")
	}
	return filepath.Join(s.Root, name, version+".json"), nil
}

func (s FileStore) Put(name, version string, signed policybundle.Signed) error {
	if _, err := policybundle.Verify(signed); err != nil {
		return fmt.Errorf("registry refuses unverified envelope: %w", err)
	}
	path, err := s.path(name, version)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(signed, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s FileStore) Get(name, version string) (policybundle.Signed, error) {
	if s.revoked(name, version) {
		return policybundle.Signed{}, os.ErrPermission
	}
	path, err := s.path(name, version)
	if err != nil {
		return policybundle.Signed{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return policybundle.Signed{}, err
	}
	var signed policybundle.Signed
	if err := json.Unmarshal(data, &signed); err != nil {
		return policybundle.Signed{}, err
	}
	if _, err := policybundle.Verify(signed); err != nil {
		return policybundle.Signed{}, fmt.Errorf("registry contains unverified envelope: %w", err)
	}
	return signed, nil
}

func (s FileStore) List() ([]Entry, error) {
	if strings.TrimSpace(s.Root) == "" {
		return nil, fmt.Errorf("registry root is required")
	}
	var entries []Entry
	err := filepath.WalkDir(s.Root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || strings.HasSuffix(entry.Name(), ".tmp") {
			return nil
		}
		relative, err := filepath.Rel(s.Root, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) != 2 || strings.HasPrefix(parts[0], ".") {
			return nil
		}
		version := strings.TrimSuffix(parts[1], ".json")
		if s.revoked(parts[0], version) {
			return nil
		}
		entries = append(entries, Entry{Name: parts[0], Version: version})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Version < entries[j].Version
	})
	return entries, nil
}

func (s FileStore) Revoke(name, version string) error {
	if _, err := s.Get(name, version); err != nil {
		return err
	}
	path, err := s.path(name, version)
	if err != nil {
		return err
	}
	marker := filepath.Join(filepath.Dir(path), ".revoked", filepath.Base(path))
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		return err
	}
	tmp := marker + ".tmp"
	if err := os.WriteFile(tmp, []byte("revoked\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, marker)
}

func (s FileStore) revoked(name, version string) bool {
	path, err := s.path(name, version)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(filepath.Dir(path), ".revoked", filepath.Base(path)))
	return err == nil
}

type Server struct {
	Store  FileStore
	Bearer string
}

func (s Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Bearer != "" && r.Header.Get("Authorization") != "Bearer "+s.Bearer {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		prefix := "/v1/registry/policies/"
		if r.URL.Path == "/v1/registry/policies" && r.Method == http.MethodGet {
			entries, err := s.Store.List()
			if err != nil {
				http.Error(w, "registry list failed", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(entries)
			return
		}
		if r.URL.Path == "/v1/registry/policies" && r.Method == http.MethodPost {
			var signed policybundle.Signed
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&signed); err != nil {
				http.Error(w, "invalid envelope", http.StatusBadRequest)
				return
			}
			bundle, err := policybundle.Verify(signed)
			if err != nil {
				http.Error(w, "unverified envelope", http.StatusBadRequest)
				return
			}
			if err := s.Store.Put(bundle.Name, bundle.Version, signed); err != nil {
				http.Error(w, "store failed", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			return
		}
		if strings.HasPrefix(r.URL.Path, prefix) && (r.Method == http.MethodGet || r.Method == http.MethodDelete) {
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
			if len(parts) != 2 {
				http.NotFound(w, r)
				return
			}
			version := strings.TrimSuffix(parts[1], ".json")
			if r.Method == http.MethodDelete {
				if err := s.Store.Revoke(parts[0], version); err != nil {
					if os.IsNotExist(err) {
						http.NotFound(w, r)
						return
					}
					http.Error(w, "registry revoke failed", http.StatusConflict)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			signed, err := s.Store.Get(parts[0], version)
			if errors.Is(err, os.ErrPermission) {
				http.Error(w, "policy revoked", http.StatusGone)
				return
			}
			if os.IsNotExist(err) {
				http.NotFound(w, r)
				return
			}
			if err != nil {
				http.Error(w, "registry read failed", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(signed)
			return
		}
		http.NotFound(w, r)
	})
}
