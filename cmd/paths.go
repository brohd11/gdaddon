package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// resolveRoot resolves the Godot project root from the optional [project_root] arg,
// auto-detecting the git toplevel when omitted. It may prompt on stdin if the git root
// cannot be found; this runs before any TUI starts. The manifest itself is no longer
// resolved here — the TUI context scans for it under the root (see appctx.Ctx.Scan).
func resolveRoot(args []string) (projectRoot string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not get current working directory: %w", err)
	}

	if len(args) == 1 {
		projectRoot = args[0]
	} else {
		projectRoot = getGitDirectory()
	}

	if projectRoot == "" {
		fmt.Printf("Could not get git directory, use current dir instead? (%s) [y/N]: ", cwd)
		var input string
		fmt.Scan(&input)
		if strings.ToLower(input) == "y" {
			projectRoot = cwd
		} else {
			fmt.Println("Aborting.")
			os.Exit(0)
		}
	}

	return projectRoot, nil
}

func getGitDirectory() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
