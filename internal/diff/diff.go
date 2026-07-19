// Package diff compares the immutable start manifest with the current merged
// view of a protected run. It does not apply changes or mutate either tree.
package diff

import (
	"sort"

	"github.com/rewindbpf/rewind/internal/manifest"
)

type Kind string

const (
	Created  Kind = "created"
	Modified Kind = "modified"
	Deleted  Kind = "deleted"
)

type Change struct {
	Path   string          `json:"path"`
	Kind   Kind            `json:"kind"`
	Before *manifest.Entry `json:"before,omitempty"`
	After  *manifest.Entry `json:"after,omitempty"`
}

func Compare(before, after manifest.Manifest) []Change {
	left := make(map[string]manifest.Entry, len(before.Entries))
	right := make(map[string]manifest.Entry, len(after.Entries))
	for _, entry := range before.Entries {
		left[entry.Path] = entry
	}
	for _, entry := range after.Entries {
		right[entry.Path] = entry
	}
	changes := make([]Change, 0)
	for path, entry := range left {
		entryCopy := entry
		other, ok := right[path]
		if !ok {
			changes = append(changes, Change{Path: path, Kind: Deleted, Before: &entryCopy})
			continue
		}
		if entry != other {
			otherCopy := other
			changes = append(changes, Change{Path: path, Kind: Modified, Before: &entryCopy, After: &otherCopy})
		}
	}
	for path, entry := range right {
		if _, ok := left[path]; ok {
			continue
		}
		entryCopy := entry
		changes = append(changes, Change{Path: path, Kind: Created, After: &entryCopy})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}
