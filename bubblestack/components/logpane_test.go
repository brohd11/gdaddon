package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The failure the wrap mode exists for: a line far wider than the pane, whose tail the
// viewport clips away with no way to scroll to it (a deep path, no spaces to fold at).
const longPath = "xattr: [Errno 13] permission denied: " +
	"'/Users/brohd/godot/Godot-Plugin-Dev-No-Sub/addons/syntax_plus/.git/objects/24/c2a606d5b73cc522d3fee8d3b8fe2f48866047'"

// paneAt builds a sized pane holding lines. 80x24 gives innerWidth 74.
func paneAt(lines ...string) *LogPane {
	p := NewLogPane()
	for _, l := range lines {
		p.Log(l, true)
	}
	p.SetSize(80, 24)
	return p
}

// TestContentTruncatedMode is the baseline: unwrapped, every entry is one row (the
// viewport is what clips it), so the long line is a single overlong row.
func TestContentTruncatedMode(t *testing.T) {
	p := paneAt("short", longPath)

	rows := strings.Split(p.content(), "\n")
	if len(rows) != 2 {
		t.Fatalf("unwrapped content should be one row per entry, got %d rows", len(rows))
	}
	if w := ansi.StringWidth(rows[1]); w <= p.innerWidth() {
		t.Fatalf("the long entry should overflow the pane (width %d, inner %d)", w, p.innerWidth())
	}
}

// TestContentWrapMode: every row fits the pane, entries stay distinguishable (bullet
// then hanging indent), and no text is lost in the fold.
func TestContentWrapMode(t *testing.T) {
	p := paneAt("short", longPath)
	p.ToggleWrap()

	rows := strings.Split(p.content(), "\n")
	if len(rows) < 3 {
		t.Fatalf("the long entry should fold across rows, got %d rows total", len(rows))
	}

	var bullets, recovered []string
	for _, row := range rows {
		if w := ansi.StringWidth(row); w > p.innerWidth() {
			t.Errorf("row wider than the pane (%d > %d): %q", w, p.innerWidth(), row)
		}
		r := ansi.Strip(row) // LogStyle wraps each row in escape codes on a color terminal
		switch {
		case strings.HasPrefix(r, logBullet):
			bullets = append(bullets, r)
			recovered = append(recovered, strings.TrimPrefix(r, logBullet))
		case strings.HasPrefix(r, logIndent):
			recovered = append(recovered, strings.TrimPrefix(r, logIndent))
		default:
			t.Errorf("row carries neither bullet nor indent: %q", r)
		}
	}
	if len(bullets) != 2 {
		t.Errorf("want one bullet per entry (2), got %d", len(bullets))
	}

	// Folding is lossless: rejoining the rows recovers both entries. ansi.Wrap eats the
	// space it breaks at, so compare with whitespace collapsed.
	joined := strings.Join(recovered, "")
	for _, want := range []string{"short", longPath} {
		if !strings.Contains(collapse(joined), collapse(want)) {
			t.Errorf("wrapping lost text: %q not recoverable from the rows", want)
		}
	}
}

// TestToggleWrapRoundTrip: wrap is a plain mode flag over the same in-memory lines.
func TestToggleWrapRoundTrip(t *testing.T) {
	p := paneAt(longPath)
	before := p.content()
	if p.Wrapped() {
		t.Fatal("wrap should start off")
	}

	p.ToggleWrap()
	if !p.Wrapped() {
		t.Fatal("ToggleWrap should turn wrap on")
	}
	if p.content() == before {
		t.Fatal("wrapped content should differ from truncated content")
	}

	p.ToggleWrap()
	if p.Wrapped() {
		t.Fatal("ToggleWrap should turn wrap back off")
	}
	if p.content() != before {
		t.Fatal("toggling back should restore the truncated rendering exactly")
	}
}

// TestWrapNarrowPane: the bullet leaves no room at all — wrapping must still terminate
// and never produce a negative width.
func TestWrapNarrowPane(t *testing.T) {
	p := NewLogPane()
	p.Log(longPath, true)
	p.SetSize(1, 1) // innerWidth clamps to its 10-col floor
	p.ToggleWrap()

	for _, r := range strings.Split(p.content(), "\n") {
		if w := ansi.StringWidth(r); w > p.innerWidth() {
			t.Fatalf("row wider than the clamped pane (%d > %d): %q", w, p.innerWidth(), r)
		}
	}
}

// collapse strips whitespace so a comparison ignores where the wrap fell.
func collapse(s string) string { return strings.Join(strings.Fields(s), "") }
