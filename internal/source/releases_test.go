package source

import (
	"context"
	"reflect"
	"testing"
)

func TestSortReleases(t *testing.T) {
	releases := []Release{{Tag: "unknown-a"}, {Tag: "v1.9.0"}, {Tag: "v1.10.0-beta2"}, {Tag: "unknown-b"}, {Tag: "v1.10.0+first"}, {Tag: "v1.10.0+second"}, {Tag: "v2.0.0-beta"}}
	SortReleases(releases)
	var got []string
	for _, r := range releases {
		got = append(got, r.Tag)
	}
	want := []string{"v2.0.0-beta", "v1.10.0+first", "v1.10.0+second", "v1.10.0-beta2", "v1.9.0", "unknown-a", "unknown-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestReleasePagination(t *testing.T) {
	const first = "/repos/u/r/releases?per_page=30"
	for _, tc := range []struct {
		name    string
		second  tagPage
		wantErr bool
	}{
		{"complete", tagPage{body: `[{"tag_name":"v1.0.0"},{"tag_name":"v1.10.0"}]`}, false},
		{"failed", tagPage{status: 503}, true},
		{"cycle", tagPage{body: `[]`, next: first}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := tagHTTP(t, map[string]tagPage{
				first:    {body: `[{"tag_name":"v1.0.0-beta2","prerelease":false},{"tag_name":"v1.0.0"}]`, next: "/page2"},
				"/page2": tc.second,
			})
			listing, err := AvailableVersions(context.Background(), "https://github.com/u/r")
			if len(*calls) != 2 {
				t.Fatalf("calls=%v", *calls)
			}
			if tc.wantErr {
				if err == nil || listing != nil {
					t.Fatalf("partial listing=%+v err=%v", listing, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(listing.Releases) != 3 || listing.Releases[0].Tag != "v1.10.0" || !listing.Releases[2].Prerelease {
				t.Fatalf("releases=%+v", listing.Releases)
			}
		})
	}
}

func TestReleasePaginationCanceled(t *testing.T) {
	calls := tagHTTP(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	listing, err := AvailableVersions(ctx, "https://github.com/u/r")
	if err == nil || listing != nil || len(*calls) != 0 {
		t.Fatalf("listing=%v err=%v calls=%v", listing, err, *calls)
	}
}
