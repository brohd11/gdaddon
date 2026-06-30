package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"gdaddon/internal/installer"
	"gdaddon/internal/selfupdate"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var (
	selfUpdateCheck       bool
	selfUpdateJSON        bool
	selfUpdateInteractive bool
)

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Check for a newer gdaddon release and install it over this binary",
	Long: `Self-update compares this binary's version against the latest gdaddon
release. With no flags it downloads and installs the update when one is available
(to wherever this binary is installed, or ~/.gdaddon/bin otherwise).

  --check        only report whether an update is available; don't install
  --json         with --check, print the result as JSON (for the Godot plugin)
  --interactive  choose the install location via a menu instead of the default`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: false,
	RunE:          runSelfUpdate,
}

func init() {
	selfUpdateCmd.Flags().BoolVar(&selfUpdateCheck, "check", false, "only check for an update; don't download or install")
	selfUpdateCmd.Flags().BoolVar(&selfUpdateJSON, "json", false, "with --check, print the result as JSON")
	selfUpdateCmd.Flags().BoolVarP(&selfUpdateInteractive, "interactive", "i", false, "choose the install location via a menu")
	rootCmd.AddCommand(selfUpdateCmd)
}

// selfUpdateJSONOut is the stable machine-parseable shape of a self-update check,
// mirroring the --list --json convention for the GDScript side.
type selfUpdateJSONOut struct {
	Current   string `json:"current"`
	LatestTag string `json:"latest_tag"`
	Available bool   `json:"available"`
}

func runSelfUpdate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	info, err := selfupdate.Check(ctx, version)
	if err != nil {
		return err
	}

	if selfUpdateCheck {
		if selfUpdateJSON {
			out, err := json.Marshal(selfUpdateJSONOut{
				Current:   info.Current,
				LatestTag: info.LatestTag,
				Available: info.Available,
			})
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			return nil
		}
		if info.Available {
			fmt.Printf("gdaddon update available: %s → %s\n", info.Current, info.LatestTag)
		} else {
			fmt.Printf("gdaddon %s is up to date.\n", info.Current)
		}
		return nil
	}

	if !info.Available {
		fmt.Printf("gdaddon %s is up to date.\n", info.Current)
		return nil
	}

	dest := selfupdate.DefaultDest()
	if selfUpdateInteractive && isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd()) {
		src, err := installer.Self()
		if err != nil {
			return err
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

	report := func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	path, err := selfupdate.Apply(ctx, info, dest, report)
	if err != nil {
		return err
	}
	fmt.Printf("updated to %s at %s\n", info.LatestTag, path)
	return nil
}
