package search

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// renderSearchURL substitutes a search rule's template placeholders. {query} and
// {godot_version} are query-escaped (they live in the query string); {page} is
// page+pageBase. Params named in omitIfEmpty whose value is empty are dropped
// entirely (e.g. the Asset Library wants godot_version omitted, not blank).
func renderSearchURL(tmpl, query, godotVersion string, page, pageBase int, omitIfEmpty []string) string {
	out := tmpl
	out = strings.ReplaceAll(out, "{query}", url.QueryEscape(query))
	out = strings.ReplaceAll(out, "{godot_version}", url.QueryEscape(godotVersion))
	out = strings.ReplaceAll(out, "{page}", strconv.Itoa(page+pageBase))

	values := map[string]string{"query": query, "godot_version": godotVersion}
	for _, name := range omitIfEmpty {
		if values[name] == "" {
			out = dropEmptyParam(out, name)
		}
	}
	return out
}

// renderDetailURL substitutes {id} raw. The id may contain '/' (e.g. a GitHub
// "owner/repo"), which must stay a path separator, so it is not escaped.
func renderDetailURL(tmpl, id string) string {
	return strings.ReplaceAll(tmpl, "{id}", id)
}

// dropEmptyParam removes a now-empty "name=" query parameter and its leading
// separator, leaving the rest of the URL well-formed.
func dropEmptyParam(rawURL, name string) string {
	re := regexp.MustCompile(`([?&])` + regexp.QuoteMeta(name) + `=(&|$)`)
	return re.ReplaceAllStringFunc(rawURL, func(m string) string {
		sub := re.FindStringSubmatch(m)
		// "?x=&" → keep "?"; "?x=" / "&x=" / "&x=&" → keep the trailing sep or drop both.
		if sub[1] == "?" && sub[2] == "&" {
			return "?"
		}
		if sub[2] == "&" {
			return sub[1]
		}
		return ""
	})
}
