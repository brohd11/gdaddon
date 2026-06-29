package cmd

import (
	"fmt"

	"gdaddon/internal/installer"

	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the gdaddon binary from all install locations",
	Long: `Uninstall removes the gdaddon binary from every location 'install' uses
(system, user, and ~/.gdaddon/bin), wherever it is present. It touches only the
binary — PATH entries and other ~/.gdaddon files are left alone. Removing the
system copy may prompt for sudo.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: false,
	RunE:          runUninstallCmd,
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstallCmd(cmd *cobra.Command, args []string) error {
	removed, skipped, err := installer.Uninstall()
	if err != nil {
		return err
	}
	if len(removed) == 0 && len(skipped) == 0 {
		fmt.Println("nothing to remove — no installed gdaddon binary found")
		return nil
	}
	for _, p := range removed {
		fmt.Printf("removed %s\n", p)
	}
	for _, p := range skipped {
		fmt.Printf("skipped %s (currently running)\n", p)
	}
	return nil
}
