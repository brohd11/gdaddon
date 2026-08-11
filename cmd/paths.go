package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// resolveRootArg resolves the project root from an optional positional [project_root]
// argument, the shape the root command and the project-scoped subcommands share. It
// never prompts (see resolveRootQuiet); `install` takes its root as --root instead,
// because its positional slot holds the repo spec.
func resolveRootArg(args []string) (string, error) {
	override := ""
	if len(args) == 1 {
		override = args[0]
	}
	return resolveRootQuiet(override)
}

// resolveRootQuiet resolves the project root without ever prompting or exiting: the
// explicit override when given, else the git toplevel, else the current directory.
// Where resolveRoot asks the user what to do about a missing git root (fine before a
// TUI), a scriptable subcommand must just pick something and say what it picked — so
// callers should report the resolved root.
func resolveRootQuiet(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	if root := getGitDirectory(); root != "" {
		return root, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not get current working directory: %w", err)
	}
	return cwd, nil
}

func getGitDirectory() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
