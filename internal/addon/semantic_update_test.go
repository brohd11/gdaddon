package addon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/brohd11/gdaddon/internal/source"
)

func TestLatestReleaseSemantic(t *testing.T) {
	for _, tc := range []struct {
		name     string
		releases []source.Release
		want     string
	}{
		{"misflagged beta", []source.Release{{Tag: "v0.1.1-beta2"}, {Tag: "v0.1.1"}}, "v0.1.1"},
		{"unordered stable", []source.Release{{Tag: "v1.9.0"}, {Tag: "v2.0.0-beta"}, {Tag: "v1.10.0"}}, "v1.10.0"},
		{"provider prerelease", []source.Release{{Tag: "v2.0.0", Prerelease: true}, {Tag: "v1.0.0"}}, "v1.0.0"},
		{"only beta", []source.Release{{Tag: "v1.0.0-beta.2"}, {Tag: "v1.0.0-beta.10"}}, "v1.0.0-beta.10"},
		{"mixed tags", []source.Release{{Tag: "release-final"}, {Tag: "v1.0.0"}}, "v1.0.0"},
		{"unknown stable before beta", []source.Release{{Tag: "v1.0.0-beta"}, {Tag: "release-final"}}, "release-final"},
		{"unknown fallback", []source.Release{{Tag: "release-final"}, {Tag: "2024-01-02"}}, "release-final"},
		{"equal precedence", []source.Release{{Tag: "v1.0.0+first"}, {Tag: "v1.0.0+second"}}, "v1.0.0+first"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rel, ok := LatestRelease(tc.releases)
			if !ok || rel.Tag != tc.want {
				t.Fatalf("got %q, %v; want %q", rel.Tag, ok, tc.want)
			}
		})
	}
}

func TestSemanticUpdateDecisions(t *testing.T) {
	const repo = "https://github.com/u/r"
	assetURL := func(tag string) string { return repo + "/releases/download/" + tag + "/addon.zip" }
	for _, tc := range []struct {
		name                                  string
		tags                                  []string
		installed, manifestTag, pluginVersion string
		bare                                  bool
		want                                  UpdateState
		latest                                string
	}{
		{"stable never downgraded", []string{"v0.1.1-beta2", "v0.1.1"}, "v0.1.1", "", "", false, UpdateCurrent, "v0.1.1"},
		{"beta to stable", []string{"v0.1.1-beta2", "v0.1.1"}, "v0.1.1-beta2", "v9.0.0", "", false, UpdateAvailable, "v0.1.1"},
		{"future beta not downgraded", []string{"v2.0.0-beta", "v1.0.0"}, "v2.0.0-beta", "", "", false, UpdateCurrent, "v1.0.0"},
		{"newer unlisted install", []string{"v1.0.0"}, "v2.0.0", "v2.0.0", "", false, UpdateCurrent, "v1.0.0"},
		{"beta only upgrade", []string{"v1.0.0-beta.2", "v1.0.0-beta.10"}, "v1.0.0-beta.2", "", "", false, UpdateAvailable, "v1.0.0-beta.10"},
		{"build metadata equal", []string{"v1.0.0+one", "v1.0.0+two"}, "v1.0.0+two", "", "", false, UpdateCurrent, "v1.0.0+one"},
		{"matched unknown ignores stale tag", []string{"v1.0.0", "release-final"}, "release-final", "v0.1.0", "", false, UpdateUnknown, ""},
		{"unknown candidate", []string{"release-final", "release-old"}, "release-old", "", "", false, UpdateUnknown, ""},
		{"exact unknown current", []string{"release-final"}, "release-final", "", "", false, UpdateCurrent, "release-final"},
		{"bare beta to stable", []string{"v0.1.1"}, "", "v0.1.1-beta2", "", true, UpdateAvailable, "v0.1.1"},
		{"plugin fallback", []string{"v0.1.1"}, "", "", "0.1.1-beta2", true, UpdateAvailable, "v0.1.1"},
		{"bad tag stays unknown", []string{"v1.0.0"}, "", "release-old", "0.1.0", true, UpdateUnknown, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bodies, _ := tagInstallHTTP(t)
			var releases []map[string]any
			for _, tag := range tc.tags {
				releases = append(releases, map[string]any{"tag_name": tag, "prerelease": false, "assets": []map[string]string{{"name": "addon.zip", "browser_download_url": assetURL(tag)}}})
			}
			body, err := json.Marshal(releases)
			if err != nil {
				t.Fatal(err)
			}
			bodies["/repos/u/r/releases"] = body
			a := Addon{URL: assetURL(tc.installed), Tag: tc.manifestTag, Version: tc.pluginVersion}
			if tc.bare {
				a.URL = repo
			}
			info := CheckUpdate(context.Background(), a)
			if info.State != tc.want || info.LatestTag != tc.latest {
				t.Fatalf("check = %+v, want %v (%s)", info, tc.want, tc.latest)
			}
			plan, resolution := ResolveUpdate(context.Background(), a, tc.pluginVersion)
			if tc.want == UpdateAvailable {
				if resolution != ResolvePlan || plan.NewTag != tc.latest || plan.Asset.URL != assetURL(tc.latest) {
					t.Fatalf("plan=%+v, resolution=%v", plan, resolution)
				}
			} else if resolution != ResolveNone {
				t.Fatalf("unexpected plan=%+v, resolution=%v", plan, resolution)
			}
		})
	}
}

