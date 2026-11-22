package cmd

import (
	"fmt"
	"os"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage the gdaddon companion editor plugin",
}

var pluginInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Download the latest gdaddon editor plugin into the current Godot project",
	Long: `Download the latest stable release ZIP of brohd11/gdaddon-editor-plugin
into addons/gdaddon/. Run this command in the directory containing project.godot;
it does not search parent directories or read or create an addon manifest.

Run it again to refresh the plugin independently of the gdaddon binary version.
Use --force to replace an unrelated folder or a newer installed plugin.
Enable gdaddon in Project Settings > Plugins after installation.`,
	Args: cobra.NoArgs,
	RunE: runPluginInstall,
}

func init() {
	pluginInstallCmd.Flags().Bool("force", false, "replace an unrelated destination or a newer installed plugin")
	pluginCmd.AddCommand(pluginInstallCmd)
	rootCmd.AddCommand(pluginCmd)
}

func runPluginInstall(cmd *cobra.Command, args []string) error {
	project, err := os.Getwd()
	if err != nil {
		return err
	}
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	report := func(format string, a ...any) {
		fmt.Fprintf(cmd.OutOrStdout(), format+"\n", a...)
	}
	res, err := addon.InstallEditorPlugin(cmd.Context(), project, force, report)
	if err != nil {
		return err
	}
	report("Installed gdaddon editor plugin %s in %s/%s", res.Version, project, res.Path)
	report("Enable \"gdaddon\" in Project Settings > Plugins (an enabled plugin reloads on its own).")
	return nil
}
