package components

import (
	"strings"

	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FormField is one row of a FormScreen. It renders its own row (marker + label +
// content) so the form just stacks them. Key is a stable identifier used by the
// form's Value/SetValue/Focus lookups; non-focusable rows (headings/notes/spacers)
// return false from Focusable and are skipped by field navigation.
type FormField interface {
	Key() string
	Focusable() bool
	Focus() tea.Cmd
	Blur()
	SetWidth(int)
	View(focused bool) string
}

// Toggler is a field that responds to Left/Right while focused (a multi-option
// switch). OnToggle moves the selection forward (right) or backward (left).
type Toggler interface{ OnToggle(forward bool) }

// Activator is a field that handles Enter itself instead of submitting the form
// (e.g. the search Source row, whose Enter opens a sub-picker). It returns an Action
// and whether it consumed the Enter; when not consumed the form runs its OnSubmit.
type Activator interface {
	OnSelect(*core.Shared) (core.Action, bool)
}

// editable is the (unexported) field capability QueryUpdate needs: a focused text
// field exposing its input. TextField satisfies it.
type editable interface{ Input() *textinput.Model }

// fieldBase carries the key + label every concrete field shares.
type fieldBase struct {
	key, label string
}

func (b fieldBase) Key() string { return b.key }

// fieldLabel is the muted style for a field's label. Built per call (not cached in
// a package var) so it tracks the active theme after a core.SetTheme switch.
func fieldLabel() lipgloss.Style { return lipgloss.NewStyle().Foreground(core.MutedColor) }

// fieldMarker is the focus arrow rendered to the left of a focusable row.
func fieldMarker(focused bool) string {
	if focused {
		return lipgloss.NewStyle().Foreground(core.FocusedColor).Render("▸ ")
	}
	return "  "
}

// ---------- TextField ----------

// TextField is a free-text row backed by a textinput.Model. It satisfies editable,
// so the form routes typed characters here via QueryUpdate.
type TextField struct {
	fieldBase
	input textinput.Model
}

func NewTextField(key, label, placeholder string) *TextField {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = "" // the label is rendered separately
	return &TextField{fieldBase: fieldBase{key, label}, input: ti}
}

func (t *TextField) Focusable() bool         { return true }
func (t *TextField) Focus() tea.Cmd          { return t.input.Focus() }
func (t *TextField) Blur()                   { t.input.Blur() }
func (t *TextField) SetWidth(w int)          { t.input.Width = w }
func (t *TextField) Input() *textinput.Model { return &t.input }
func (t *TextField) Value() string           { return t.input.Value() }
func (t *TextField) SetValue(v string)       { t.input.SetValue(v) }
func (t *TextField) View(focused bool) string {
	return fieldMarker(focused) + fieldLabel().Render(t.label) + t.input.View()
}

// ---------- ToggleField ----------

// ToggleField is a multi-option switch (e.g. Project/Global). OnToggle cycles the
// index, so it works for any number of options; delim controls how RenderToggle
// joins them (empty → the ◄ ► arrows).
type ToggleField struct {
	fieldBase
	options []string
	index   int
	delim   string
}

// NewToggleField builds a multi-option switch. delim is optional: omit it for the
// default ◄ ► arrows, or pass one (e.g. "|") to join the options differently.
func NewToggleField(key, label string, options []string, delim ...string) *ToggleField {
	t := &ToggleField{fieldBase: fieldBase{key, label}, options: options}
	if len(delim) > 0 {
		t.delim = delim[0]
	}
	return t
}

func (t *ToggleField) Focusable() bool { return true }
func (t *ToggleField) Focus() tea.Cmd  { return nil }
func (t *ToggleField) Blur()           {}
func (t *ToggleField) SetWidth(int)    {}
func (t *ToggleField) Index() int      { return t.index }
func (t *ToggleField) Value() string   { return t.options[t.index] }

// SetIndex pre-selects an option (e.g. to seed a toggle from detected state). Out-of-
// range values are ignored, so it's safe to call before options are known to match.
func (t *ToggleField) SetIndex(i int) {
	if i >= 0 && i < len(t.options) {
		t.index = i
	}
}

func (t *ToggleField) OnToggle(forward bool) {
	n := len(t.options)
	if forward {
		t.index = (t.index + 1) % n
	} else {
		t.index = (t.index - 1 + n) % n
	}
}

func (t *ToggleField) View(focused bool) string {
	return fieldMarker(focused) + fieldLabel().Render(t.label) + RenderToggle(t.options, t.index, t.delim)
}

// RenderToggle renders a multi-option switch with the active option highlighted,
// joining the options with delim (empty → the "◄ ►" arrows). Pure rendering — the
// cycling lives in the caller (ToggleField.OnToggle), so it works for any option
// count. Reused by ToggleField and the New Plugin confirm screen.
func RenderToggle(options []string, index int, delim string) string {
	active := lipgloss.NewStyle().Foreground(core.FocusedColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(core.MutedColor)
	parts := make([]string, len(options))
	for i, o := range options {
		if i == index {
			parts[i] = active.Render(o)
		} else {
			parts[i] = dim.Render(o)
		}
	}
	sep := "  ◄ ►  "
	if delim != "" {
		sep = " " + delim + " "
	}
	return strings.Join(parts, sep)
}

// ---------- PickField ----------

// PickField is a focusable row whose Enter runs a custom action (an Activator) — used
// for the search Source row, whose value is chosen in a pushed sub-picker. value
// supplies the current display text; onSel runs on Enter.
type PickField struct {
	fieldBase
	value func() string
	onSel func(*core.Shared) (core.Action, bool)
}

func NewPickField(key, label string, value func() string, onSel func(*core.Shared) (core.Action, bool)) *PickField {
	return &PickField{fieldBase: fieldBase{key, label}, value: value, onSel: onSel}
}

func (p *PickField) Focusable() bool                              { return true }
func (p *PickField) Focus() tea.Cmd                               { return nil }
func (p *PickField) Blur()                                        {}
func (p *PickField) SetWidth(int)                                 {}
func (p *PickField) OnSelect(sh *core.Shared) (core.Action, bool) { return p.onSel(sh) }
func (p *PickField) View(focused bool) string {
	return fieldMarker(focused) + fieldLabel().Render(p.label) + p.value()
}

// ---------- StaticField ----------

// StaticField is a non-focusable display row: a heading, a muted note, or a blank
// spacer. Field navigation skips it.
type StaticField struct {
	text  string
	style lipgloss.Style
}

func NewHeading(text string) *StaticField { return &StaticField{text: text} }
func NewNote(text string) *StaticField    { return &StaticField{text: text, style: fieldLabel()} }
func NewSpacer() *StaticField             { return &StaticField{} }

func (s *StaticField) Key() string      { return "" }
func (s *StaticField) Focusable() bool  { return false }
func (s *StaticField) Focus() tea.Cmd   { return nil }
func (s *StaticField) Blur()            {}
func (s *StaticField) SetWidth(int)     {}
func (s *StaticField) View(bool) string { return s.style.Render(s.text) }
