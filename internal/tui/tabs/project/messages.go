package project

import "gdaddon/internal/source"

// releasesMsg / branchesMsg carry the result of an upstream fetch back to the
// loading screen's onResult closure. They are domain messages: produced by the
// project fetch commands and consumed by the project loaders (loadingScreen never
// names them), so they stay in the tui package, not core.
type releasesMsg struct {
	listing *source.Listing
	err     error
}

type branchesMsg struct {
	branches []source.Asset
	err      error
}
