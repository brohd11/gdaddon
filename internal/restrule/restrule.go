// Package restrule holds the dependency-free primitives that drive gdaddon's
// declarative REST rules: an auth-aware JSON GET, a dotted-path walker over
// decoded JSON, and {placeholder} URL templating. Both internal/search (asset
// stores) and internal/source (VCS version resolution) build their config-driven
// providers on top of these, so the logic lives in one neutral place.
package restrule

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// GetJSON performs a GET and decodes the JSON body into out, with a shared
// timeout and User-Agent. auth == "github" adds Bearer $GITHUB_TOKEN (when set)
// to raise GitHub's API rate limit. The body is decoded with UseNumber so numeric
// ids and pagination fields coerce cleanly when out is an any.
func GetJSON(ctx context.Context, endpoint, auth string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "gdaddon")
	if auth == "github" {
		if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("request to %s rate-limited (set GITHUB_TOKEN to raise the limit)", req.URL.Host)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %s", req.URL.Host, resp.Status)
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	return dec.Decode(out)
}

// GetPath walks dot-separated keys over JSON decoded into any (map[string]any /
// []any / scalars). A numeric segment indexes into an array (e.g. "items.0.name").
// An empty path returns v unchanged. Any miss returns (nil, false).
func GetPath(v any, path string) (any, bool) {
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

// GetPathString resolves path and coerces the leaf to a string: strings as-is,
// json.Number by its literal, bools by strconv, nil/missing to "".
func GetPathString(v any, path string) string {
	if path == "" {
		return ""
	}
	leaf, ok := GetPath(v, path)
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
	default:
		return ""
	}
}

// GetPathInt resolves path and coerces a numeric leaf (json.Number or numeric
// string) to an int. A miss or non-numeric leaf returns 0.
func GetPathInt(v any, path string) int {
	if path == "" {
		return 0
	}
	leaf, ok := GetPath(v, path)
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

// GetPathBool resolves path and coerces the leaf to a bool (bool as-is, or the
// string "true"). A miss returns false.
func GetPathBool(v any, path string) bool {
	if path == "" {
		return false
	}
	leaf, ok := GetPath(v, path)
	if !ok {
		return false
	}
	switch x := leaf.(type) {
	case bool:
		return x
	case string:
		b, _ := strconv.ParseBool(x)
		return b
	default:
		return false
	}
}

// Render substitutes {key} placeholders with their values, verbatim (no
// escaping — callers escape values that need it before calling).
func Render(tmpl string, vars map[string]string) string {
	for k, v := range vars {
		tmpl = strings.ReplaceAll(tmpl, "{"+k+"}", v)
	}
	return tmpl
}
