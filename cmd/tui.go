package cmd

import (
	"gdutil/internal/tui"

	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui [yaml_path] [project_root]",
	Short: "Interactively browse and install Godot addons",
	Args:  cobra.RangeArgs(0, 2),
	RunE:  runTUI,
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

func runTUI(cmd *cobra.Command, args []string) error {
	yamlFile, projectRoot, err := resolvePaths(args)
	if err != nil {
		return err
	}
	return tui.Run(yamlFile, projectRoot)
}
