package newplugin

import (
	"strings"
	"testing"

	"gdaddon/internal/addon"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// TestNewStoreForm prefills the canonical store url and focuses the Name field.
func TestNewStoreForm(t *testing.T) {
	f := NewStoreForm("https://store.godotengine.org/publisher/slug", "2.1.0")
	if got := f.Value("url"); got != "https://store.godotengine.org/publisher/slug" {
		t.Fatalf("url not prefilled, got %q", got)
	}
	if f.FocusedKey() != "name" {
		t.Fatalf("focus should be the Name field, got %q", f.FocusedKey())
	}
}

// TestStoreFormToConfirm checks the store submit keeps the canonical store url
// untouched (never normalized into a .git url) and the confirm shows the release.
func TestStoreFormToConfirm(t *testing.T) {
	tm := sized(newTestRouter())
	tm, _ = tm.Update(core.Push(NewStoreForm("https://store.godotengine.org/publisher/slug", "2.1.0")))

	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tm.(core.Router).Top().(*components.DialogScreen); !ok {
		t.Fatalf("filled URL should push confirm, got %T", tm.(core.Router).Top())
	}
	view := tm.View()
	if !strings.Contains(view, "https://store.godotengine.org/publisher/slug") {
		t.Fatal("confirm should show the store url as typed")
	}
	if strings.Contains(view, "slug.git") {
		t.Fatal("store url must not be normalized into a .git url")
	}
	if !strings.Contains(view, "2.1.0") {
		t.Fatal("confirm should show the release version")
	}
}

// TestNewFromInstall prefills name/path/url from the scanned install, presets the
// kind picker for a git checkout, and focuses the url field so the user confirms it.
func TestNewFromInstall(t *testing.T) {
	f := NewFromInstall("addons/foo", "foo", "1.2.3", "https://github.com/owner/foo", addon.KindSubmodule, "main")
	if got := f.Value("url"); got != "https://github.com/owner/foo" {
		t.Fatalf("url not prefilled, got %q", got)
	}
	if got := f.Value("name"); got != "foo" {
		t.Fatalf("name not prefilled, got %q", got)
	}
	if got := f.Value("path"); got != "addons/foo" {
		t.Fatalf("path not prefilled, got %q", got)
	}
	// The kind preset is asserted end-to-end in TestTrackFormToConfirm: ToggleField is
	// not a `valued` field, so FormScreen.Value("kind") cannot read it here.
	if f.FocusedKey() != "url" {
		t.Fatalf("focus should be the URL field, got %q", f.FocusedKey())
	}
}

// TestTrackFormToConfirm checks the track submit normalizes a git url and the confirm
// shows the version label and the kind line (with branch) for a git checkout.
func TestTrackFormToConfirm(t *testing.T) {
	tm := sized(newTestRouter())
	tm, _ = tm.Update(core.Push(NewFromInstall("addons/foo", "foo", "1.2.3", "https://github.com/owner/foo", addon.KindSubmodule, "main")))

	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tm.(core.Router).Top().(*components.DialogScreen); !ok {
		t.Fatalf("filled URL should push confirm, got %T", tm.(core.Router).Top())
	}
	view := tm.View()
	for _, want := range []string{"Track plugin", "https://github.com/owner/foo.git", "v1.2.3", "submodule", "(branch main)"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirm should show %q", want)
		}
	}
}

// TestNormalizeTrackURL leaves store urls canonical and normalizes everything else.
func TestNormalizeTrackURL(t *testing.T) {
	if got := normalizeTrackURL("https://store.godotengine.org/publisher/slug"); got != "https://store.godotengine.org/publisher/slug" {
		t.Fatalf("store url must stay as typed, got %q", got)
	}
	if got := normalizeTrackURL("https://github.com/owner/repo"); got != "https://github.com/owner/repo.git" {
		t.Fatalf("git url should gain a .git suffix, got %q", got)
	}
}
