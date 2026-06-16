package source

import (
	"fmt"
	"net/url"
	"strings"
)

// repoRef is the parsed coordinates of a manifest/repo URL, host-agnostic.
type repoRef struct {
	Host   string
	Owner  string
	Repo   string
	Branch string // non-empty if the URL was a refs/heads archive
}

// parseRepoURL extracts host/owner/repo (and a tracked branch) from any standard
// git-host URL — a .git clone URL, a plain host/owner/repo, a release-download
// asset, or an archive/refs URL. It does not restrict the host; whether a host is
// supported for version listing is decided by ruleForHost. Nested-group hosts
// (e.g. GitLab subgroups) are out of scope and would mis-parse owner/repo.
func parseRepoURL(rawURL string) (repoRef, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return repoRef{}, fmt.Errorf("could not parse URL: %w", err)
	}
	host := strings.TrimPrefix(u.Host, "www.")
	if host == "" {
		return repoRef{}, fmt.Errorf("no host in %q", rawURL)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return repoRef{}, fmt.Errorf("could not find owner/repo in %q", rawURL)
	}

	ref := repoRef{Host: host, Owner: parts[0], Repo: strings.TrimSuffix(parts[1], ".git")}

	// .../archive/refs/heads/<branch>.zip → branch-tracking archive (GitHub form).
	if len(parts) >= 6 && parts[2] == "archive" && parts[3] == "refs" && parts[4] == "heads" {
		ref.Branch = strings.TrimSuffix(strings.Join(parts[5:], "/"), ".zip")
	}

	return ref, nil
}
