// Package quarantine clears macOS's com.apple.quarantine attribute from a project's
// addons. Gatekeeper uses the attribute to block a compiled plugin's native binaries
// from loading, so an addon downloaded as a zip needs it removed before Godot can use
// it. Clear is a no-op stub on non-darwin platforms.
package quarantine

// Attr is the extended attribute Gatekeeper reads.
const Attr = "com.apple.quarantine"

// maxErrs caps how many per-path failures Clear reports; a broken tree would
// otherwise produce one line per file.
const maxErrs = 5

// Result summarizes a Clear run.
type Result struct {
	Cleared int      // entries the attribute was actually removed from
	Denied  int      // entries that refused the removal (permission), skipped
	Errs    []string // the first maxErrs other failures, for the log
}
