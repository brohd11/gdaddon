package core

import (
	tea "github.com/charmbracelet/bubbletea"
)

// globalKey handles the keys available in any screen. It returns (act, true) when it
// consumed the key — act carries a control message resolved inline and/or an async cmd
// (e.g. tea.Quit or an output-scroll cmd) — or (Action{}, false) to let the active
// screen handle it. Pointer receiver: [ / ] mutate active/stack, which must persist
// back to Update's router.
func (r *Router) globalKey(msg tea.KeyMsg) (Action, bool) {
	k := msg.String()
	if k == "ctrl+c" {
		return Async(tea.Quit), true
	}

	ch := r.sh.Chrome
	outputOn := ch != nil && ch.Output != nil

	// When the output pane holds focus, navigation keys scroll it; everything
	// else either toggles back or clears.
	if outputOn && ch.outputFocused {
		switch {
		case MatchKey(k, Keys.ToggleOutput), MatchKey(k, Keys.Back):
			ch.outputFocused = false
			return Action{}, true
		case MatchKey(k, Keys.Output):
			ch.Output.Hide()
			ch.outputFocused = false
			return Action{}, true
		case MatchKey(k, Keys.Clear):
			r.clearOutput()
			return Action{}, true
		case MatchKey(k, Keys.Quit):
			return Async(tea.Quit), true
		}
		return Async(ch.Output.Update(msg)), true
	}

	// tab jumps into the output pane, c clears the log, [ / ] switch top-level tabs
	// (only at the root, so the live stack always belongs to the active tab), and `
	// unwinds a deep stack back to the root for a quick exit — unless the active
	// screen is capturing filter text. The output keys pass through (no consume) when
	// there is no output pane, so a chromeless app can bind tab/o itself.
	if f, ok := r.Top().(Filterer); !ok || !f.Filtering() {
		switch {
		case MatchKey(k, Keys.ToggleOutput):
			if !outputOn {
				break
			}
			if ch.Output.Shown() {
				ch.outputFocused = true
				ch.Output.GotoBottom()
			}
			return Action{}, true
		case MatchKey(k, Keys.Output):
			if !outputOn {
				break
			}
			ch.Output.Toggle()
			if !ch.Output.Shown() {
				ch.outputFocused = false
			}
			return Action{}, true
		case MatchKey(k, Keys.Clear):
			if ch == nil {
				break
			}
			r.clearOutput()
			return Action{}, true
		case MatchKey(k, Keys.Quit):
			// q is the global quit, handled once here for every screen (the filter
			// gate above keeps it from firing while a list/form is capturing text).
			return Async(tea.Quit), true
		case MatchKey(k, Keys.NextTab):
			return Action{}, r.switchTab(1)
		case MatchKey(k, Keys.PrevTab):
			return Action{}, r.switchTab(-1)
		case MatchKey(k, Keys.Unwind):
			// Unwind a deep stack back to the root for a quick exit. Only consume it
			// when there's something to unwind, so at the root the key passes through
			// to the active screen instead of being swallowed.
			if len(r.stack) > 1 {
				return ResetToRoot(), true
			}
		}
	}
	return Action{}, false
}

// switchTab moves the active tab by delta (wrapping), but only at the root — when
// drilled into a sub-screen the live stack belongs to the active tab and must not
// be swapped out from under it. The cached root preserves the tab's prior state.
// Reports whether it switched; when it didn't, the key passes through to the
// active screen (so [ / ] can be typed into a form at depth).
func (r *Router) switchTab(delta int) bool {
	if len(r.tabs) < 2 || len(r.stack) != 1 {
		return false
	}
	r.active = (r.active + delta + len(r.tabs)) % len(r.tabs)
	r.stack = []Screen{r.roots[r.active]}
	return true
}

// clearOutput empties the output pane and the status line and returns focus to the
// body (the Clear key). No-op without chrome.
func (r *Router) clearOutput() {
	ch := r.sh.Chrome
	if ch == nil {
		return
	}
	if ch.Output != nil {
		ch.Output.Clear()
	}
	if ch.Status != nil {
		ch.Status.Clear()
	}
	ch.outputFocused = false
}
