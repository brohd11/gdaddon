package addon

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/brohd11/goutil/strutil"
)

// resolveUnder joins rel onto baseDir and refuses results outside it. Install paths can
// come from the downloaded package's own config (installDir), and the destination is
// deleted before writing, so an unchecked ".." would be a recursive delete outside the
// project. unzip applies the same guard to archive members.
func resolveUnder(baseDir, rel string) (string, error) {
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("could not resolve path: %w", err)
	}
	full, err := filepath.Abs(filepath.Join(base, rel))
	if err != nil {
		return "", fmt.Errorf("could not resolve path: %w", err)
	}
	if _, ok := strutil.RelUnder(base, full); !ok {
		return "", fmt.Errorf("refusing to use %q: it resolves outside the project root", rel)
	}
	return full, nil
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
		// Close is checked on the success path too: a write buffered by Copy can still
		// fail at close, which would otherwise land as a silently truncated file.
		cerr := out.Close()
		rc.Close()
		if err != nil {
			return err
		}
		if cerr != nil {
			return cerr
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
