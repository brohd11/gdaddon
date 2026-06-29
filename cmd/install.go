package cmd

import (
	"fmt"
	"os"

	"gdaddon/internal/config"
	"gdaddon/internal/installer"

	"github.com/brohd11/bubblestack"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var installDest string

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Copy this gdaddon binary to a chosen location (and wire up PATH)",
	Long: `Install copies the running gdaddon binary to one of:

  system   on PATH by default, needs sudo/admin
  user     ~/.local/bin / %LOCALAPPDATA%\Programs\gdaddon, sets up PATH
  home     ~/.gdaddon/bin, not on PATH (the target the Godot plugin launches)

With no flags it opens a menu to pick the destination; pass --dest for a
non-interactive install.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: false,
	RunE:          runInstallCmd,
}

func init() {
	installCmd.Flags().StringVar(&installDest, "dest", "", "non-interactive destination: system|user|home")
	rootCmd.AddCommand(installCmd)
}

func runInstallCmd(cmd *cobra.Command, args []string) error {
	src, err := installer.Self()
	if err != nil {
		return err
	}

	var dest installer.Dest
	if installDest != "" {
		dest, err = installer.ParseDest(installDest)
		if err != nil {
			return err
		}
	} else {
		if !isatty.IsTerminal(os.Stdin.Fd()) || !isatty.IsTerminal(os.Stdout.Fd()) {
			return fmt.Errorf("not a terminal; pass --dest system|user|home")
		}
		d, ok, err := pickDest(src)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("cancelled")
			return nil
		}
		dest = d
	}

	// The privileged copy / PATH wiring runs here, after the TUI has released the
	// terminal — so sudo can prompt and output prints to normal scrollback.
	res, err := installer.Install(dest)
	if err != nil {
		return err
	}
	fmt.Printf("installed to %s\n", res.Path)
	if res.Note != "" {
		fmt.Println(res.Note)
	}
	return nil
}

// pickDest runs a small bubbletea menu (reusing the app's bubblestack stack) that
// only selects a destination; it performs no filesystem work. Returns the choice
// and whether the user confirmed (false on Cancel / ctrl+c).
func pickDest(src string) (installer.Dest, bool, error) {
	var chosen installer.Dest
	var ok bool

	theme := "mono"
	if cfg, err := config.Load(); err == nil && cfg.CurrentTheme != "" {
		theme = cfg.CurrentTheme
	}

	items := make([]list.Item, 0, 4)
	for _, opt := range installer.Dests() {
		o := opt // capture
		items = append(items, components.Item{
			Name: o.Label,
			Desc: o.Desc,
			Pick: func(sh *core.Shared) core.Action {
				chosen, ok = o.Dest, true
				return core.Async(tea.Quit)
			},
		})
	}
	items = append(items, components.Item{
		Name: "Cancel",
		Desc: "Exit without installing",
		Pick: func(sh *core.Shared) core.Action {
			ok = false
			return core.Async(tea.Quit)
		},
	})

	header := func(*core.Shared) string {
		return fmt.Sprintf("gdaddon %s — choose where to install\nsource: %s", version, src)
	}

	err := bubblestack.Run(bubblestack.Config{
		App:    struct{}{},
		Header: header,
		Theme:  theme,
		Tabs: []bubblestack.TabEntry{
			{Title: "Install", New: func(sh *bubblestack.Shared) bubblestack.Screen {
				return components.NewPicker(items, components.PickerOpts{Title: "Install gdaddon"})
			}},
		},
	})
	return chosen, ok, err
}
