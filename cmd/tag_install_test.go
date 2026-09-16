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

func TestResolveEntryTagWithoutRelease(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("GITHUB_TOKEN", "test-token")
	orig := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = orig })
	http.DefaultClient.Transport = tagTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Path {
		case "/repos/u/r/releases":
			body = `[]`
		case "/repos/u/r/tags":
			body = `[{"name":"v1.0.0"}]`
		default:
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	spec, _ := addon.ParseRepoSpec("u/r@1.0.0")
	entry, err := resolveEntry(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Kind != addon.KindPackage || entry.Tag != "v1.0.0" || entry.URL != "https://github.com/u/r/archive/refs/tags/v1.0.0.zip" {
		t.Fatalf("entry = %+v", entry)
	}
	spec.Tag = ""
	if _, err := resolveEntry(context.Background(), spec); err == nil {
		t.Fatal("tag-only repo became latest release")
	}
	spec.Tag = "main"
	if _, err := resolveEntry(context.Background(), spec); err == nil {
		t.Fatal("@main silently became a branch")
	}

	// A branch is only ever reached through the spec's own clone: prefix — the ref half
	// alone never changes what kind of install this is.
	cloneSpec, ok := addon.ParseRepoSpec("clone:u/r@main")
	if !ok {
		t.Fatal("clone:u/r@main did not parse")
	}
	entry, err = resolveEntry(context.Background(), cloneSpec)
	if err != nil || entry.Kind != addon.KindClone || entry.Tag != "main" || !strings.HasSuffix(entry.URL, ".git") {
		t.Fatalf("clone entry = %+v, err=%v", entry, err)
	}
}
