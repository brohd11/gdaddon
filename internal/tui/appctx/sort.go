package appctx

import (
	"github.com/brohd11/bubblestack/components"

	"charm.land/bubbles/v2/list"
)

// Re-exports of bubblestack's sort toggle under gdaddon's names. The domain sort lives in
// each tab's row builder (tabs/project/items.go).

type SortMode = components.SortMode

const (
	SortAlpha   = components.SortAlpha
	SortReverse = components.SortReverse
	SortStatus  = components.SortStatus
	// SortStatusInstalled is gdaddon's own mode, valued high to stay clear of the shared enum:
	// it sorts like SortStatus but hides uninstalled addons.
	SortStatusInstalled SortMode = 100
)

var (
	NextSort         = components.NextSort
	SortItemsByTitle = components.SortItemsByTitle
	SelectedTitle    = components.SelectedTitle
	SelectByTitle    = components.SelectByTitle
)

// SortTitle is components.SortTitle plus a label for the gdaddon-owned mode, which the
// shared package doesn't know.
func SortTitle(base string, m SortMode) string {
	if m == SortStatusInstalled {
		return base + " — status-installed"
	}
	return components.SortTitle(base, m)
}

// CycleSort is components.CycleSort retitling via the local SortTitle (above) instead
// of the shared one, so the gdaddon-owned mode gets its label.
func CycleSort(l *list.Model, mode *SortMode, modes []SortMode, base string, items func(SortMode) []list.Item) {
	sel := SelectedTitle(l)
	*mode = NextSort(*mode, modes)
	components.SetListItems(l, items(*mode))
	SelectByTitle(l, sel)
	l.Title = SortTitle(base, *mode)
}
