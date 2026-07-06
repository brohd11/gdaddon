package components

import (
	"reflect"
	"testing"

	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// navKey builds a real (non-rune) key message, so navigation keys reach the form's
// keybind switch instead of being diverted into a focused text field by QueryUpdate
// (which only swallows rune/space/backspace).
func navKey(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

// sampleForm builds a form whose focusable rows (name, scope, url) are separated by a
// non-focusable heading, so navigation has something to skip. It returns the form plus
// the toggle field for index assertions.
func sampleForm(opts ...func(*FormOpts)) (*FormScreen, *ToggleField) {
	scope := NewToggleField("scope", "Scope", []string{"A", "B", "C"})
	o := FormOpts{Fields: []FormField{
		NewTextField("name", "Name", ""),
		NewHeading("— section —"),
		scope,
		NewTextField("url", "URL", ""),
	}}
	for _, f := range opts {
		f(&o)
	}
	return NewForm(o), scope
}

func TestFormFocusCyclingSkipsAndWraps(t *testing.T) {
	f, _ := sampleForm()
	sh := core.NewShared(nil)
	if got := f.FocusedKey(); got != "name" {
		t.Fatalf("initial focus should be the first focusable field, got %q", got)
	}

	// down skips the heading and lands on the toggle, then the last text field.
	f.Update(sh, navKey(tea.KeyDown))
	if got := f.FocusedKey(); got != "scope" {
		t.Fatalf("NextField should skip the heading to scope, got %q", got)
	}
	f.Update(sh, navKey(tea.KeyDown))
	if got := f.FocusedKey(); got != "url" {
		t.Fatalf("NextField should advance to url, got %q", got)
	}
	// wrap forward back to the first focusable.
	f.Update(sh, navKey(tea.KeyDown))
	if got := f.FocusedKey(); got != "name" {
		t.Fatalf("NextField should wrap to name, got %q", got)
	}
	// up wraps backward to the last focusable, skipping the heading.
	f.Update(sh, navKey(tea.KeyUp))
	if got := f.FocusedKey(); got != "url" {
		t.Fatalf("PrevField should wrap backward to url, got %q", got)
	}
}

func TestFormInitialFocusOpt(t *testing.T) {
	f, _ := sampleForm(func(o *FormOpts) { o.Focus = "url" })
	if got := f.FocusedKey(); got != "url" {
		t.Fatalf("Focus opt should honor a focusable key, got %q", got)
	}

	// An unknown Focus key falls back to the first focusable field.
	g, _ := sampleForm(func(o *FormOpts) { o.Focus = "nope" })
	if got := g.FocusedKey(); got != "name" {
		t.Fatalf("unknown Focus key should fall back to first focusable, got %q", got)
	}
}

func TestFormToggleLeftRight(t *testing.T) {
	f, scope := sampleForm(func(o *FormOpts) { o.Focus = "scope" })
	sh := core.NewShared(nil)

	f.Update(sh, navKey(tea.KeyRight))
	if scope.Index() != 1 {
		t.Fatalf("Right on the focused toggle should advance the index, got %d", scope.Index())
	}
	f.Update(sh, navKey(tea.KeyLeft))
	if scope.Index() != 0 {
		t.Fatalf("Left on the focused toggle should retreat the index, got %d", scope.Index())
	}
}

func TestFormSelectRunsOnSubmit(t *testing.T) {
	submitted := false
	f, _ := sampleForm(func(o *FormOpts) {
		o.OnSubmit = func(*core.Shared, *FormScreen) core.Action { submitted = true; return core.Action{} }
	})
	f.Update(core.NewShared(nil), navKey(tea.KeyEnter))
	if !submitted {
		t.Error("Select on a plain field should run OnSubmit")
	}
}

func TestFormActivatorConsumesEnter(t *testing.T) {
	submitted := false
	pick := NewPickField("src", "Source", func() string { return "" },
		func(*core.Shared) (core.Action, bool) { return core.Pop(), true })
	f := NewForm(FormOpts{
		Fields:   []FormField{pick},
		OnSubmit: func(*core.Shared, *FormScreen) core.Action { submitted = true; return core.Action{} },
	})
	_, act := f.Update(core.NewShared(nil), navKey(tea.KeyEnter))
	if !reflect.DeepEqual(act, core.Pop()) {
		t.Errorf("an Activator consuming Enter should return its own Action, got %+v", act)
	}
	if submitted {
		t.Error("a consuming Activator should short-circuit OnSubmit")
	}
}

func TestFormBack(t *testing.T) {
	// Default Back pops.
	f, _ := sampleForm()
	_, act := f.Update(core.NewShared(nil), keyMsg("esc"))
	if !reflect.DeepEqual(act, core.Pop()) {
		t.Errorf("Back with no OnCancel should pop, got %+v", act)
	}

	// OnCancel wins when supplied.
	cancelled := false
	g, _ := sampleForm(func(o *FormOpts) {
		o.OnCancel = func(*core.Shared) core.Action { cancelled = true; return core.ResetToRoot() }
	})
	_, act = g.Update(core.NewShared(nil), keyMsg("esc"))
	if !cancelled || !reflect.DeepEqual(act, core.ResetToRoot()) {
		t.Errorf("Back should run OnCancel, got cancelled=%v act=%+v", cancelled, act)
	}
}

func TestFormTypingAndValueRoundTrip(t *testing.T) {
	f, _ := sampleForm()
	sh := core.NewShared(nil)
	f.Init(sh) // focus the first field so its input is focused

	// Typing feeds the focused text field.
	f.Update(sh, keyMsg("h"))
	f.Update(sh, keyMsg("i"))
	if got := f.Value("name"); got != "hi" {
		t.Fatalf("typed keys should reach the focused text field, Value = %q", got)
	}

	// SetValue / Value by key round-trip, and Focus moves focus by key.
	f.SetValue("url", "https://x")
	if got := f.Value("url"); got != "https://x" {
		t.Fatalf("SetValue/Value should round-trip, got %q", got)
	}
	f.Focus("url")
	if got := f.FocusedKey(); got != "url" {
		t.Fatalf("Focus(key) should move focus, got %q", got)
	}
	// A missing / non-text key is inert.
	if got := f.Value("missing"); got != "" {
		t.Errorf("Value of an absent key should be empty, got %q", got)
	}
}

func TestFormCrumbLabel(t *testing.T) {
	if got := NewForm(FormOpts{}).CrumbLabel(false); got != "Form" {
		t.Errorf("CrumbLabel default should be Form, got %q", got)
	}
	if got := NewForm(FormOpts{Crumb: "New"}).CrumbLabel(false); got != "New" {
		t.Errorf("CrumbLabel should use Crumb, got %q", got)
	}
	if got := NewForm(FormOpts{Crumb: "New", CrumbShort: "N"}).CrumbLabel(true); got != "N" {
		t.Errorf("CrumbLabel(short) should use CrumbShort, got %q", got)
	}
}
