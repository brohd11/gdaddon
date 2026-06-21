package core

import (
	"sort"

	"github.com/charmbracelet/lipgloss"
)

// Theme is a named set of the framework's semantic colors. The derived styles in
// shared.go are built from these four colors, so a Theme is the single knob that
// repaints the whole TUI. SetTheme swaps the active Theme and rebuilds those
// styles. Two presets ship below; consumers can add their own with RegisterTheme
// (the hook a future config file will load presets through).
type Theme struct {
	Name      string
	Muted     lipgloss.Color // secondary text: labels, help, list descriptions
	Log       lipgloss.Color // near-white output/log text
	Border    lipgloss.Color // box/rule borders
	Focused   lipgloss.Color // selection / active accent
	OnFocused lipgloss.Color // text drawn on the accent (title bar); empty ⇒ defaultOnFocused
}

// defaultOnFocused is the title-bar text color used when a theme leaves OnFocused
// empty: near-black, readable on a light accent. Themes with a dark Focused should
// set OnFocused to a light color instead.
const defaultOnFocused = lipgloss.Color("232")

// themes is the preset registry, keyed by Theme.Name.
var themes = map[string]Theme{
	"lipgloss": {Name: "lipgloss", Muted: "247", Log: "252", Border: "245", Focused: "212", OnFocused: "232"},
	// mono is a monochrome black/white/grey palette: a bright-white accent in
	// place of the default pink, greys for borders and secondary text, and the
	// terminal's own background for black.
	"mono":  {Name: "mono", Muted: "245", Log: "252", Border: "243", Focused: "255", OnFocused: "232"},
	"godot": {Name: "godot", Muted: "247", Log: "252", Border: "245", Focused: "67", OnFocused: "232"},
	"red":   {Name: "red", Muted: "247", Log: "252", Border: "245", Focused: "203", OnFocused: "232"},
	"green": {Name: "green", Muted: "247", Log: "252", Border: "245", Focused: "114", OnFocused: "232"},
	"amber": {Name: "amber", Muted: "247", Log: "252", Border: "245", Focused: "214", OnFocused: "232"},
}

// current is the active theme; applyTheme keeps it and the color vars in sync.
var current = themes["mono"]

// RegisterTheme adds or overrides a preset (keyed by t.Name). This is the entry
// point a config file uses to define custom themes; it does not switch to the
// theme — call SetTheme(t.Name) for that.
func RegisterTheme(t Theme) { themes[t.Name] = t }

// SetTheme switches to the named preset, reassigns the palette, and rebuilds the
// derived styles so the next render uses the new colors. An unknown name leaves
// the current theme untouched and returns false.
func SetTheme(name string) bool {
	t, ok := themes[name]
	if !ok {
		return false
	}
	applyTheme(t)
	return true
}

// CurrentTheme is the name of the active theme, for a picker to mark/select it.
func CurrentTheme() string { return current.Name }

// ApplyTheme is the in-TUI form of SetTheme: it switches the theme (synchronously) and
// broadcasts MsgThemeChanged via PropagateAll. The router only routes the payload; a
// consumer's App Receive recognizes it and returns RefreshRoots() to rebuild the cached
// tab roots with the new palette — so the framework no longer hard-codes that policy. A
// picker's row returns this Action on select.
func ApplyTheme(name string) Action {
	SetTheme(name)
	return PropagateAll(MsgThemeChanged{})
}

// ThemeNames returns the registered preset names, sorted, for a picker/listing.
func ThemeNames() []string {
	names := make([]string, 0, len(themes))
	for name := range themes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// applyTheme makes t the active theme: it points the exported color vars at t's
// colors, then rebuilds every derived style from them.
func applyTheme(t Theme) {
	current = t
	MutedColor, logColor, BorderColor, FocusedColor = t.Muted, t.Log, t.Border, t.Focused
	OnFocusedColor = t.OnFocused
	if OnFocusedColor == "" {
		OnFocusedColor = defaultOnFocused
	}
	rebuildStyles()
}
