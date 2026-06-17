// Package bubblestack is the consumer-facing entry point to the TUI framework: a
// thin facade over core that hides the Shared/Router/bubbletea wiring behind a
// single Run call. A consumer supplies only its own context, optional header/output
// chrome, theme, and tabs; everything else is constructed here.
//
// This is deliberately a small surface. The deeper API — navigation commands,
// chrome/style helpers, reusable screens — still lives in core and components and
// is imported directly; only the few names the entry point touches are re-exported
// below.
package bubblestack

import (
	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// Re-exported as aliases (not new types) so a consumer can build these at the call
// site without importing core, while screens still satisfy core.Screen unchanged.
type (
	Shared     = core.Shared
	TabEntry   = core.TabEntry
	Screen     = core.Screen
	Output     = core.Output
	Status     = core.Status
	ChromeMask = core.ChromeMask
)

// FullscreenMask re-exports core.FullscreenMask so a consumer's screen can return it
// from ChromeMask() to claim the whole canvas without importing core.
var FullscreenMask = core.FullscreenMask

// Config is the consumer-supplied input to Run. App and Tabs are required; Header,
// Output, Status, and Theme are optional. A nil Header ⇒ no header box; a nil Output ⇒
// no output pane (pass components.NewLogPane() for the default scrollable log); a nil
// Status ⇒ no status line (pass components.NewStatusLine() for the default).
type Config struct {
	App    any                       // consumer context, recovered via core.App[T]
	Header func(*core.Shared) string // persistent context box (nil ⇒ none)
	Output core.Output               // below-body pane (nil ⇒ none)
	Status core.Status               // transient status line (nil ⇒ none)
	Tabs   []core.TabEntry           // top-level tabs
	Theme  string                    // named theme; empty ⇒ leave the default
}

// Run builds the chrome from the config, applies the theme, wires the router over
// the tabs, and blocks on the bubbletea program until the user quits.
func Run(cfg Config) error {
	sh := core.NewShared(cfg.App)
	sh.Chrome = &core.Chrome{Output: cfg.Output, Status: cfg.Status}
	if cfg.Header != nil {
		sh.Chrome.Header = core.NewHeaderPane(cfg.Header)
	}
	if cfg.Theme != "" {
		core.SetTheme(cfg.Theme)
	}
	r := core.NewRouter(sh, cfg.Tabs)
	_, err := tea.NewProgram(r, tea.WithAltScreen()).Run()
	return err
}
