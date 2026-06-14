package addon

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func downloadAndExtractZip(url, targetPath, addonName string, report Reporter) error {
	report("[%s] Downloading ZIP from %s...", addonName, url)

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

	report("[%s] Extracting...", addonName)
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

	report("  -> Successfully installed to %s", targetPath)
	return nil
}

func cloneGitRepo(url, fullPath, addonName, targetDir string, report Reporter) error {
	report("[%s] Cloning repository...", addonName)

	tempDir := fullPath + "_temp"
	os.RemoveAll(tempDir)

	cmd := exec.Command("git", "clone", "--depth", "1", url, tempDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		report("  -> Failed to clone %s:\n%s", addonName, string(out))
		return err
	}

	addonsPath := filepath.Join(tempDir, "addons")

	if _, err := os.Stat(addonsPath); os.IsNotExist(err) {
		report("  -> No 'addons' folder detected. Installing full repository...")
		os.RemoveAll(fullPath)
		if err := os.Rename(tempDir, fullPath); err != nil {
			return err
		}
		report("  -> Successfully installed to %s", fullPath)
	} else {
		var sourceDir string
		if targetDir != "" {
			sourceDir = filepath.Clean(filepath.Join(tempDir, targetDir))
		} else {
			sourceDir = tempDir
		}

		if _, err := os.Stat(sourceDir); err == nil {
			report("  -> Detected 'addons'. Extracting target: '%s'", targetDir)
			os.RemoveAll(fullPath)
			if err := os.Rename(sourceDir, fullPath); err != nil {
				return err
			}
			report("  -> Successfully installed '%s' to %s", targetDir, fullPath)
		} else {
			return fmt.Errorf("target directory '%s' was not found in the repository", targetDir)
		}
	}

	os.RemoveAll(tempDir)
	os.RemoveAll(filepath.Join(fullPath, ".git"))

	return nil
}

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
