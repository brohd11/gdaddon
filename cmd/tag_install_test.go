package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/brohd11/gdaddon/internal/addon"
)

type tagTransport func(*http.Request) (*http.Response, error)

func (f tagTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// fakeGitHub serves u/r's releases and tags bodies, recording each path requested.
func fakeGitHub(t *testing.T, releases, tags string) *[]string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("GITHUB_TOKEN", "test-token")
	orig := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = orig })
	var seen []string
	http.DefaultClient.Transport = tagTransport(func(r *http.Request) (*http.Response, error) {
		seen = append(seen, r.URL.Path)
		body := ""
		switch r.URL.Path {
		case "/repos/u/r/releases":
			body = releases
		case "/repos/u/r/tags":
			body = tags
		default:
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	return &seen
}

func TestResolveEntryTagWithoutRelease(t *testing.T) {
	fakeGitHub(t, `[]`, `[{"name":"v1.1.0-beta"},{"name":"v1.0.0"},{"name":"v0.9.0"}]`)
	const v1 = "https://github.com/u/r/archive/refs/tags/v1.0.0.zip"

	spec, _ := addon.ParseRepoSpec("u/r@1.0.0")
	entry, err := resolveEntry(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Kind != addon.KindPackage || entry.Tag != "v1.0.0" || entry.URL != v1 {
		t.Fatalf("entry = %+v", entry)
	}

	// No releases at all: a bare spec and @latest take the newest non-prerelease tag.
	for _, raw := range []string{"u/r", "u/r@latest"} {
		spec, _ := addon.ParseRepoSpec(raw)
		entry, err := resolveEntry(context.Background(), spec)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if entry.Kind != addon.KindPackage || entry.Tag != "v1.0.0" || entry.URL != v1 {
			t.Fatalf("%s: entry = %+v", raw, entry)
		}
	}

	// A /release has no uploads to take from a bare tag.
	relSpec, _ := addon.ParseRepoSpec("u/r/release")
	if _, err := resolveEntry(context.Background(), relSpec); err == nil || !strings.Contains(err.Error(), "drop /release") {
		t.Fatalf("/release with no releases: err = %v", err)
	}

	spec.Tag = "main"
	if _, err := resolveEntry(context.Background(), spec); err == nil {
		t.Fatal("@main silently became a branch")
	}

	// A branch is only ever reached through the spec's own /clone marker — the ref half
	// alone never changes what kind of install this is.
	cloneSpec, ok := addon.ParseRepoSpec("u/r/clone@main")
	if !ok {
		t.Fatal("u/r/clone@main did not parse")
	}
	entry, err = resolveEntry(context.Background(), cloneSpec)
	if err != nil || entry.Kind != addon.KindClone || entry.Tag != "main" || !strings.HasSuffix(entry.URL, ".git") {
		t.Fatalf("clone entry = %+v, err=%v", entry, err)
	}
}

// A published release wins over a newer unpublished tag; tags aren't even listed.
func TestResolveEntryReleaseBeatsNewerTag(t *testing.T) {
	seen := fakeGitHub(t, `[{"tag_name":"v1.0.0","assets":[]}]`, `[{"name":"v2.0.0"},{"name":"v1.0.0"}]`)
	spec, _ := addon.ParseRepoSpec("u/r")
	entry, err := resolveEntry(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Tag != "v1.0.0" {
		t.Fatalf("entry = %+v, want the v1.0.0 release", entry)
	}
	for _, p := range *seen {
		if p == "/repos/u/r/tags" {
			t.Fatal("tags were listed although a release exists")
		}
	}
}

func TestResolveEntryNoReleasesNoTags(t *testing.T) {
	fakeGitHub(t, `[]`, `[]`)
	spec, _ := addon.ParseRepoSpec("u/r")
	if _, err := resolveEntry(context.Background(), spec); err == nil || !strings.Contains(err.Error(), "no releases or Git tags") {
		t.Fatalf("err = %v", err)
	}
}
