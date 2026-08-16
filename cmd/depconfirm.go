package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"gdaddon/internal/addon"

	"github.com/brohd11/goutil/strutil"
	"github.com/mattn/go-isatty"
)

// depPrompter is the CLI's addon.DepConfirmer. A plugin's dependencies are declared by
// its author and followed transitively, so `gdaddon install <repo>` can pull in repos
// the user never named; this asks about each one before it is recorded or downloaded.
//
// Off a terminal there is nobody to ask, so every dependency is declined and reported
// rather than silently installed — an unattended run never pulls unreviewed code.
// `--trust-deps` skips the prompter entirely (a nil confirmer) for a package the user
// already trusts.
type depPrompter struct {
	in      *bufio.Reader
	out     io.Writer
	tty     bool
	all     bool               // latched by "a": install the rest without asking
	skipped []addon.DepRequest // for the end-of-run summary
}

func newDepPrompter() *depPrompter {
	fd := os.Stdin.Fd()
	return &depPrompter{
		in:  bufio.NewReader(os.Stdin),
		out: os.Stdout,
		tty: isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd),
	}
}

// confirm implements addon.DepConfirmer.
func (p *depPrompter) confirm(req addon.DepRequest) (bool, error) {
	if p.all {
		return true, nil
	}
	if !p.tty {
		p.skipped = append(p.skipped, req)
		return false, nil
	}

	fmt.Fprint(p.out, depBanner(req))
	for {
		fmt.Fprint(p.out, "  Install it? [y/N/a(ll)/q(uit)]: ")
		line, err := p.in.ReadString('\n')
		// A read error with nothing buffered means stdin ended mid-run (the terminal
		// went away). Decline rather than spin on an unanswerable prompt.
		if err != nil && strings.TrimSpace(line) == "" {
			fmt.Fprintln(p.out)
			p.skipped = append(p.skipped, req)
			return false, nil
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		case "", "n", "no": // empty input takes the safe default
			p.skipped = append(p.skipped, req)
			return false, nil
		case "a", "all":
			p.all = true
			return true, nil
		case "q", "quit":
			return false, addon.ErrDepAborted
		}
		fmt.Fprintln(p.out, "  Please answer y, n, a or q.")
	}
}

// depBanner describes one pending dependency: who declared it, what it is, and the url
// that would actually be downloaded — the asset, not the repo, since a release can
// point anywhere.
func depBanner(req addon.DepRequest) string {
	declarer := req.DeclaredBy
	if declarer == "" {
		declarer = "this project"
	}
	version := "(no version)"
	if req.Dep.Tag != "" {
		version = "@" + req.Dep.Tag
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n  %s declares a dependency:\n", declarer)
	fmt.Fprintf(&b, "    %s %s\n", req.Dep.RepoID, version)
	if req.AssetURL != "" {
		fmt.Fprintf(&b, "    %s\n", req.AssetURL)
	}
	switch req.Action {
	case addon.DepRepin:
		fmt.Fprintf(&b, "    re-pins the existing %q entry from %s\n", req.EntryName, orNone(req.LocalTag))
	case addon.DepReinstall:
		fmt.Fprintf(&b, "    re-installs the existing %q entry (recorded, but not on disk)\n", req.EntryName)
	}
	return b.String()
}

func orNone(tag string) string {
	if tag == "" {
		return "(no version)"
	}
	return tag
}

// reportSkipped prints the dependencies that were not installed, and how to install
// them. Nil-safe so the --trust-deps path (no prompter at all) can call it blindly.
func (p *depPrompter) reportSkipped(w io.Writer) {
	if p == nil || len(p.skipped) == 0 {
		return
	}
	reason := "declined"
	if !p.tty {
		reason = "not a terminal, nothing to confirm with"
	}
	fmt.Fprintf(w, "\nskipped %d dependenc%s (%s):\n",
		len(p.skipped), strutil.Plural(len(p.skipped), "y", "ies"), reason)
	for _, req := range p.skipped {
		version := ""
		if req.Dep.Tag != "" {
			version = " @" + req.Dep.Tag
		}
		fmt.Fprintf(w, "  %s%s\n", req.Dep.RepoID, version)
	}
	fmt.Fprintln(w, "pass --trust-deps to install them")
}

// depConfirmer builds the confirmer the install flows vet dependencies with, plus the
// prompter behind it for the end-of-run summary. Both are nil when there is nothing to
// vet — --trust-deps (install the closure unattended, gdaddon's behavior before this
// existed) or --no-deps (no closure at all) — and addon treats a nil DepConfirmer as
// unconditional yes.
func depConfirmer() (*depPrompter, addon.DepConfirmer) {
	if addonInstallTrustDeps || addonInstallNoDeps {
		return nil, nil
	}
	p := newDepPrompter()
	return p, p.confirm
}
