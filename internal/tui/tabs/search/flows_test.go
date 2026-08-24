package search

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// newTestRouter builds a one-tab router over the Search screen. HOME is redirected at
// the call site so config.Sources() falls back to DefaultSources and the menu's rows are
// the built-in three plus the Asset Store.
func newTestRouter() core.Router {
	sh := core.NewShared(appctx.New("/tmp/gdaddon-test", "dev"))
	sh.Chrome = &core.Chrome{Header: core.NewHeaderPane(appctx.Header), Output: components.NewLogPane()}
	return core.NewRouter(sh, []core.TabEntry{
		{Title: "Search", New: func(*core.Shared) core.Screen { return NewSearchScreen() }},
	})
}

// pump delivers msg and feeds back the (single, non-batch) result of the command it
// returns — enough to drive the push/pop navigation commands. Copied from the tui
// package's router_test.go, which can't be imported from here.
func pump(tm tea.Model, msg tea.Msg) tea.Model {
	tm, cmd := tm.Update(msg)
	for i := 0; i < 8 && cmd != nil; i++ {
		out := cmd()
		if out == nil {
			break
		}
		if _, isBatch := out.(tea.BatchMsg); isBatch {
			break
		}
		tm, cmd = tm.Update(out)
	}
	return tm
}

// openSourceMenu walks the real path a user takes to the dropdown: the tab root's row
// pushes the query form, up moves focus from the query field to the Source row, and
// enter opens the menu.
func openSourceMenu(t *testing.T) (tea.Model, *components.MenuScreen) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	tm, _ := newTestRouter().Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEnter}) // "⌕ New search" → the query form
	if _, ok := tm.(core.Router).Top().(*components.FormScreen); !ok {
		t.Fatalf("enter on the search row should open the query form, got %T", tm.(core.Router).Top())
	}
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyUp}) // query → source
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEnter})

	menu, ok := tm.(core.Router).Top().(*components.MenuScreen)
	if !ok {
		t.Fatalf("enter on the Source row should drop a menu, got %T", tm.(core.Router).Top())
	}
	return tm, menu
}

// TestSourceMenuOpensUnderTheSourceRow is the end-to-end anchor check: the dropdown's
// box lands on the row directly below the Source field, at the field's value column, in
// the frame the router actually renders. A one-cell disagreement here is what puts the
// menu over the row it belongs to — and click hit-testing on the wrong row with it.
func TestSourceMenuOpensUnderTheSourceRow(t *testing.T) {
	tm, menu := openSourceMenu(t)
	if !menu.IsOverlay() {
		t.Fatal("the source menu should be an overlay, leaving the form drawn behind it")
	}

	lines := strings.Split(tm.View(), "\n")

	// The dropdown contributes no breadcrumb segment (MenuOpts.Crumb unset): it isn't a
	// place the user navigated to, and a bar that grows a segment on every open flickers.
	// Trimmed, because the bar is padded out to the terminal width.
	crumbs := ""
	for _, ln := range lines {
		if s := strings.TrimSpace(ansi.Strip(ln)); strings.HasPrefix(s, "Tab ›") {
			crumbs = s
			break
		}
	}
	if crumbs != "Tab › Search" {
		t.Errorf("the open menu should leave the trail at \"Tab › Search\", got %q", crumbs)
	}

	srcRow := -1
	for i, ln := range lines {
		if strings.Contains(ansi.Strip(ln), "Source:") {
			srcRow = i
			break
		}
	}
	if srcRow < 0 {
		t.Fatalf("no Source row in the rendered frame:\n%s", tm.View())
	}

	x, y := menu.OverlayPos(0, 0)
	if y != srcRow+1 {
		t.Errorf("menu should open one row below the Source row (%d), got %d", srcRow, y)
	}
	// The box's top-left corner has to be at exactly the cell OverlayPos reports, under
	// the value column — that agreement is what the menu hit-tests clicks against.
	row := []rune(ansi.Strip(lines[y]))
	if x >= len(row) || row[x] != '╭' {
		t.Errorf("menu box should start at column %d of row %d, got %q", x, y, string(row))
	}
	// Rune count, not the byte index: the row carries a box border and a "▸" marker, so
	// strings.Index would report a column 4 cells to the right of the real one.
	src := ansi.Strip(lines[srcRow])
	if want := utf8.RuneCountInString(src[:strings.Index(src, "GitHub")]); x != want {
		t.Errorf("menu should align with the Source value column %d, got %d", want, x)
	}
}

// TestSourceMenuSelects pins the two ways out: enter writes the choice back through the
// captured source and pops to the form, esc leaves it untouched.
func TestSourceMenuSelects(t *testing.T) {
	tm, menu := openSourceMenu(t)
	if got := menu.Selected(); got != 0 {
		t.Errorf("the cursor should start on the current source, got row %d", got)
	}

	tm = pump(tm, tea.KeyMsg{Type: tea.KeyDown})
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tm.(core.Router).Top().(*components.FormScreen); !ok {
		t.Fatalf("picking a row should pop back to the form, got %T", tm.(core.Router).Top())
	}
	// Read the row back off the router's own frame: the form renders through the Shared
	// the router sized, where a fresh one would fold the value at the 24-column floor.
	if view := ansi.Strip(tm.View()); !strings.Contains(view, "Source:  Asset Library") {
		t.Errorf("the Source row should show the picked source:\n%s", view)
	}

	// Esc closes the menu without changing the row.
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEnter}) // reopen
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyDown})
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEsc})
	if _, ok := tm.(core.Router).Top().(*components.FormScreen); !ok {
		t.Fatalf("esc should pop the menu back to the form, got %T", tm.(core.Router).Top())
	}
	if view := ansi.Strip(tm.View()); !strings.Contains(view, "Source:  Asset Library") {
		t.Errorf("esc should leave the source unchanged:\n%s", view)
	}
}
