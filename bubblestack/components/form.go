package components

import (
	"strings"

	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// FormScreen is the reusable, item-driven form: a column of self-rendering fields
// with one focused at a time. It owns only the generic key handling — field-focus
// cycling, the QueryUpdate typing split, Back/Left/Right/Select dispatch, and the
// titled box — while each field carries its own behavior, the same inversion as the
// self-dispatching Item list row (see internal/tui/doc.go). A tab/flow supplies the
// fields and an OnSubmit closure; FormScreen names no domain type.
//
// The field types (FormField + TextField/ToggleField/PickField/StaticField) and the
// optional interfaces (Toggler/Activator/editable) live in form_fields.go.

type FormOpts struct {
	Title      string // optional in-body title bar (core.WithTitle); omitted ⇒ no bar
	Crumb      string // breadcrumb segment (CrumbLabel); omitted ⇒ contributes none
	CrumbShort string // optional short breadcrumb-bar segment; defaults to Crumb
	Fields     []FormField
	Help       []key.Binding
	Focus      string // initial focused field key; default first focusable
	OnSubmit   func(*core.Shared, *FormScreen) core.Action
	OnCancel   func(*core.Shared) core.Action // Back handler; defaults to a plain Pop
	// OnKey claims extra keys before the form's default handling: it returns
	// (action, true) for a key it handles, or (_, false) to let the form process it
	// normally. Consumers must only claim non-text keys so editing still works.
	OnKey func(*core.Shared, string) (core.Action, bool)
}

type FormScreen struct {
	title      string
	crumb      string
	crumbShort string
	fields     []FormField
	help       []key.Binding
	focus      int
	onSubmit   func(*core.Shared, *FormScreen) core.Action
	onCancel   func(*core.Shared) core.Action
	onKey      func(*core.Shared, string) (core.Action, bool)
}

var _ core.Screen = (*FormScreen)(nil)
var _ core.Filterer = (*FormScreen)(nil)
var _ core.Crumber = (*FormScreen)(nil)
var _ Typable = (*FormScreen)(nil)

// CrumbLabel contributes the form's breadcrumb segment, defaulting to "Form" when no
// Crumb is declared.
func (f *FormScreen) CrumbLabel(short bool) string {
	return crumbSeg(short, f.crumbShort, f.crumb, "Form")
}

func NewForm(opts FormOpts) *FormScreen {
	f := &FormScreen{title: opts.Title, crumb: opts.Crumb, crumbShort: opts.CrumbShort, fields: opts.Fields, help: opts.Help, onSubmit: opts.OnSubmit, onCancel: opts.OnCancel, onKey: opts.OnKey}
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

func (f *FormScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	if cmd, ok := QueryUpdate(f, msg); ok {
		return f, core.Async(cmd)
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, core.Action{}
	}
	k := key.String()
	if f.onKey != nil {
		if act, handled := f.onKey(sh, k); handled {
			return f, act
		}
	}
	switch {
	case core.MatchKey(k, core.Keys.Back):
		if f.onCancel != nil {
			return f, f.onCancel(sh)
		}
		return f, core.Pop()
	case core.MatchKey(k, core.Keys.PrevField):
		f.move(-1)
		return f, core.Async(f.syncFocus())
	case core.MatchKey(k, core.Keys.NextField):
		f.move(1)
		return f, core.Async(f.syncFocus())
	case core.MatchKey(k, core.Keys.Left), core.MatchKey(k, core.Keys.Right):
		// On a Toggler row these cycle the option; on a text row they fall through
		// to the input (cursor movement / literal characters).
		if t, ok := f.current().(Toggler); ok {
			t.OnToggle(core.MatchKey(k, core.Keys.Right))
			return f, core.Action{}
		}
	case core.MatchKey(k, core.Keys.Select):
		if a, ok := f.current().(Activator); ok {
			if act, handled := a.OnSelect(sh); handled {
				return f, act
			}
		}
		if f.onSubmit != nil {
			return f, f.onSubmit(sh, f)
		}
		return f, core.Action{}
	}
	// Editing keys (backspace, cursor) fall through to the focused text field.
	if in := f.editable(); in != nil {
		var cmd tea.Cmd
		*in, cmd = in.Update(msg)
		return f, core.Async(cmd)
	}
	return f, core.Action{}
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
	return core.WithTitle(f.title, sh.Box(strings.Join(rows, "\n")))
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
