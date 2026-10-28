// Package editversion provides the one-field addon config version editor.
package editversion

import (
	"path/filepath"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gitstack/repo"

	"charm.land/bubbles/v2/key"
)

// New reads the config when the editor opens, rather than using a cached list version.
func New(dir, label string) (*components.FormScreen, error) {
	cfg, err := addon.ReadConfigVersion(dir)
	if err != nil {
		return nil, err
	}
	current := cfg.Version
	if current == "" {
		current = "(no version)"
	}
	input := components.NewTextField("version", current+" → ", "1.0.0")
	next := repo.NextTag([]string{cfg.Version}, nil)
	if next == "" {
		next = cfg.Version
	}
	input.SetValue(next)
	return components.NewForm(components.FormOpts{
		Crumb: "Edit Version/Tag",
		Fields: []components.FormField{
			components.NewHeading(label + " · " + filepath.Base(cfg.Path)),
			components.NewSpacer(),
			input,
		},
		Focus: "version",
		Help: []key.Binding{
			core.Hint("save", core.Keys.Select),
			core.Hint("cancel", core.Keys.Back),
		},
		OnSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			if err := addon.SetConfigVersion(cfg.Path, cfg.Version, f.Value("version")); err != nil {
				return core.StatusErr(err)
			}
			return core.Seq(
				core.SetStatusAndLog(label+": version updated"),
				core.PropagateAll(appctx.ProjectDirty{}),
				core.Pop(2),
			)
		},
	}), nil
}
