package components

import (
	"github.com/brohd/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// Item is a self-dispatching list row: instead of a domain-specific item type +
// a kind enum + a switch in the owning screen, each row carries its own Pick
// closure. A PickerScreen (or a tab root) runs Pick on enter, so a list of mixed
// commands needs no bespoke Update logic — building the rows is the whole flow.
//
// It is context-agnostic (names no domain type), like the other components: the
// caller supplies the closures. A nil Pick marks an inert row (e.g. an empty-list
// placeholder); Keys is optional per-row key handling.
type Item struct {
	Name, Desc, Filter string
	Pick               func(*core.Shared) tea.Cmd
	Keys               func(*core.Shared, string) (tea.Cmd, bool)
}

func (i Item) Title() string       { return i.Name }
func (i Item) Description() string { return i.Desc }
func (i Item) FilterValue() string {
	if i.Filter != "" {
		return i.Filter
	}
	return i.Name
}
