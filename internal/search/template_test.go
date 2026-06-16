package search

import "testing"

func TestRenderSearchURL(t *testing.T) {
	tmpl := "https://ex.com/a?filter={query}&page={page}&godot_version={godot_version}"

	// page+pageBase, query escaped.
	got := renderSearchURL(tmpl, "my addon", "4.3", 0, 0, nil)
	want := "https://ex.com/a?filter=my+addon&page=0&godot_version=4.3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// page_base 1 shifts the page number.
	got = renderSearchURL(tmpl, "x", "4.3", 2, 1, nil)
	want = "https://ex.com/a?filter=x&page=3&godot_version=4.3"
	if got != want {
		t.Errorf("page_base: got %q, want %q", got, want)
	}

	// omit_if_empty drops the blank trailing param.
	got = renderSearchURL(tmpl, "x", "", 0, 0, []string{"godot_version"})
	want = "https://ex.com/a?filter=x&page=0"
	if got != want {
		t.Errorf("omit trailing: got %q, want %q", got, want)
	}

	// omit_if_empty drops a blank middle param, keeping the rest well-formed.
	mid := "https://ex.com/a?godot_version={godot_version}&page={page}"
	got = renderSearchURL(mid, "x", "", 0, 0, []string{"godot_version"})
	want = "https://ex.com/a?page=0"
	if got != want {
		t.Errorf("omit middle: got %q, want %q", got, want)
	}
}

func TestRenderDetailURL(t *testing.T) {
	// {id} is substituted raw so a slash stays a path separator.
	got := renderDetailURL("https://api.github.com/repos/{id}", "owner/repo")
	want := "https://api.github.com/repos/owner/repo"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
