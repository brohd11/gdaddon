package appctx

import "github.com/brohd11/bubblestack/components"

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
)

var (
	SortTitle        = components.SortTitle
	NextSort         = components.NextSort
	SortItemsByTitle = components.SortItemsByTitle
	SelectedTitle    = components.SelectedTitle
	SelectByTitle    = components.SelectByTitle
	CycleSort        = components.CycleSort
)
