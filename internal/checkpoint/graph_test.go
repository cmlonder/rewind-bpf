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
