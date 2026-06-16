package search

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

func TestGetPathString(t *testing.T) {
	root := decode(t, `{
		"full_name": "owner/repo",
		"owner": {"login": "owner"},
		"items": [{"name": "a"}, {"name": "b"}],
		"count": 1600
	}`)

	tests := []struct {
		path string
		want string
	}{
		{"full_name", "owner/repo"},
		{"owner.login", "owner"},
		{"items.1.name", "b"},
		{"count", "1600"}, // json.Number, not "1.6e+03"
		{"missing", ""},
		{"owner.missing", ""},
		{"items.9.name", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := getPathString(root, tt.path); got != tt.want {
			t.Errorf("getPathString(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestGetPathInt(t *testing.T) {
	root := decode(t, `{"total": 42, "as_string": "7", "name": "x"}`)
	if got := getPathInt(root, "total"); got != 42 {
		t.Errorf("total = %d, want 42", got)
	}
	if got := getPathInt(root, "as_string"); got != 7 {
		t.Errorf("as_string = %d, want 7", got)
	}
	if got := getPathInt(root, "name"); got != 0 {
		t.Errorf("name = %d, want 0", got)
	}
	if got := getPathInt(root, "missing"); got != 0 {
		t.Errorf("missing = %d, want 0", got)
	}
}
