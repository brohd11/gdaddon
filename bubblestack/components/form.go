package components

import (
	"strings"

	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FormScreen is the reusable, item-driven form: a column of self-rendering fields
// with one focused at a time. It owns only the generic key handling — field-focus
// cycling, the QueryUpdate typing split, Back/Left/Right/Select dispatch, and the
// titled box — while each field carries its own behavior, the same inversion as the
// self-dispatching Item list row (see internal/tui/doc.go). A tab/flow supplies the
// fields and an OnSubmit closure; FormScreen names no domain type.
//
// Fields implement FormField; a field opts into extra behavior by also implementing
// one of the optional interfaces the form type-asserts: Toggler (Left/Right while
// focused), Activator (Enter handled by the field itself, e.g. opening a sub-picker),
// or the editable interface{ Input() *textinput.Model } (free-text, fed by QueryUpdate).

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
// (e.g. the search Source row, whose Enter opens a sub-picker). It returns the
// command to run and whether it consumed the Enter; when not consumed the form runs
// its OnSubmit.
type Activator interface {
	OnSelect(*core.Shared) (tea.Cmd, bool)
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
	onSel func(*core.Shared) (tea.Cmd, bool)
}

func NewPickField(key, label string, value func() string, onSel func(*core.Shared) (tea.Cmd, bool)) *PickField {
	return &PickField{fieldBase: fieldBase{key, label}, value: value, onSel: onSel}
}

func (p *PickField) Focusable() bool                          { return true }
func (p *PickField) Focus() tea.Cmd                           { return nil }
func (p *PickField) Blur()                                    {}
func (p *PickField) SetWidth(int)                             {}
func (p *PickField) OnSelect(sh *core.Shared) (tea.Cmd, bool) { return p.onSel(sh) }
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

// ---------- FormScreen ----------

type FormOpts struct {
	Crumb    string // e.g. core.RenderTitleBar("New Plugin")
	Fields   []FormField
	Help     []key.Binding
	Focus    string // initial focused field key; default first focusable
	OnSubmit func(*core.Shared, *FormScreen) tea.Cmd
}

type FormScreen struct {
	crumb    string
	fields   []FormField
	help     []key.Binding
	focus    int
	onSubmit func(*core.Shared, *FormScreen) tea.Cmd
}

var _ core.Screen = (*FormScreen)(nil)
var _ core.Filterer = (*FormScreen)(nil)
var _ Typable = (*FormScreen)(nil)

func NewForm(opts FormOpts) *FormScreen {
	f := &FormScreen{crumb: opts.Crumb, fields: opts.Fields, help: opts.Help, onSubmit: opts.OnSubmit}
	f.focus = f.firstFocusable()
	if opts.Focus != "" {
		for i, fld := range f.fields {
			if fld.Key() == opts.Focus && fld.Focusable() {
				f.focus = i
				break
			}
		}
	}
	return f
}

func (f *FormScreen) firstFocusable() int {
	for i, fld := range f.fields {
		if fld.Focusable() {
			return i
		}
	}
	return 0
}

func (f *FormScreen) current() FormField { return f.fields[f.focus] }

// editable returns the focused field's text input, or nil if it isn't a text field.
func (f *FormScreen) editable() *textinput.Model {
	if e, ok := f.current().(editable); ok {
		return e.Input()
	}
	return nil
}

// Typable: a free-text field has focus iff the current field exposes an input.
func (f *FormScreen) Typing() bool              { return f.editable() != nil }
func (f *FormScreen) Input() *textinput.Model   { return f.editable() }
func (f *FormScreen) Filtering() bool           { return true }
func (f *FormScreen) Init(*core.Shared) tea.Cmd { return f.syncFocus() }

func (f *FormScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
	if cmd, ok := QueryUpdate(f, msg); ok {
		return f, cmd
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, nil
	}
	k := key.String()
	switch {
	case core.MatchKey(k, core.Keys.Back):
		return f, core.Pop()
	case core.MatchKey(k, core.Keys.PrevField):
		f.move(-1)
		return f, f.syncFocus()
	case core.MatchKey(k, core.Keys.NextField):
		f.move(1)
		return f, f.syncFocus()
	case core.MatchKey(k, core.Keys.Left), core.MatchKey(k, core.Keys.Right):
		// On a Toggler row these cycle the option; on a text row they fall through
		// to the input (cursor movement / literal characters).
		if t, ok := f.current().(Toggler); ok {
			t.OnToggle(core.MatchKey(k, core.Keys.Right))
			return f, nil
		}
	case core.MatchKey(k, core.Keys.Select):
		if a, ok := f.current().(Activator); ok {
			if cmd, handled := a.OnSelect(sh); handled {
				return f, cmd
			}
		}
		if f.onSubmit != nil {
			return f, f.onSubmit(sh, f)
		}
		return f, nil
	}
	// Editing keys (backspace, cursor) fall through to the focused text field.
	if in := f.editable(); in != nil {
		var cmd tea.Cmd
		*in, cmd = in.Update(msg)
		return f, cmd
	}
	return f, nil
}

// move shifts focus by delta, skipping non-focusable fields and wrapping around.
func (f *FormScreen) move(delta int) {
	n := len(f.fields)
	for i := 1; i <= n; i++ {
		j := ((f.focus+delta*i)%n + n) % n
		if f.fields[j].Focusable() {
			f.focus = j
			return
		}
	}
}

// syncFocus focuses the current field and blurs the rest, returning the focused
// field's command (the cursor blink for a text field).
func (f *FormScreen) syncFocus() tea.Cmd {
	var cmd tea.Cmd
	for i, fld := range f.fields {
		if i == f.focus {
			cmd = fld.Focus()
		} else {
			fld.Blur()
		}
	}
	return cmd
}

// field looks up a field by key (nil if none).
func (f *FormScreen) field(key string) FormField {
	for _, fld := range f.fields {
		if fld.Key() == key {
			return fld
		}
	}
	return nil
}

// Value reads a text field's value by key ("" if the key is absent or not text).
func (f *FormScreen) Value(key string) string {
	if t, ok := f.field(key).(*TextField); ok {
		return t.Value()
	}
	return ""
}

// SetValue sets a text field's value by key (no-op if absent or not text).
func (f *FormScreen) SetValue(key, v string) {
	if t, ok := f.field(key).(*TextField); ok {
		t.SetValue(v)
	}
}

// Focus moves focus to the field with the given key, returning its focus command.
func (f *FormScreen) Focus(key string) tea.Cmd {
	for i, fld := range f.fields {
		if fld.Key() == key {
			f.focus = i
			return f.syncFocus()
		}
	}
	return nil
}

// FocusedKey is the key of the currently focused field.
func (f *FormScreen) FocusedKey() string { return f.current().Key() }

func (f *FormScreen) View(sh *core.Shared) string {
	rows := make([]string, len(f.fields))
	for i, fld := range f.fields {
		rows[i] = fld.View(i == f.focus)
	}
	return lipgloss.JoinVertical(lipgloss.Left, f.crumb, sh.Box(strings.Join(rows, "\n")))
}

func (f *FormScreen) HelpView(sh *core.Shared) string { return sh.BindingHelp(f.help) }

func (f *FormScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	w := sh.ConfirmWidth() - 12 // box room minus the label column
	if w < 10 {
		w = 10
	}
	for _, fld := range f.fields {
		fld.SetWidth(w)
	}
}
