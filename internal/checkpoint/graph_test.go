package checkpoint

import (
	"path/filepath"
	"testing"
)

func TestGraphTracksDependenciesAndTransitions(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "graph.json"))
	if err := store.Add(Node{ID: "root", RunID: "run-root"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Node{ID: "child", RunID: "run-child", Parents: []string{"root"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition("root", Running); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition("root", Succeeded); err == nil {
		t.Fatal("expected pending child refusal")
	}
	if err := store.Transition("child", Running); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition("child", Succeeded); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition("root", Succeeded); err != nil {
		t.Fatal(err)
	}
}

func TestGraphRejectsUnknownParent(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "graph.json"))
	if err := store.Add(Node{ID: "child", RunID: "run-child", Parents: []string{"missing"}}); err == nil {
		t.Fatal("expected unknown parent refusal")
	}
}

func TestRollbackIsDescendantFirst(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "graph.json"))
	for _, node := range []Node{{ID: "root", RunID: "r"}, {ID: "child", RunID: "c", Parents: []string{"root"}}, {ID: "grandchild", RunID: "g", Parents: []string{"child"}}} {
		if err := store.Add(node); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Transition("root", Running); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition("child", Running); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition("grandchild", Running); err != nil {
		t.Fatal(err)
	}
	if err := store.Rollback("root"); err != nil {
		t.Fatal(err)
	}
	graph, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range graph.Nodes {
		if node.State != RolledBack {
			t.Fatalf("node %s=%s, want rolled_back", node.ID, node.State)
		}
	}
}
