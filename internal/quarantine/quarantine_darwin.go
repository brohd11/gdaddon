package quarantine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Clear removes com.apple.quarantine from every non-hidden entry under root. Hidden
// directories (.git) are skipped: their read-only objects only produce errors and never
// hold blocked binaries. Per-entry failures are counted; only an unreadable root aborts.
// ctx stops it.
func Clear(ctx context.Context, root string) (Result, error) {
	var res Result
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			res.note(path, err)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// The L form acts on a symlink itself; WalkDir doesn't follow them either.
		switch rmErr := unix.Lremovexattr(path, Attr); {
		case rmErr == nil:
			res.Cleared++
		case errors.Is(rmErr, unix.ENOATTR):
			// Not quarantined — the common case, nothing to do.
		default:
			res.note(path, rmErr)
		}
		return nil
	})
	return res, err
}

func (r *Result) note(path string, err error) {
	if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
		r.Denied++
		return
	}
	if len(r.Errs) < maxErrs {
		r.Errs = append(r.Errs, fmt.Sprintf("%s: %v", path, err))
	}
}
