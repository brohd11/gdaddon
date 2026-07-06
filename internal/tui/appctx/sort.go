package appctx

// SortMode selects the ordering of a data-backed tab list (Project/Global/Archive).
// It is a per-screen session choice cycled by the "i" key; the owning screen holds
// the value and rebuilds its list when it changes. Only the enum + label + cycle
// live here (no bubbles/list dependency); the list-touching helpers are in
// listsort.go, and the domain sort (by name / by install state) is applied in each
// tab's item builder.
type SortMode int

const (
	SortAlpha   SortMode = iota // A→Z by name (case-insensitive)
	SortReverse                 // Z→A by name (case-insensitive)
	SortStatus                  // grouped by install state (Project only)
)

// SortTitle renders a list's base title with its active sort mode appended, e.g.
// "Godot Addons — A→Z". Shared by each tab root's New* and CycleSort.
func SortTitle(base string, m SortMode) string { return base + " — " + m.Label() }

// Label is the short suffix shown in a list's Title, e.g. "Godot Addons — A→Z".
func (m SortMode) Label() string {
	switch m {
	case SortReverse:
		return "Z→A"
	case SortStatus:
		return "status"
	default:
		return "A→Z"
	}
}

// NextSort advances cur to the next mode within the allowed set (wrapping), so a
// tab can restrict the cycle — Project offers {Alpha, Reverse, Status} while
// Global/Archive offer {Alpha, Reverse}. A cur not in modes (or an empty set)
// falls back to the first allowed mode.
func NextSort(cur SortMode, modes []SortMode) SortMode {
	for i, m := range modes {
		if m == cur {
			return modes[(i+1)%len(modes)]
		}
	}
	if len(modes) > 0 {
		return modes[0]
	}
	return cur
}
