package cmd

import (
	"fmt"

	"gdutil/internal/addon"

	"github.com/spf13/cobra"
)

var addonInstallCmd = &cobra.Command{
	Use:   "addon_install [yaml_path] [project_root]",
	Short: "Install Godot addons from a YAML manifest",
	Args:  cobra.RangeArgs(0, 2),
	RunE:  runAddonInstall,
}

func init() {
	rootCmd.AddCommand(addonInstallCmd)
}

func runAddonInstall(cmd *cobra.Command, args []string) error {
	yamlFile, projectRoot, err := resolvePaths(args)
	if err != nil {
		return err
	}

	statuses, err := addon.Inspect(yamlFile, projectRoot)
	if err != nil {
		return err
	}
	if len(statuses) == 0 {
		fmt.Println("No addons found in YAML.")
		return nil
	}

	report := func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	return addon.InstallAll(statuses, projectRoot, report)
}
