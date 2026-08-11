package cmd

import (
	"context"
	"fmt"

	"gdaddon/internal/addon"

	"github.com/spf13/cobra"
)

var updateAddonsCmd = &cobra.Command{
	Use:   "update-addons [project_root]",
	Short: "Update the project's installed addons to their latest release",
	Long: `Update-addons resolves an update plan for every installed addon with a newer
release and installs them, reporting progress as it goes.

A locked entry is never updated, and an addon whose release ships several packages is
skipped with a note (there is no way to pick between them without a user).

The project root defaults to the git toplevel, else the current directory.

This updates Godot addons. To update the gdaddon binary itself, see 'gdaddon update'.`,
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: false,
	RunE:          runUpdateAddons,
}

func init() {
	rootCmd.AddCommand(updateAddonsCmd)
}

func runUpdateAddons(cmd *cobra.Command, args []string) error {
	projectRoot, err := resolveRootArg(args)
	if err != nil {
		return err
	}
	manifest, err := discoverManifest(projectRoot)
	if err != nil {
		return err
	}
	plans, skipped, err := addon.ResolveUpdatePlans(context.Background(), manifest, projectRoot)
	if err != nil {
		return err
	}
	for _, s := range skipped {
		fmt.Printf("Skipping %s (%s): multiple packages — update manually.\n", s.Name, s.Tag)
	}
	if len(plans) == 0 {
		if len(skipped) == 0 {
			fmt.Println("All installed addons are up to date.")
		}
		return nil
	}
	report := func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	_, err = addon.UpdateAll(context.Background(), manifest, plans, projectRoot, report)
	return err
}
