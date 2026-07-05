package search

import (
	"context"
	"fmt"

	"gdaddon/internal/config"
	"gdaddon/internal/restrule"
)

// configSource is a search.Source driven entirely by a declarative
// config.SourceConfig — it turns a YAML "rule" into search/detail HTTP calls and
// dotted-path extraction, so a new JSON-API store needs no Go backend. Only the
// "json" source type is implemented today.
type configSource struct{ cfg config.SourceConfig }

func (s configSource) Name() string { return s.cfg.Name }

// validate reports whether the rule is a usable JSON search source. Sources()
// uses it to skip a misconfigured (or vcs-only) entry rather than abort.
func (s configSource) validate() error {
	c := s.cfg
	if c.Name == "" {
		return fmt.Errorf("source: missing name")
	}
	if c.Type != "json" {
		return fmt.Errorf("source %q: unsupported type %q (only \"json\")", c.Name, c.Type)
	}
	if c.Search == nil || c.Search.URL == "" || c.Search.ResultsPath == "" {
		return fmt.Errorf("source %q: search.url and search.results_path are required", c.Name)
	}
	if c.Detail == nil || c.Detail.URL == "" || c.Detail.BrowseURLPath == "" {
		return fmt.Errorf("source %q: detail.url and detail.browse_url_path are required", c.Name)
	}
	return nil
}

func (s configSource) Search(ctx context.Context, query, godotVersion string, page int) (*Page, error) {
	r := s.cfg.Search
	endpoint := renderSearchURL(r.URL, query, godotVersion, page, r.PageBase, r.OmitIfEmpty)

	var root any
	if err := restrule.GetJSON(ctx, endpoint, &root); err != nil {
		return nil, err
	}

	arr, ok := restrule.GetPath(root, r.ResultsPath)
	results, _ := arr.([]any)
	if !ok || results == nil {
		// No array at results_path → an empty page, not an error (e.g. zero hits).
		return &Page{Page: page, Pages: page + 1}, nil
	}

	out := &Page{Page: page}
	for _, el := range results {
		out.Results = append(out.Results, Summary{
			ID:            restrule.GetPathString(el, r.Fields.ID),
			Title:         restrule.GetPathString(el, r.Fields.Title),
			Author:        restrule.GetPathString(el, r.Fields.Author),
			Category:      restrule.GetPathString(el, r.Fields.Category),
			Cost:          restrule.GetPathString(el, r.Fields.Cost),
			GodotVersion:  restrule.GetPathString(el, r.Fields.GodotVersion),
			VersionString: restrule.GetPathString(el, r.Fields.VersionString),
		})
	}

	if r.PagePath != "" {
		out.Page = restrule.GetPathInt(root, r.PagePath)
	}
	out.TotalItems = restrule.GetPathInt(root, r.TotalPath)
	switch {
	case r.PagesPath != "":
		out.Pages = restrule.GetPathInt(root, r.PagesPath)
	case r.TotalPath != "" && r.PerPage > 0:
		out.Pages = (out.TotalItems + r.PerPage - 1) / r.PerPage
	default:
		out.Pages = out.Page + 1
	}
	return out, nil
}

func (s configSource) Detail(ctx context.Context, id string) (*Detail, error) {
	r := s.cfg.Detail
	endpoint := renderDetailURL(r.URL, id)

	var root any
	if err := restrule.GetJSON(ctx, endpoint, &root); err != nil {
		return nil, err
	}

	return &Detail{
		Summary: Summary{
			ID:     id,
			Title:  restrule.GetPathString(root, r.TitlePath),
			Author: restrule.GetPathString(root, r.AuthorPath),
		},
		BrowseURL:   restrule.GetPathString(root, r.BrowseURLPath),
		DownloadURL: restrule.GetPathString(root, r.DownloadURLPath),
		Description: restrule.GetPathString(root, r.DescriptionPath),
	}, nil
}
