//go:build !darwin

package quarantine

import (
	"context"
	"errors"
)

// Clear is unavailable off macOS: no other platform has com.apple.quarantine. The
// caller (the Actions row) is already gated on runtime.GOOS, so this only exists to
// keep the package building on linux/windows.
func Clear(ctx context.Context, root string) (Result, error) {
	return Result{}, errors.New("quarantine clearing is only supported on macOS")
}
