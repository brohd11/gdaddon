//go:build !darwin

package quarantine

import (
	"context"
	"errors"
)

// Clear is unavailable off macOS; callers are gated on runtime.GOOS.
func Clear(ctx context.Context, root string) (Result, error) {
	return Result{}, errors.New("quarantine clearing is only supported on macOS")
}
