package cmd

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
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
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not get current working directory: %w", err)
	}

	var yamlFile, projectRoot string

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
		return fmt.Errorf("YAML file not found at %s", yamlFile)
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

	return installAddons(yamlFile, projectRoot)
}

// ---------- install logic ----------

type addon struct {
	URL     string `yaml:"url"`
	Path    string `yaml:"path"`
	Version string `yaml:"version"`
}

func installAddons(yamlPath, baseDir string) error {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return fmt.Errorf("could not read %s: %w", yamlPath, err)
	}

	var addons map[string]addon
	if err := yaml.Unmarshal(data, &addons); err != nil {
		return fmt.Errorf("could not parse YAML: %w", err)
	}

	if len(addons) == 0 {
		fmt.Println("No addons found in YAML.")
		return nil
	}

	for addonName, a := range addons {
		if a.URL == "" || a.Path == "" {
			fmt.Printf("Skipping %s: missing 'url' or 'path'\n", addonName)
			continue
		}

		fullPath, err := filepath.Abs(filepath.Join(baseDir, a.Path))
		if err != nil {
			fmt.Printf("Skipping %s: could not resolve path: %v\n", addonName, err)
			continue
		}

		if _, err := os.Stat(fullPath); err == nil {
			if a.Version != "" {
				localVersion := getLocalPluginVersion(fullPath)
				if localVersion == a.Version {
					fmt.Printf("[%s] v%s is already installed. Skipping...\n", addonName, localVersion)
					continue
				}
				oldVer := localVersion
				if oldVer == "" {
					oldVer = "Unknown/None"
				}
				fmt.Printf("[%s] Version mismatch! Local is %s, YAML wants %s. Updating...\n", addonName, oldVer, a.Version)
				os.RemoveAll(fullPath)
			} else {
				fmt.Printf("[%s] already exists at %s (no version specified). Skipping...\n", addonName, a.Path)
				continue
			}
		}

		switch {
		case strings.HasSuffix(a.URL, ".zip"):
			if err := downloadAndExtractZip(a.URL, fullPath, addonName); err != nil {
				fmt.Printf("[%s] Error: %v\n", addonName, err)
			}
		case strings.HasSuffix(a.URL, ".git"):
			if err := cloneGitRepo(a.URL, fullPath, addonName, a.Path); err != nil {
				fmt.Printf("[%s] Error: %v\n", addonName, err)
			}
		default:
			fmt.Printf("[%s] Error: URL must end in '.zip' or '.git'. Found: %s\n", addonName, a.URL)
		}
	}

	return nil
}

func getLocalPluginVersion(addonPath string) string {
	cfgPath := filepath.Join(addonPath, "plugin.cfg")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return ""
	}

	cfg, err := ini.Load(cfgPath)
	if err != nil {
		fmt.Printf("  Warning: could not parse %s (%v)\n", cfgPath, err)
		return ""
	}

	section := cfg.Section("plugin")
	if section == nil {
		return ""
	}

	key := section.Key("version")
	if key == nil {
		return ""
	}

	return strings.Trim(key.String(), `'"`)
}

func downloadAndExtractZip(url, targetPath, addonName string) error {
	fmt.Printf("[%s] Downloading ZIP from %s...\n", addonName, url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tmpFile, err := os.CreateTemp("", "godot-addon-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return err
	}
	tmpFile.Close()

	extractDir, err := os.MkdirTemp("", "godot-addon-extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(extractDir)

	fmt.Printf("[%s] Extracting...\n", addonName)
	if err := unzip(tmpFile.Name(), extractDir); err != nil {
		return err
	}

	targetFolderName := filepath.Base(targetPath)
	foundSource := ""

	filepath.Walk(extractDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || foundSource != "" {
			return nil
		}
		if info.IsDir() && info.Name() == targetFolderName {
			foundSource = path
		}
		return nil
	})

	if foundSource != "" {
		if err := copyDir(foundSource, targetPath); err != nil {
			return err
		}
	} else {
		entries, err := os.ReadDir(extractDir)
		if err != nil {
			return err
		}
		if len(entries) == 1 && entries[0].IsDir() {
			if err := copyDir(filepath.Join(extractDir, entries[0].Name()), targetPath); err != nil {
				return err
			}
		} else {
			if err := copyDir(extractDir, targetPath); err != nil {
				return err
			}
		}
	}

	fmt.Printf("  -> Successfully installed to %s\n", targetPath)
	return nil
}

func cloneGitRepo(url, fullPath, addonName, targetDir string) error {
	fmt.Printf("[%s] Cloning repository...\n", addonName)

	tempDir := fullPath + "_temp"
	os.RemoveAll(tempDir)

	cmd := exec.Command("git", "clone", "--depth", "1", url, tempDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("  -> Failed to clone %s:\n%s\n", addonName, string(out))
		return err
	}

	addonsPath := filepath.Join(tempDir, "addons")

	if _, err := os.Stat(addonsPath); os.IsNotExist(err) {
		fmt.Println("  -> No 'addons' folder detected. Installing full repository...")
		os.RemoveAll(fullPath)
		if err := os.Rename(tempDir, fullPath); err != nil {
			return err
		}
		fmt.Printf("  -> Successfully installed to %s\n", fullPath)
	} else {
		var sourceDir string
		if targetDir != "" {
			sourceDir = filepath.Clean(filepath.Join(tempDir, targetDir))
		} else {
			sourceDir = tempDir
		}

		if _, err := os.Stat(sourceDir); err == nil {
			fmt.Printf("  -> Detected 'addons'. Extracting target: '%s'\n", targetDir)
			os.RemoveAll(fullPath)
			if err := os.Rename(sourceDir, fullPath); err != nil {
				return err
			}
			fmt.Printf("  -> Successfully installed '%s' to %s\n", targetDir, fullPath)
		} else {
			fmt.Printf("  -> Error: target directory '%s' was not found in the repository.\n", targetDir)
		}
	}

	os.RemoveAll(tempDir)
	os.RemoveAll(filepath.Join(fullPath, ".git"))

	return nil
}

// ---------- helpers ----------

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if strings.HasPrefix(f.Name, "__MACOSX/") ||
			base == ".DS_Store" ||
			base == "Thumbs.db" ||
			base == "desktop.ini" ||
			base == "ehthumbs.db" {
			continue
		}
		fpath := filepath.Join(dest, filepath.Clean("/"+f.Name))
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}
		out, err := os.Create(fpath)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func getGitDirectory() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
