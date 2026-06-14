package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// resolvePaths turns the optional [yaml_path] [project_root] args into concrete
// paths, auto-detecting the manifest and git root when omitted. It may prompt
// on stdin if the git root cannot be found; this runs before any TUI starts.
func resolvePaths(args []string) (yamlFile, projectRoot string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("could not get current working directory: %w", err)
	}

	switch len(args) {
	case 0:
		yamlFile = filepath.Join(cwd, "addon_manifest.yml")
		projectRoot = getGitDirectory()
	case 1:
		yamlFile = args[0]
		projectRoot = getGitDirectory()
	case 2:
		yamlFile = args[0]
		projectRoot = args[1]
	}

	if _, err := os.Stat(yamlFile); os.IsNotExist(err) {
		return "", "", fmt.Errorf("YAML file not found at %s", yamlFile)
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

	return yamlFile, projectRoot, nil
}

func getGitDirectory() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
