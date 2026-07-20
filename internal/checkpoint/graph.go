// Package checkpoint stores a small, deterministic dependency graph for
// future multi-agent transactions. It is deliberately separate from the
// single-run lifecycle until merge semantics are proven.
package checkpoint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type State string

const (
	Pending    State = "pending"
	Running    State = "running"
	Succeeded  State = "succeeded"
	RolledBack State = "rolled_back"
	Blocked    State = "blocked"
)

type Node struct {
	ID        string    `json:"id"`
	RunID     string    `json:"run_id"`
	Parents   []string  `json:"parents,omitempty"`
	State     State     `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Graph struct {
	Version int    `json:"version"`
	Nodes   []Node `json:"nodes"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func Open(path string) *Store { return &Store{path: strings.TrimSpace(path)} }

func (s *Store) Snapshot() (Graph, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLocked()
}

func (s *Store) Add(node Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(node.ID) == "" || strings.TrimSpace(node.RunID) == "" {
		return fmt.Errorf("checkpoint node id and run_id are required")
	}
	if node.State == "" {
		node.State = Pending
	}
	if !validState(node.State) {
		return fmt.Errorf("checkpoint node %s has invalid state %q", node.ID, node.State)
	}
	graph, err := s.readLocked()
	if err != nil {
		return err
	}
	for _, existing := range graph.Nodes {
		if existing.ID == node.ID {
			return fmt.Errorf("checkpoint node %s already exists", node.ID)
		}
	}
	for _, parent := range node.Parents {
		if _, ok := findNode(graph, parent); !ok {
			return fmt.Errorf("checkpoint parent %s does not exist", parent)
		}
	}
	now := time.Now().UTC()
	node.CreatedAt, node.UpdatedAt = now, now
	graph.Nodes = append(graph.Nodes, node)
	return s.writeLocked(graph)
}

func (s *Store) Transition(id string, state State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validState(state) {
		return fmt.Errorf("checkpoint state %q is invalid", state)
	}
	graph, err := s.readLocked()
	if err != nil {
		return err
	}
	index := -1
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("checkpoint node %s does not exist", id)
	}
	if !allowedTransition(graph.Nodes[index].State, state) {
		return fmt.Errorf("checkpoint transition %s -> %s is not allowed", graph.Nodes[index].State, state)
	}
	if state == Succeeded {
		for _, child := range graph.Nodes {
			if contains(child.Parents, id) && child.State == Pending {
				return fmt.Errorf("checkpoint %s cannot succeed while child %s is pending", id, child.ID)
			}
		}
	}
	graph.Nodes[index].State = state
	graph.Nodes[index].UpdatedAt = time.Now().UTC()
	return s.writeLocked(graph)
}

func (s *Store) readLocked() (Graph, error) {
	if s.path == "" {
		return Graph{Version: 1, Nodes: []Node{}}, nil
	}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return Graph{Version: 1, Nodes: []Node{}}, nil
	}
	if err != nil {
		return Graph{}, fmt.Errorf("read checkpoint graph: %w", err)
	}
	var graph Graph
	if err := json.Unmarshal(data, &graph); err != nil {
		return Graph{}, fmt.Errorf("decode checkpoint graph: %w", err)
	}
	if graph.Version == 0 {
		graph.Version = 1
	}
	sort.Slice(graph.Nodes, func(i, j int) bool { return graph.Nodes[i].ID < graph.Nodes[j].ID })
	return graph, nil
}

func (s *Store) writeLocked(graph Graph) error {
	if s.path == "" {
		return fmt.Errorf("checkpoint graph path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create checkpoint graph directory: %w", err)
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return fmt.Errorf("encode checkpoint graph: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".checkpoint-*.tmp")
	if err != nil {
		return fmt.Errorf("create checkpoint graph temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace checkpoint graph: %w", err)
	}
	return nil
}

func findNode(graph Graph, id string) (Node, bool) {
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return Node{}, false
}
func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func validState(value State) bool {
	return value == Pending || value == Running || value == Succeeded || value == RolledBack || value == Blocked
}
func allowedTransition(from, to State) bool {
	if from == to {
		return true
	}
	switch from {
	case Pending:
		return to == Running || to == Blocked || to == RolledBack
	case Running:
		return to == Succeeded || to == RolledBack || to == Blocked
	case Succeeded:
		return to == RolledBack
	case Blocked, RolledBack:
		return false
	}
	return false
}