func TestBetaDoesNotSatisfyStableDependency(t *testing.T) {
	d := Dependency{Tag: "v0.1.1"}
	if satisfied, verified := d.SatisfiedByTag("v0.1.1-beta2"); satisfied || !verified {
		t.Fatalf("satisfied=%v verified=%v", satisfied, verified)
	}
}

func TestUpdatesUseCompleteReleaseListing(t *testing.T) {
	for _, failure := range []string{"", "error", "cycle"} {
		t.Run("second page "+failure, func(t *testing.T) {
			tagInstallHTTP(t)
			http.DefaultClient.Transport = tagTransport(func(r *http.Request) (*http.Response, error) {
				header := make(http.Header)
				body := `[{"tag_name":"v1.9.0"}]`
				switch r.URL.Path {
				case "/repos/u/r/releases":
					header.Set("Link", `</page2>; rel="next"`)
				case "/page2":
					if failure == "error" {
						return nil, fmt.Errorf("page fetch failed")
					}
					if failure == "cycle" {
						header.Set("Link", `</page2>; rel="next"`)
					}
					body = `[{"tag_name":"v1.10.0"}]`
				default:
					t.Fatalf("unexpected request %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			a := Addon{URL: "https://github.com/u/r/archive/refs/tags/v1.9.0.zip", Tag: "v1.9.0"}
			info := CheckUpdate(context.Background(), a)
			plan, res := ResolveUpdate(context.Background(), a, a.Tag)
			if failure != "" {
				if info.State != UpdateUnknown || res != ResolveNone {
					t.Fatalf("info=%+v plan=%+v res=%v", info, plan, res)
				}
			} else if info.State != UpdateAvailable || info.LatestTag != "v1.10.0" || res != ResolvePlan || plan.NewTag != "v1.10.0" {
				t.Fatalf("info=%+v plan=%+v res=%v", info, plan, res)
			}
		})
	}
}

func TestUpdateExclusionsApplyToPlans(t *testing.T) {
	for _, a := range []Addon{
		{Kind: KindClone}, {Kind: KindSubmodule}, {Commit: "abc123"}, {Lock: true},
	} {
		t.Run(fmt.Sprintf("%+v", a), func(t *testing.T) {
			_, calls := tagInstallHTTP(t)
			a.URL = "https://github.com/u/r"
			a.Tag = "v1.0.0"
			want := UpdateUnknown
			if a.Lock {
				want = UpdateLocked
			}
			if info := CheckUpdate(context.Background(), a); info.State != want {
				t.Fatalf("check=%+v", info)
			}
			if plan, res := ResolveUpdate(context.Background(), a, a.Tag); res != ResolveNone {
				t.Fatalf("plan=%+v res=%v", plan, res)
			}
			if len(calls) != 0 {
				t.Fatalf("unexpected network requests=%v", calls)
			}
		})
	}
}
