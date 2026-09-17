package tui

import (
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
)

func TestDensityFollowsPushedPickers(t *testing.T) {
	tm := sized(newTestRouter())
	root := tm.(core.Router).Top().(*components.RootListScreen)
	tm = pump(tm, keyMsg(densityKey()))
	p := components.ThemePicker().(*components.PickerScreen)
	tm = pump(tm, core.Push(p))
	if !p.Compact() {
		t.Fatal("a shared theme picker must inherit the root density")
	}
	tm = pump(tm, keyMsg(densityKey()))
	if p.Compact() || root.Compact() {
		t.Fatal("a picker toggle must update the root below it")
	}
	tm = pump(tm, keyMsg("esc"))
	if tm.(core.Router).Top() != root || root.Compact() {
		t.Fatal("back must retain the root and the picker's density choice")
	}
}

// densityKey is the chord under test, read off the keymap rather than spelled out, so a
// rebind moves these tests with it.
func densityKey() string { return appctx.AppKeys.Density.Keys()[0] }

// lineWith returns the first rendered line containing want, so a test can ask whether a
// row's title and description share a line without counting rows.
func lineWith(view, want string) string {
	for _, ln := range strings.Split(view, "\n") {
		if strings.Contains(ln, want) {
			return ln
		}
	}
	return ""
}

// The Actions tab's first row, title and description, as actionItems builds it.
const (
	actionRow  = "New Plugin"
	actionDesc = "add a plugin"
)

// TestDensityKeyCompactsTabList is the feature: the chord puts each row's description on
// its title line instead of a row of its own.
func TestDensityKeyCompactsTabList(t *testing.T) {
	tm := sized(newTestRouter())
	tm = pump(tm, keyMsg("]")) // Browse → Actions

	if line := lineWith(view(tm), actionRow); strings.Contains(line, actionDesc) {
		t.Fatalf("setup: the default density keeps the description on its own row, got %q", line)
	}

	tm = pump(tm, keyMsg(densityKey()))
	line := lineWith(view(tm), actionRow)
	if line == "" {
		t.Fatalf("the compact list must still render its rows:\n%s", view(tm))
	}
	if !strings.Contains(line, actionDesc) {
		t.Errorf("compact should carry the description as a suffix, got %q", line)
	}

	tm = pump(tm, keyMsg(densityKey()))
	if line := lineWith(view(tm), actionRow); strings.Contains(line, actionDesc) {
		t.Errorf("the chord should flip back to the three-row density, got %q", line)
	}
}

// TestDensityIsAppWide: the preference lives on Ctx, not on a tab root, so flipping it on
// one tab is already in force when another is opened. A density is a reading preference,
// not a property of one listing.
func TestDensityIsAppWide(t *testing.T) {
	tm := sized(newTestRouter())
	tm = pump(tm, keyMsg(densityKey())) // pressed on Browse
	tm = pump(tm, keyMsg("]"))          // → Actions, never pressed there

	if line := lineWith(view(tm), actionRow); !strings.Contains(line, actionDesc) {
		t.Errorf("a density set on one tab should be live on the next, got %q", line)
	}
}

// TestDensitySurvivesThemeChange is why Ctx holds the flag: a theme switch reinstances
// every tab root (core.RefreshRoots), so a screen-local flag would silently reset — the way
// the per-root sort mode already does.
func TestDensitySurvivesThemeChange(t *testing.T) {
	tm := sized(newTestRouter())
	tm = pump(tm, keyMsg("]"))
	tm = pump(tm, keyMsg(densityKey()))

	names := core.ThemeNames()
	tm = pump(tm, core.ApplyTheme(names[len(names)-1]).Msg)

	if line := lineWith(view(tm), actionRow); !strings.Contains(line, actionDesc) {
		t.Errorf("a theme change reinstances the roots; the density must survive it, got %q", line)
	}
}

// TestDensityKeyIsTypableWhileFiltering: the chord is a bare letter, so like every other
// tab key it stays text while a /-filter is being typed.
func TestDensityKeyIsTypableWhileFiltering(t *testing.T) {
	tm := sized(newTestRouter())
	tm = pump(tm, keyMsg("]"))
	tm = pump(tm, keyMsg("/"))
	tm = pump(tm, keyMsg(densityKey()))

	out := view(tm)
	if !strings.Contains(out, "Filter: "+densityKey()) {
		t.Errorf("the density chord should be typed into an open filter, got:\n%s", out)
	}
	if line := lineWith(out, actionRow); strings.Contains(line, actionDesc) {
		t.Error("the density must not flip from a keystroke meant for the filter")
	}
}

// TestDensityKeyInFullHelpNotBar: a flip is a command, so it belongs in the (?) menu and
// never on the deliberately sparse bottom bar.
func TestDensityKeyInFullHelpNotBar(t *testing.T) {
	tm := sized(newTestRouter())
	if out := view(tm); strings.Contains(out, "density") {
		t.Errorf("the help bar must stay sparse, got:\n%s", out)
	}
	tm = pump(tm, keyMsg("?"))
	if out := view(tm); !strings.Contains(out, "density") {
		t.Errorf("the (?) menu should advertise the density key, got:\n%s", out)
	}
}
