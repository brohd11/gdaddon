package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
)

// Measure list publication separately from Git, manifest parsing and dependency I/O.
func BenchmarkRefreshList(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		for _, filtered := range []bool{false, true} {
			b.Run(fmt.Sprintf("n=%d/filter=%v", n, filtered), func(b *testing.B) {
				dir := b.TempDir()
				var manifest strings.Builder
				for i := range n {
					fmt.Fprintf(&manifest, "repo%05d:\n  url: https://example.invalid/repo%d.zip\n  path: addons/repo%d\n", i, i, i)
				}
				if err := os.WriteFile(filepath.Join(dir, "addon_manifest.yml"), []byte(manifest.String()), 0644); err != nil {
					b.Fatal(err)
				}
				c := appctx.NewDeferred(dir, "test")
				statuses := c.ProjectStatuses()
				sh := core.NewShared(c)
				s := &projectState{screen: components.NewRootList(nil, components.RootListOpts{})}
				s.screen.SetSize(sh, 100, 30)
				if filtered {
					s.screen.List().SetFilterText("repo")
				}
				b.ReportAllocs()
				publications, built := 0, 0
				b.ResetTimer()
				for range b.N {
					for _, st := range statuses {
						c.Pending[st.Addon.Name] = true
					}
					s.rebuild(sh)
					publications++
					built += len(statuses)
					for i := range statuses {
						statuses[i].State = addon.StateInstalled
						delete(c.Pending, statuses[i].Addon.Name)
						c.DirtyProjectRows[statuses[i].Addon.Name] = true
						if (i+1)%(n/10) == 0 && i+1 < n {
							publications++
							built += len(c.DirtyProjectRows)
							s.publish(sh)
						}
					}
					s.rebuild(sh)
					publications++
					built += len(statuses)
				}
				b.ReportMetric(float64(publications)/float64(b.N), "publications/op")
				b.ReportMetric(float64(built)/float64(b.N), "rows/op")
			})
		}
	}
}
