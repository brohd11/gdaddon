package core

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
)

// Shared holds the cross-cutting state owned by the router: the consumer's own
// context (App), terminal size, the spinner, a help model for rendering static help
// bars, the in-flight task channel, and the optional Chrome (header/status/output —
// see chrome.go). A single instance is created in NewShared and pointed at by the
// router; screens receive it as a method argument. The framework names no domain
// type: App carries whatever struct the consumer wants (recover it typed with
// App[T]); the header renderer and output pane ride on Chrome.
type Shared struct {
	App    any     // consumer-owned context; recover it with App[T]
	Chrome *Chrome // optional header/status/output furniture (nil ⇒ fullscreen)

	width  int
	height int

	Spinner spinner.Model
	help    help.Model // renders static (non-list) help bars

	Events chan TaskEvent // the in-flight streaming task channel
}

func NewShared(app any) *Shared {
	sp := spinner.New()
	sp.Spinner = spinner.Points

	h := help.New()

	return &Shared{
		App:     app,
		Spinner: sp,
		help:    h,
	}
}

// Log appends a line to the output pane when one is present and supports logging
// (the default LogPane does), and is a no-op for a chromeless app or a non-logging
// Output — so context-agnostic callers (e.g. the task screen) needn't know the pane
// type or whether chrome exists.
func (s *Shared) Log(line string, forceShow ...bool) {
	if s.Chrome == nil || s.Chrome.Output == nil {
		return
	}
	if l, ok := s.Chrome.Output.(interface{ Log(string, bool) }); ok {
		force := GetOptional(true, forceShow...)
		l.Log(line, force)
	}
}

// WriteStatus sets the transient status line. Meant to be used with core.SetStatus,
// where a timer will be started to clear after 5s. No-op without chrome.
// variadic params: log=false forceShow=false
func (s *Shared) WriteStatus(line string, logParams ...bool) {
	if s.Chrome == nil {
		return
	}
	if s.Chrome.Status != nil {
		s.Chrome.Status.Set(line) // Set("") clears (Shown() == false)
	}
	var log = GetOptionalIdx(false, 0, logParams...)
	if log && line != "" {
		var forceShow = GetOptionalIdx(false, 1, logParams...)
		s.Log(line, forceShow)
	}
}

func (s *Shared) ClearStatus() {
	if s.Chrome != nil && s.Chrome.Status != nil {
		s.Chrome.Status.Clear()
	}
}

// SetStatus sets the status line only (no log) — the thin wrapper the existing call
// sites use. Migrate selected sites to WriteStatus(line, true) to also log the line.
func (s *Shared) SetStatus(msg string) { s.WriteStatus(msg) }

// App recovers the consumer's context from a Shared, type-asserted to *T. The
// consumer stores a *T in NewShared and reads it back here, so the framework stays
// domain-agnostic while tabs get a typed handle: c := core.App[MyCtx](sh).
func App[T any](s *Shared) *T { return s.App.(*T) }

// Width reports the current terminal width, so a Header closure can size/truncate
// its content to fit (see HeaderInnerWidth).
func (s *Shared) Width() int { return s.width }
