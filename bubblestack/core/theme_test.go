package core

import (
	"reflect"
	"sort"
	"testing"
)

// restoreTheme snaps the active theme back after a test mutates the global palette.
func restoreTheme(t *testing.T) {
	prev := CurrentTheme()
	t.Cleanup(func() { SetTheme(prev) })
}

func TestSetThemeKnownUnknown(t *testing.T) {
	restoreTheme(t)
	if !SetTheme("godot") {
		t.Fatal("SetTheme should accept a known preset")
	}
	if CurrentTheme() != "godot" {
		t.Fatalf("CurrentTheme should track the switch, got %q", CurrentTheme())
	}
	if SetTheme("does-not-exist") {
		t.Error("SetTheme should reject an unknown preset")
	}
	if CurrentTheme() != "godot" {
		t.Errorf("a rejected SetTheme should leave the theme untouched, got %q", CurrentTheme())
	}
}

func TestRegisterThemeAndNames(t *testing.T) {
	restoreTheme(t)
	RegisterTheme(Theme{Name: "zz-test", Muted: "1", Log: "2", Border: "3", Focused: "4", OnFocused: "5"})
	if !SetTheme("zz-test") {
		t.Fatal("a registered theme should be resolvable by SetTheme")
	}
	names := ThemeNames()
	if !sort.StringsAreSorted(names) {
		t.Errorf("ThemeNames should be sorted, got %v", names)
	}
	found := false
	for _, n := range names {
		if n == "zz-test" {
			found = true
		}
	}
	if !found {
		t.Error("ThemeNames should include a newly registered theme")
	}
}

func TestApplyThemeBroadcasts(t *testing.T) {
	restoreTheme(t)
	act := ApplyTheme("amber")
	if CurrentTheme() != "amber" {
		t.Fatalf("ApplyTheme should switch the active theme, got %q", CurrentTheme())
	}
	if !reflect.DeepEqual(act, PropagateAll(MsgThemeChanged{})) {
		t.Errorf("ApplyTheme should broadcast MsgThemeChanged, got %+v", act)
	}
}

func TestApplyThemeOnFocusedFallback(t *testing.T) {
	restoreTheme(t)
	// A theme leaving OnFocused empty falls back to defaultOnFocused.
	RegisterTheme(Theme{Name: "no-onfocused", Muted: "1", Log: "2", Border: "3", Focused: "4"})
	SetTheme("no-onfocused")
	if OnFocusedColor != defaultOnFocused {
		t.Errorf("empty OnFocused should fall back to defaultOnFocused, got %q", OnFocusedColor)
	}
}
