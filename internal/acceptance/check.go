// Package acceptance checks whether a candidate run can be applied without
// overwriting edits made in the destination after the run started.
package acceptance

import (
	"sort"

	"github.com/rewindbpf/rewind/internal/diff"
	"github.com/rewindbpf/rewind/internal/manifest"
)

type Report struct {
	CanApply  bool          `json:"can_apply"`
	Changes   []diff.Change `json:"changes"`
	Conflicts []string      `json:"conflicts,omitempty"`
}

func Check(base, destination, candidate manifest.Manifest) Report {
	candidateChanges := diff.Compare(base, candidate)
	destinationChanges := diff.Compare(base, destination)
	changed := make(map[string]struct{}, len(candidateChanges))
	for _, change := range candidateChanges {
		changed[change.Path] = struct{}{}
	}
	conflicts := make([]string, 0)
	for _, change := range destinationChanges {
		if _, ok := changed[change.Path]; ok {
			conflicts = append(conflicts, change.Path)
		}
	}
	sort.Strings(conflicts)
	return Report{CanApply: len(conflicts) == 0, Changes: candidateChanges, Conflicts: conflicts}
}
