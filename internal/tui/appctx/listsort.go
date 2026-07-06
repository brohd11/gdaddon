package appctx

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

// This file holds the generic (domain-agnostic) list helpers backing the sort
// toggle: reorder rows by their display Title, and preserve the cursor on the same
// row across a rebuild. Tabs whose rows carry no sortable domain field (Archive)
// sort with SortItemsByTitle; tabs that sort real domain data (Project by state,
// Global/Project by name) still use SelectedTitle/SelectByTitle to keep the cursor
// on the highlighted row after re-sorting.

// SortItemsByTitle reorders rows in place by their Title, case-insensitively;
// reverse flips it. Stable, so equal titles keep their prior order.
func SortItemsByTitle(items []list.Item, reverse bool) {
	sort.SliceStable(items, func(i, j int) bool {
		a := strings.ToLower(itemTitle(items[i]))
		b := strings.ToLower(itemTitle(items[j]))
		if reverse {
			return a > b
		}
		return a < b
	})
}

// SelectedTitle returns the highlighted row's Title, or "" if there is none.
func SelectedTitle(l *list.Model) string { return itemTitle(l.SelectedItem()) }

// SelectByTitle moves the cursor to the first row whose Title matches title (a
// no-op for an empty title or no match), so a caller can keep the cursor on the
// same row after SetItems reorders the list.
func SelectByTitle(l *list.Model, title string) {
	if title == "" {
		return
	}
	for i, it := range l.Items() {
		if itemTitle(it) == title {
			l.Select(i)
			return
		}
	}
}

// CycleSort advances *sort to the next mode in modes and rebuilds l from items(*sort),
// keeping the cursor on the same row and retitling via SortTitle(base, *sort). The
// shared body of every tab root's sort toggle; items adapts each tab's row builder to
// the (mode) signature.
func CycleSort(l *list.Model, sort *SortMode, modes []SortMode, base string, items func(SortMode) []list.Item) {
	sel := SelectedTitle(l)
	*sort = NextSort(*sort, modes)
	l.SetItems(items(*sort))
	SelectByTitle(l, sel)
	l.Title = SortTitle(base, *sort)
}

func itemTitle(it list.Item) string {
	if t, ok := it.(interface{ Title() string }); ok {
		return t.Title()
	}
	return ""
}
