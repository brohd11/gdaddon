package search

import (
	"encoding/json"
	"strconv"
	"strings"
)

// getPath walks dot-separated keys over JSON decoded into any (map[string]any /
// []any / scalars). A numeric segment indexes into an array (e.g.
// "items.0.name"). An empty path returns v unchanged. Any miss returns
// (nil, false).
func getPath(v any, path string) (any, bool) {
	if path == "" {
		return v, true
	}
	for _, seg := range strings.Split(path, ".") {
		switch cur := v.(type) {
		case map[string]any:
			x, ok := cur[seg]
			if !ok {
				return nil, false
			}
			v = x
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(cur) {
				return nil, false
			}
			v = cur[i]
		default:
			return nil, false
		}
	}
	return v, true
}

// getPathString resolves path and coerces the leaf to a string: strings are
// returned as-is, json.Number by its literal, bools by strconv, and nil/missing
// to "". An empty path returns "".
func getPathString(v any, path string) string {
	if path == "" {
		return ""
	}
	leaf, ok := getPath(v, path)
	if !ok {
		return ""
	}
	switch x := leaf.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return ""
	default:
		return ""
	}
}

// getPathInt resolves path and coerces the leaf to an int (json.Number or a
// numeric string). A miss or non-numeric leaf returns 0.
func getPathInt(v any, path string) int {
	if path == "" {
		return 0
	}
	leaf, ok := getPath(v, path)
	if !ok {
		return 0
	}
	switch x := leaf.(type) {
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	default:
		return 0
	}
}
