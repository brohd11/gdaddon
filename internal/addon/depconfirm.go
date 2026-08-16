package addon

import "errors"

// ErrDepAborted ends a dependency walk at the user's request. A DepConfirmer returns
// it to stop the whole closure rather than declining one dependency; the walk
// propagates it out unchanged so the front-end can report a clean abort instead of a
// failure.
var ErrDepAborted = errors.New("dependency installation aborted")

// DepAction is what a dependency walk is about to do to one declared dependency —
// the three cases that reach the network and the disk. An already-satisfied dep is
// not one of them: it is skipped before any DepRequest is built.
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

// DepRequest is one pending dependency install: fully resolved (the asset lookup has
// already happened, so AssetURL is the url that would actually be downloaded) but not
// yet acted on. Nothing has been written to the manifest or to disk when a
// DepConfirmer sees it.
type DepRequest struct {
	Dep        Dependency // the declared spec — RepoID, Tag and RepoURL
	DeclaredBy string     // manifest name of the addon whose config declares it
	Action     DepAction
	EntryName  string // the manifest entry name it will be recorded as
	AssetURL   string // the url that will be downloaded ("" for a tagless, repo-only add)
	LocalTag   string // the tag currently recorded, for DepRepin ("" otherwise)
}

// DepConfirmer decides whether one dependency may be recorded and installed. It is
// called after the dependency resolves and before anything is written, so declining
// leaves no trace: no manifest entry, no download, and — because the walk drops a
// declined dependency from its queue — no visit to whatever *it* declares in turn.
// Returning an error aborts the entire walk (ErrDepAborted for a deliberate quit).
//
// A nil DepConfirmer installs everything, which is what every caller did before this
// existed — so it is the front-end-agnostic seam Reporter is, not a behavior change
// for callers that don't opt in. gdaddon's dependency graph is author-declared and
// transitively followed, so this is the point at which a user can decline code they
// never named.
type DepConfirmer func(DepRequest) (bool, error)

// allowDep applies a (possibly nil) confirmer, defaulting to yes.
func allowDep(confirm DepConfirmer, req DepRequest) (bool, error) {
	if confirm == nil {
		return true, nil
	}
	return confirm(req)
}
