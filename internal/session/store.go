// Package session stores short-lived operator ownership leases for runs.
// It coordinates reconnect/takeover; it never grants filesystem privileges by
// itself. Runtime actions still pass through the authenticated supervisor.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Lease struct {
	ID        string    `json:"id"`
	RunID     string    `json:"run_id"`
	Owner     string    `json:"owner"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Request struct {
	Action string `json:"action"`
	RunID  string `json:"run_id"`
	Owner  string `json:"owner"`
	TTL    int    `json:"ttl_seconds,omitempty"`
}

type Store struct {
	path string
	mu   *sync.Mutex
}

func Open(path string) Store { return Store{path: path, mu: &sync.Mutex{}} }

func (s Store) List() ([]Lease, error) {
	return s.listAt(time.Now())
}

func (s Store) listAt(now time.Time) ([]Lease, error) {
	if s.path == "" {
		return nil, fmt.Errorf("session path is required")
	}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []Lease{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read sessions: %w", err)
	}
	var leases []Lease
	if err := json.Unmarshal(data, &leases); err != nil {
		return nil, fmt.Errorf("decode sessions: %w", err)
	}
	filtered := leases[:0]
	for _, lease := range leases {
		if lease.ExpiresAt.After(now) {
			filtered = append(filtered, lease)
		}
	}
	return filtered, nil
}

func (s Store) Apply(request Request, now time.Time) (Lease, error) {
	if strings.TrimSpace(request.RunID) == "" || strings.TrimSpace(request.Owner) == "" {
		return Lease{}, fmt.Errorf("session run_id and owner are required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	ttl := time.Duration(request.TTL) * time.Second
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if ttl > 24*time.Hour {
		return Lease{}, fmt.Errorf("session TTL cannot exceed 24 hours")
	}
	if s.mu == nil {
		s.mu = &sync.Mutex{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	leases, err := s.listAt(now)
	if err != nil {
		return Lease{}, err
	}
	index := -1
	for i := range leases {
		if leases[i].RunID == request.RunID {
			index = i
			break
		}
	}
	switch request.Action {
	case "acquire":
		if index >= 0 && leases[index].Owner != request.Owner {
			return Lease{}, fmt.Errorf("run is owned by %s until %s", leases[index].Owner, leases[index].ExpiresAt.UTC().Format(time.RFC3339))
		}
		if index >= 0 {
			leases[index].UpdatedAt = now
			leases[index].ExpiresAt = now.Add(ttl)
			if err := s.write(leases); err != nil {
				return Lease{}, err
			}
			return leases[index], nil
		}
		id, err := newID()
		if err != nil {
			return Lease{}, err
		}
		lease := Lease{ID: id, RunID: request.RunID, Owner: request.Owner, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(ttl)}
		if err := s.write(append(leases, lease)); err != nil {
			return Lease{}, err
		}
		return lease, nil
	case "heartbeat":
		if index < 0 || leases[index].Owner != request.Owner {
			return Lease{}, fmt.Errorf("session heartbeat refused")
		}
		leases[index].UpdatedAt = now
		leases[index].ExpiresAt = now.Add(ttl)
		if err := s.write(leases); err != nil {
			return Lease{}, err
		}
		return leases[index], nil
	case "takeover":
		id, err := newID()
		if err != nil {
			return Lease{}, err
		}
		lease := Lease{ID: id, RunID: request.RunID, Owner: request.Owner, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(ttl)}
		if index >= 0 {
			leases[index] = lease
		} else {
			leases = append(leases, lease)
		}
		if err := s.write(leases); err != nil {
			return Lease{}, err
		}
		return lease, nil
	case "release":
		if index < 0 || leases[index].Owner != request.Owner {
			return Lease{}, fmt.Errorf("session release refused")
		}
		lease := leases[index]
		if err := s.write(append(leases[:index], leases[index+1:]...)); err != nil {
			return Lease{}, err
		}
		return lease, nil
	default:
		return Lease{}, fmt.Errorf("unsupported session action %q", request.Action)
	}
}

func (s Store) write(leases []Lease) error {
	if s.path == "" {
		return fmt.Errorf("session path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}
	data, err := json.MarshalIndent(leases, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}
