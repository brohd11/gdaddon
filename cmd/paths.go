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
		yamlFile, err = findManifest(cwd)
		if err != nil {
			return "", "", err
		}
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

// maxManifestDepth limits how deep findManifest descends from the start dir.
const maxManifestDepth = 5

// findManifest walks the tree rooted at start (up to maxManifestDepth dirs deep,
// including hidden dirs but skipping ".godot") looking for an addon manifest. It
// returns the path of the first match in a shallow-first traversal.
func findManifest(start string) (string, error) {
	names := map[string]bool{"addon_manifest.yml": true, "addon_manifest.yaml": true}

	var found string
	err := filepath.WalkDir(start, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			if path != start && d.Name() == ".godot" {
				return filepath.SkipDir
			}
			if depth(start, path) > maxManifestDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if names[d.Name()] {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error searching for manifest: %w", err)
	}
	if found == "" {
		return "", fmt.Errorf("no addon_manifest.yml found within %d directories of %s", maxManifestDepth, start)
	}
	return found, nil
}

// depth returns how many directory levels path is below base.
func depth(base, path string) int {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(rel, string(filepath.Separator)))
}

func getGitDirectory() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
