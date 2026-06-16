package search

import (
	"context"

	"gdaddon/internal/restrule"
)

// getJSON is the no-auth JSON GET used by the Asset Store detail endpoint. The
// generic provider calls restrule.GetJSON directly (with the source's auth).
func getJSON(ctx context.Context, endpoint string, out any) error {
	return restrule.GetJSON(ctx, endpoint, "", out)
}
