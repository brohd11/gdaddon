package docs

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The pages carry no front matter: their title and index description are read out of the
// markdown itself, so a page that forgets its "# " heading would silently show up in the
// menu as a blank row. Assert the convention holds for every page that ships.
func TestPagesParse(t *testing.T) {
	pages := Pages()
	if len(pages) == 0 {
		t.Fatal("no pages embedded")
	}
	for _, p := range pages {
		if p.Title == "" {
			t.Errorf("page has no title (missing a `# ` heading?): %.40q", p.Body)
		}
		if p.Desc == "" {
			t.Errorf("page %q has no description line under its heading", p.Title)
		}
		if strings.TrimSpace(p.Body) == "" {
			t.Errorf("page %q has no body", p.Title)
		}
	}
}

// The renderer folds to the width DocScreen hands it; a row wider than that would be
// clipped at the pane edge with no way to scroll to it.
func TestRenderFitsWidth(t *testing.T) {
	for _, width := range []int{40, 80} {
		for _, p := range Pages() {
			for i, line := range strings.Split(render(p.Body, width), "\n") {
				if w := ansi.StringWidth(line); w > width {
					t.Errorf("page %q at width %d: line %d is %d cols wide: %q", p.Title, width, i+1, w, line)
				}
			}
		}
	}
}
