package actions

import "github.com/charmbracelet/bubbles/list"

// ---------- list items ----------

// actionKind identifies a row in the Actions menu.
type actionKind int

const (
	actInstallAll actionKind = iota
	actNewPlugin
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
	}
}
