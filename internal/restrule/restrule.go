// Package restrule holds the primitives behind gdaddon's declarative REST rules: an
// authenticated JSON GET, a dotted-path walker over decoded JSON, and {placeholder} URL
// templating. internal/search and internal/source both build on them.
package restrule

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brohd11/gdaddon/internal/gitcred"
)

// GetJSON GETs endpoint and decodes JSON into out (UseNumber), with the shared timeout and
// User-Agent and a gitcred Bearer token when one exists (higher rate limits, private
// repos).
func GetJSON(ctx context.Context, endpoint string, out any) error {
	_, err := GetJSONPage(ctx, endpoint, out)
	return err
}

// GetJSONPage also returns the next page advertised by a standard Link header.
// Resolve relative links against the endpoint URL; only follow the same origin.
func GetJSONPage(ctx context.Context, endpoint string, out any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := authedGet(ctx, endpoint, "application/json", gitcred.TokenForURL(ctx, endpoint))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return "", err
	}
	for _, header := range resp.Header.Values("Link") {
		for _, link := range strings.Split(header, ",") {
			parts := strings.Split(link, ";")
			for _, param := range parts[1:] {
				if strings.TrimSpace(param) != `rel="next"` && strings.TrimSpace(param) != "rel=next" {
					continue
				}
				base, err := url.Parse(endpoint)
				if err != nil {
					return "", err
				}
				next, err := base.Parse(strings.Trim(strings.TrimSpace(parts[0]), "<>"))
				if err != nil || next.Scheme != base.Scheme || next.Host != base.Host {
					return "", fmt.Errorf("invalid pagination link from %s", endpoint)
				}
				return next.String(), nil
			}
		}
	}
	return "", nil
}

// Get is an authenticated GET returning the response for the caller to stream and close.
// No timeout (downloads can be large; ctx decides). Non-2xx is an error with the body
// closed.
func Get(ctx context.Context, url string) (*http.Response, error) {
	return authedGet(ctx, url, "", gitcred.TokenForURL(ctx, url))
}

// authedGet sends a GET with the User-Agent, optional Accept and Bearer token, returning
// only 2xx responses (rate limits and other failures are errors). Shared by Get and
// GetJSON.
func authedGet(ctx context.Context, url, accept, tok string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "gdaddon")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		return nil, fmt.Errorf("request to %s rate-limited (set GITHUB_TOKEN to raise the limit)", req.URL.Host)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("%s returned %s", req.URL.Host, resp.Status)
	}
	return resp, nil
}

// Download streams url's body into dst (created/truncated) via Get.
func Download(ctx context.Context, url, dst string) error {
	resp, err := Get(ctx, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// GetPath walks dot-separated keys over decoded JSON; numeric segments index arrays
// ("items.0.name"). An empty path returns v; a miss returns (nil, false).
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
