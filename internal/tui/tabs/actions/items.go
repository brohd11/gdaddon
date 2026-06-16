package actions

import "github.com/charmbracelet/bubbles/list"

// ---------- list items ----------

// actionKind identifies a row in the Actions menu.
type actionKind int

const (
	actInstallAll actionKind = iota
	actNewPlugin
	actImportPlugin
)

// actionItem is one row in the Actions menu.
type actionItem struct {
	title string
	desc  string
	kind  actionKind
}

func (a actionItem) Title() string       { return a.title }
func (a actionItem) FilterValue() string { return a.title }
func (a actionItem) Description() string { return a.desc }

// actionItems builds the Actions menu rows.
func actionItems() []list.Item {
	return []list.Item{
		actionItem{title: "↧ Install / update all", desc: "download everything per the manifest", kind: actInstallAll},
		actionItem{title: "+ New Plugin", desc: "add a plugin to the project or your global list", kind: actNewPlugin},
		actionItem{title: "⬇ Import Plugin", desc: "add a plugin from your global list", kind: actImportPlugin},
	}
}

// importItem is one row in the Import Plugin picker (an entry from the global
// list); selecting it copies the entry into the project manifest.
type importItem struct {
	name string
	url  string
	path string
}

func (i importItem) Title() string       { return i.name }
func (i importItem) FilterValue() string { return i.name }
func (i importItem) Description() string { return i.url }
