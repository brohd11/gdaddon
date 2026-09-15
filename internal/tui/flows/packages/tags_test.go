package packages

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/source"

	tea "charm.land/bubbletea/v2"
)

type tagTransport func(*http.Request) (*http.Response, error)

func (f tagTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBrowseTags(t *testing.T) {
	for _, published := range []bool{false, true} {
		t.Run(fmt.Sprintf("published=%v", published), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			t.Setenv("GITHUB_TOKEN", "test-token")
			calls := 0
			orig := http.DefaultClient.Transport
			t.Cleanup(func() { http.DefaultClient.Transport = orig })
			http.DefaultClient.Transport = tagTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				body := ""
				switch r.URL.Path {
				case "/repos/u/r/tags":
					body = `[{"name":"v1.0.0"}]`
				case "/repos/u/r/releases":
					body = `[]`
					if published {
						body = `[{"tag_name":"v1.0.0","assets":[{"name":"build.zip","browser_download_url":"https://download/build.zip"}]}]`
					}
				default:
					return nil, fmt.Errorf("unexpected request: %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			var selected *Selection
			opts := BrowseOpts{Source: SourceAll, IncludeHEAD: true, Endpoint: func(s Selection) core.Screen {
				selected = &s
				return components.NewPicker(nil, components.PickerOpts{Title: "Install"})
			}}
			archived := buildArchivedSet([]source.Release{{Tag: "v1.0.0", Assets: []source.Asset{{Name: "Source code.zip (archived)", URL: "/tmp/cached.zip"}}}})
			root := newVersionsPicker("github.com/u/r", "https://github.com/u/r", opts, nil, archived)
			sh := core.NewShared(nil)
			var model tea.Model = core.NewRouter(sh, []core.TabEntry{{Title: "Packages", New: func(*core.Shared) core.Screen { return root }}})
			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			if calls != 0 {
				t.Fatal("tag listing was not lazy")
			}
			// Tags is the first selectable row, before HEAD.
			model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			load, ok := model.(core.Router).Top().(*components.LoadingScreen)
			if !ok || load.Label != "fetching tags…" {
				t.Fatalf("top = %T", model.(core.Router).Top())
			}
			model, _ = model.Update(load.Run(context.Background())())
			model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			load, ok = model.(core.Router).Top().(*components.LoadingScreen)
			if !ok || load.Label != "resolving tag…" {
				t.Fatalf("top = %T", model.(core.Router).Top())
			}
			model, _ = model.Update(load.Run(context.Background())())
			if published {
				if selected != nil {
					t.Fatal("published release should offer its build and source in the asset picker")
				}
				model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				if selected == nil || selected.Asset.URL != "https://download/build.zip" {
					t.Fatalf("selected = %+v", selected)
				}
			} else {
				if selected == nil || !selected.Asset.Generated || selected.Tag != "v1.0.0" || selected.Branch || selected.ArchivedAsset.URL != "/tmp/cached.zip" {
					t.Fatalf("selected = %+v", selected)
				}
			}
		})
	}
}
