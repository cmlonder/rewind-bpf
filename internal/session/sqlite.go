package session

// SQLiteStore is the durable coordination backend for multi-process and
// remote-mounted supervisors. It intentionally keeps the same lease protocol
// as the local JSON store while delegating locking/expiry atomics to SQLite.

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct{ db *sql.DB }

func OpenSQLite(path string) (*SQLiteStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("sqlite session path is required")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite sessions: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure sqlite sessions: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS leases (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL UNIQUE,
		owner TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		expires_at INTEGER NOT NULL
	); CREATE INDEX IF NOT EXISTS leases_expiry ON leases(expires_at);`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create session schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) List() ([]Lease, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("sqlite session store is closed")
	}
	now := time.Now()
	if _, err := s.db.Exec(`DELETE FROM leases WHERE expires_at <= ?`, now.UnixNano()); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, run_id, owner, created_at, updated_at, expires_at FROM leases ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Lease
	for rows.Next() {
		var l Lease
		var created, updated, expires int64
		if err := rows.Scan(&l.ID, &l.RunID, &l.Owner, &created, &updated, &expires); err != nil {
			return nil, err
		}
		l.CreatedAt, l.UpdatedAt, l.ExpiresAt = time.Unix(0, created), time.Unix(0, updated), time.Unix(0, expires)
		result = append(result, l)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) Apply(request Request, now time.Time) (Lease, error) {
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
	tx, err := s.db.Begin()
	if err != nil {
		return Lease{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM leases WHERE expires_at <= ?`, now.UnixNano()); err != nil {
		return Lease{}, err
	}
	var current Lease
	var created, updated, expires int64
	err = tx.QueryRow(`SELECT id, run_id, owner, created_at, updated_at, expires_at FROM leases WHERE run_id = ?`, request.RunID).Scan(&current.ID, &current.RunID, &current.Owner, &created, &updated, &expires)
	found := err == nil
	if found {
		current.CreatedAt, current.UpdatedAt, current.ExpiresAt = time.Unix(0, created), time.Unix(0, updated), time.Unix(0, expires)
	}
	if err != nil && err != sql.ErrNoRows {
		return Lease{}, err
	}
	if request.Action == "acquire" && found && current.Owner != request.Owner {
		return Lease{}, fmt.Errorf("run is owned by %s until %s", current.Owner, current.ExpiresAt.UTC().Format(time.RFC3339))
	}
	if request.Action == "heartbeat" && (!found || current.Owner != request.Owner) {
		return Lease{}, fmt.Errorf("session heartbeat refused")
	}
	if request.Action == "release" && (!found || current.Owner != request.Owner) {
		return Lease{}, fmt.Errorf("session release refused")
	}
	if request.Action != "acquire" && request.Action != "heartbeat" && request.Action != "takeover" && request.Action != "release" {
		return Lease{}, fmt.Errorf("unsupported session action %q", request.Action)
	}
	if request.Action == "release" {
		if _, err := tx.Exec(`DELETE FROM leases WHERE run_id = ?`, request.RunID); err != nil {
			return Lease{}, err
		}
		if err := tx.Commit(); err != nil {
			return Lease{}, err
		}
		return current, nil
	}
	if request.Action == "acquire" && found {
		current.UpdatedAt, current.ExpiresAt = now, now.Add(ttl)
		_, err = tx.Exec(`UPDATE leases SET updated_at=?, expires_at=? WHERE run_id=?`, now.UnixNano(), now.Add(ttl).UnixNano(), request.RunID)
	} else {
		id, idErr := newID()
		if idErr != nil {
			return Lease{}, idErr
		}
		current = Lease{ID: id, RunID: request.RunID, Owner: request.Owner, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(ttl)}
		if found {
			_, err = tx.Exec(`UPDATE leases SET id=?, owner=?, created_at=?, updated_at=?, expires_at=? WHERE run_id=?`, current.ID, current.Owner, now.UnixNano(), now.UnixNano(), now.Add(ttl).UnixNano(), request.RunID)
		} else {
			_, err = tx.Exec(`INSERT INTO leases(id,run_id,owner,created_at,updated_at,expires_at) VALUES(?,?,?,?,?,?)`, current.ID, current.RunID, current.Owner, now.UnixNano(), now.UnixNano(), now.Add(ttl).UnixNano())
		}
	}
	if err != nil {
		return Lease{}, fmt.Errorf("write sqlite lease: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Lease{}, err
	}
	return current, nil
}
