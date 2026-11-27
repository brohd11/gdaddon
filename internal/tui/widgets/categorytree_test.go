package widgets

import (
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
)

// flatten renders nodes depth-first as "id(suffix)" with two spaces per depth.
func flatten(nodes []components.TreeNode, depth int, out *[]string) {
	for _, n := range nodes {
		label := strings.Repeat("  ", depth) + n.ID
		if s := n.Item.SuffixText(); s != "" && strings.HasPrefix(n.ID, CategoryFolderPrefix) {
			label += "(" + s + ")"
		}
		*out = append(*out, label)
		flatten(n.Children, depth+1, out)
	}
}

func TestCategoryTree(t *testing.T) {
	leaf := func(id, cat string) CategoryLeaf {
		return CategoryLeaf{ID: id, Category: cat, Item: components.CompactItem{Name: id}}
	}
	// Leaves arrive in caller order (z before a) and must stay that way.
	leaves := []CategoryLeaf{
		leaf("z-top", ""), leaf("deep", "Tools/editor"), leaf("z-tool", "Tools"),
		leaf("a-tool", " Tools/ "), leaf("art", "art"), leaf("a-top", ""),
	}
	var got []string
	nodes := CategoryTree(leaves, false)
	flatten(nodes, 0, &got)
	want := []string{
		"cat:art(1)",
		"  art",
		"cat:Tools(3)",
		"  cat:Tools/editor(1)",
		"    deep",
		"  z-tool",
		"  a-tool",
		"z-top",
		"a-top",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("tree:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, n := range nodes[:2] {
		if !n.Collapsed {
			t.Fatalf("folder %s should start collapsed", n.ID)
		}
	}

	got = nil
	flatten(CategoryTree(leaves, true), 0, &got)
	if got[0] != "cat:Tools(3)" || got[len(got)-1] != "a-top" {
		t.Fatalf("reverse should flip folders only, got %v", got)
	}
}

func TestCategoryTreeBubblesWarnings(t *testing.T) {
	nodes := CategoryTree([]CategoryLeaf{
		{ID: "deep", Category: "tools/editor", Warn: true, Item: components.CompactItem{Name: "deep  ⚠ [update]"}},
		{ID: "fine", Category: "tools", Item: components.CompactItem{Name: "fine"}},
		{ID: "calm", Category: "art", Item: components.CompactItem{Name: "calm"}},
		{ID: "top", Warn: true, Item: components.CompactItem{Name: "top  ⚠ [update]"}},
	}, false)
	art, tools := nodes[0], nodes[1]
	if art.Item.Title() != "art" {
		t.Fatalf("a folder with no warnings stays plain, got %q", art.Item.Title())
	}
	if tools.Item.Title() != "tools  ⚠" || tools.Item.SuffixText() != "2" {
		t.Fatalf("tools should carry a bare ⚠ and keep its count, got %q / %q", tools.Item.Title(), tools.Item.SuffixText())
	}
	editor := tools.Children[0]
	if editor.Item.Title() != "editor  ⚠" {
		t.Fatalf("every ancestor of a warning leaf is marked, got %q", editor.Item.Title())
	}
	if got := editor.Children[0].Item.Title(); got != "deep  ⚠ [update]" {
		t.Fatalf("the leaf keeps its own text, got %q", got)
	}
}
