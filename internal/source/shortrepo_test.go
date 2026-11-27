package source

import "testing"

func TestShortRepo(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/owner/repo":                                      "owner/repo",
		"https://www.github.com/owner/repo/":                                 "owner/repo",
		"https://github.com/owner/repo.git":                                  "owner/repo",
		"https://github.com/owner/repo/releases/download/v1.0/repo-v1.0.zip": "owner/repo",
		"https://github.com/owner/repo/archive/refs/heads/main.zip":          "owner/repo",
		"https://gitlab.com/group/project":                                   "gitlab.com/group/project",
		"https://codeberg.org/owner/repo.git":                                "codeberg.org/owner/repo",
		"https://example.com/":                                               "example.com/",
		"not a url":                                                          "not a url",
		"":                                                                   "",
	} {
		if got := ShortRepo(in); got != want {
			t.Errorf("ShortRepo(%q) = %q, want %q", in, got, want)
		}
	}
}
