package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
		if strings.HasPrefix(r.URL.Path, prefix) && r.Method == http.MethodGet {
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
			if len(parts) != 2 {
				http.NotFound(w, r)
				return
			}
			signed, err := s.Store.Get(parts[0], strings.TrimSuffix(parts[1], ".json"))
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
