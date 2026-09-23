package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/brohd11/gitstack/repo"
)

// resolveRoot resolves the Godot project root for the TUI: the optional [project_root]
// arg, else the git toplevel, else (after asking on stdin) the current directory. An
// empty root with a nil error means the user declined.
func resolveRoot(args []string) (string, error) {
	if len(args) == 1 {
		return filepath.Abs(args[0])
	}
	if root, ok := repo.RepoRoot("."); ok {
		return root, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not get current working directory: %w", err)
	}
	fmt.Printf("Could not get git directory, use current dir instead? (%s) [y/N]: ", cwd)
	var input string
	fmt.Scan(&input)
	if strings.ToLower(input) != "y" {
		fmt.Println("Aborting.")
		return "", nil
	}
	return cwd, nil
}

// resolveRootArg is resolveRootQuiet for an optional positional [project_root].
func resolveRootArg(args []string) (string, error) {
	override := ""
	if len(args) == 1 {
		override = args[0]
	}
	return resolveRootQuiet(override)
}

// resolveRootQuiet is resolveRoot for scriptable subcommands: it never prompts, falling
// back to the current directory. Callers should report the root they got.
func resolveRootQuiet(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	if root, ok := repo.RepoRoot("."); ok {
		return root, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not get current working directory: %w", err)
	}
	return cwd, nil
}
