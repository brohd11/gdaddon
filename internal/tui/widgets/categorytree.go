package widgets

import (
	"sort"
	"strconv"
	"strings"

	"github.com/brohd11/gdaddon/internal/addon"

	"github.com/brohd11/bubblestack/components"
)

// CategoryLeaf is one row for CategoryTree.
type CategoryLeaf struct {
	ID       string // stable node ID; folds follow it
	Category string // slash path ("tools/editor"); "" is the top level
	Item     components.CompactItem
	// Warn marks a leaf with a warning; every folder above it shows a bare ⚠ so the
	// warning stays visible while folded.
	Warn bool
}

// CategoryFolderPrefix starts every folder node's ID, keeping folders apart from leaves.
const CategoryFolderPrefix = "cat:"

// CategoryTree files leaves into nested folders, one per category segment. Leaves keep the
// given order inside each folder, so callers sort first; folders sort by name (reverse
// flips) and come before leaves at every level. Folders start collapsed and show their
// plugin count, plus a bare ⚠ when a Warn leaf sits anywhere beneath them.
func CategoryTree(leaves []CategoryLeaf, reverse bool) []components.TreeNode {
	root := newCategoryFolder("", "")
	for _, l := range leaves {
		f := root
		if norm := addon.NormalizeCategory(l.Category); norm != "" {
			for _, seg := range strings.Split(norm, "/") {
				f = f.child(seg)
			}
		}
		f.leaves = append(f.leaves, l)
	}
	return root.nodes(reverse)
}

type categoryFolder struct {
	name, path string
	folders    map[string]*categoryFolder
	leaves     []CategoryLeaf
}

func newCategoryFolder(name, path string) *categoryFolder {
	return &categoryFolder{name: name, path: path, folders: map[string]*categoryFolder{}}
}

// child returns the named subfolder, creating it.
func (f *categoryFolder) child(name string) *categoryFolder {
	if c, ok := f.folders[name]; ok {
		return c
	}
	path := name
	if f.path != "" {
		path = f.path + "/" + name
	}
	c := newCategoryFolder(name, path)
	f.folders[name] = c
	return c
}

// warns reports whether any leaf at or below f is marked Warn.
func (f *categoryFolder) warns() bool {
	for _, l := range f.leaves {
		if l.Warn {
			return true
		}
	}
	for _, c := range f.folders {
		if c.warns() {
			return true
		}
	}
	return false
}

// count is the number of leaves at and below f.
func (f *categoryFolder) count() int {
	n := len(f.leaves)
	for _, c := range f.folders {
		n += c.count()
	}
	return n
}

func (f *categoryFolder) nodes(reverse bool) []components.TreeNode {
	subs := make([]*categoryFolder, 0, len(f.folders))
	for _, c := range f.folders {
		subs = append(subs, c)
	}
	sort.Slice(subs, func(i, j int) bool {
		a, b := strings.ToLower(subs[i].name), strings.ToLower(subs[j].name)
		if reverse {
			return a > b
		}
		return a < b
	})
	out := make([]components.TreeNode, 0, len(subs)+len(f.leaves))
	for _, c := range subs {
		name := c.name
		if c.warns() {
			name += "  ⚠"
		}
		out = append(out, components.TreeNode{
			ID:        CategoryFolderPrefix + c.path,
			Item:      components.CompactItem{Name: name, Suffix: strconv.Itoa(c.count())},
			Children:  c.nodes(reverse),
			Collapsed: true,
		})
	}
	for _, l := range f.leaves {
		out = append(out, components.TreeNode{ID: l.ID, Item: l.Item})
	}
	return out
}
