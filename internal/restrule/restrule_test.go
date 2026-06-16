package restrule

import (
	"encoding/json"
	"strings"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func TestGetPath(t *testing.T) {
	root := decode(t, `{
		"full_name": "owner/repo",
		"owner": {"login": "owner"},
		"items": [{"name": "a"}, {"name": "b"}],
		"count": 1600,
		"flag": true
	}`)

	strCases := map[string]string{
		"full_name":    "owner/repo",
		"owner.login":  "owner",
		"items.1.name": "b",
		"count":        "1600", // json.Number, not "1.6e+03"
		"missing":      "",
		"items.9.name": "",
	}
	for path, want := range strCases {
		if got := GetPathString(root, path); got != want {
			t.Errorf("GetPathString(%q) = %q, want %q", path, got, want)
		}
	}
	if GetPathInt(root, "count") != 1600 {
		t.Errorf("GetPathInt(count) != 1600")
	}
	if !GetPathBool(root, "flag") {
		t.Errorf("GetPathBool(flag) should be true")
	}
	if GetPathBool(root, "missing") {
		t.Errorf("GetPathBool(missing) should be false")
	}
}

func TestRender(t *testing.T) {
	got := Render("https://h/{owner}/{repo}/archive/{tag}.zip",
		map[string]string{"owner": "o", "repo": "r", "tag": "v1.0"})
	if want := "https://h/o/r/archive/v1.0.zip"; got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}
