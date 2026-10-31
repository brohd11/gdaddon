package appctx

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/brohd11/gdaddon/internal/addon"
)

func syntheticRefresh(n int) (*Ctx, int64) {
	c := &Ctx{projectLoaded: true, Pending: make(map[string]bool), DirtyProjectRows: make(map[string]bool), RootPending: true}
	for i := range n {
		name := fmt.Sprintf("repo%05d", i)
		c.projectStatuses = append(c.projectStatuses, addon.Status{Addon: addon.Addon{Name: name}})
		c.Pending[name] = true
	}
	c.indexProject()
	gen, _ := c.Refresh.Start(n + 1)
	return c, gen
}

func TestRefreshResultsRejectUnknownDuplicateAndStale(t *testing.T) {
	c, gen := syntheticRefresh(2)
	first := addonInspectedMsg{gen: gen, status: c.projectStatuses[0]}
	if handled, changed, _ := c.ApplyRefreshResult(first); !handled || !changed || c.Pending[first.status.Addon.Name] {
		t.Fatal("current result did not update its indexed row")
	}
	for _, bad := range []addonInspectedMsg{
		first,
		{gen: gen, status: addon.Status{Addon: addon.Addon{Name: "unknown"}}},
		{gen: gen - 1, status: c.projectStatuses[1]},
	} {
		if handled, changed, _ := c.ApplyRefreshResult(bad); !handled || changed {
			t.Fatal("duplicate, unknown or stale result was accepted")
		}
	}
	if _, changed, _ := c.ApplyRefreshResult(rootDescribedMsg{gen: gen}); changed || c.RootPending {
		t.Fatal("intermediate root must land without invalidating list rows")
	}
	c.ApplyRefreshResult(rootDescribedMsg{gen: gen})
	if !c.Refresh.Running() {
		t.Fatal("invalid deliveries advanced the progress count")
	}
	c.ApplyRefreshResult(addonInspectedMsg{gen: gen, status: c.projectStatuses[1]})
	if c.Refresh.Running() || len(c.Pending) != 0 {
		t.Fatal("valid terminal result did not finish the pass")
	}
}

func TestRepeatedRefreshRetainsUnseenRows(t *testing.T) {
	dir := t.TempDir()
	manifest := "alpha:\n  url: https://example.invalid/alpha.zip\n  path: addons/alpha\nbeta:\n  url: https://example.invalid/beta.zip\n  path: addons/beta\n"
	if err := os.WriteFile(filepath.Join(dir, "addon_manifest.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	c := NewDeferred(dir, "test")
	c.StartProjectRefresh()
	if !c.Unseen("alpha") || !c.Unseen("beta") {
		t.Fatal("restarting before inspection lost unseen state")
	}
	c.unseen["alpha"] = false // alpha has now been inspected; beta still has no cached state
	c.StartProjectRefresh()
	if c.Unseen("alpha") || !c.Unseen("beta") {
		t.Fatal("refresh must distinguish inspected and still-unseen entries")
	}
}

func BenchmarkApplyRefreshResults(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			c, _ := syntheticRefresh(n)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				for _, s := range c.projectStatuses {
					c.Pending[s.Addon.Name] = true
				}
				clear(c.DirtyProjectRows)
				gen, _ := c.Refresh.Start(n + 1) // keep the root pending to exclude dependency I/O
				for _, s := range c.projectStatuses {
					c.ApplyRefreshResult(addonInspectedMsg{gen: gen, status: s})
				}
			}
		})
	}
}
