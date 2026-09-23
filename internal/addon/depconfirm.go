package addon

import "errors"

// ErrDepAborted ends a dependency walk at the user's request; it propagates unchanged so
// front-ends can report a clean abort.
var ErrDepAborted = errors.New("dependency installation aborted")

// DepAction is what a walk is about to do to a declared dependency (satisfied ones are
// skipped before any request).
type DepAction int

const (
	DepAdd       DepAction = iota // no manifest entry: record it and install
	DepRepin                      // entry present but verifiably behind: re-pin and install
	DepReinstall                  // entry recorded, nothing on disk: install it
)

func (a DepAction) String() string {
	switch a {
	case DepAdd:
		return "add"
	case DepRepin:
		return "re-pin"
	case DepReinstall:
		return "re-install"
	}
	return "unknown"
}

// DepRequest is one pending dependency, fully resolved (AssetURL is what would be
// downloaded) but not yet written anywhere.
type DepRequest struct {
	Dep        Dependency // the declared spec — RepoID, Tag and RepoURL
	DeclaredBy string     // manifest name of the addon whose config declares it
	Action     DepAction
	EntryName  string // the manifest entry name it will be recorded as
	AssetURL   string // the url that will be downloaded ("" for a tagless, repo-only add)
	LocalTag   string // the tag currently recorded, for DepRepin ("" otherwise)
}

// DepConfirmer decides whether one resolved dependency may be recorded and installed.
// Declining leaves no trace, and the walk also skips whatever it declares. An error
// aborts the walk (ErrDepAborted for a deliberate quit). nil accepts everything.
type DepConfirmer func(DepRequest) (bool, error)

// allowDep applies a (possibly nil) confirmer, defaulting to yes.
func allowDep(confirm DepConfirmer, req DepRequest) (bool, error) {
	if confirm == nil {
		return true, nil
	}
	return confirm(req)
}
