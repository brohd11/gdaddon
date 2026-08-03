package appctx

import (
	"github.com/brohd11/bubblestack/components"

	"github.com/charmbracelet/bubbles/list"
)

// The sort toggle mechanism (enum + label + cycle + the list.Model/list.Item helpers)
// lives in bubblestack/components now — it names no domain type, so a second consumer
// (repoview) can share it. appctx re-exports it under the historical names so gdaddon's
// tab call sites are unchanged (same pattern as GitRefresh = repoui.RefreshMsg). The
// domain sort — sortRows/attentionRank keyed on real addon.Status — stays in each tab's
// item builder (see tabs/project/items.go).

type SortMode = components.SortMode

const (
	SortAlpha   = components.SortAlpha
	SortReverse = components.SortReverse
	SortStatus  = components.SortStatus
	// SortStatusInstalled is gdaddon's own mode, deliberately outside the shared enum
	// ("installed" is a domain concept, and components stays generic for repoview).
	// The high value keeps it clear of any mode components adds later. It sorts like
	// SortStatus, but the Project tab hides rows whose addon isn't installed.
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
	l.SetItems(items(*mode))
	SelectByTitle(l, sel)
	l.Title = SortTitle(base, *mode)
}
