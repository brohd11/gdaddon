package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gdaddon/internal/config"
)

func TestConfigSourceSearchAndDetail(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("page"); got != "2" { // page 1 + page_base 1
			t.Errorf("page param = %q, want 2", got)
		}
		w.Write([]byte(`{
			"total_count": 45,
			"items": [
				{"full_name": "owner/a", "owner": {"login": "owner"}},
				{"full_name": "owner/b", "owner": {"login": "owner"}}
			]
		}`))
	})
	mux.HandleFunc("/repos/owner/a", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"clone_url": "https://github.com/owner/a.git", "description": "hi"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	src := configSource{cfg: config.SourceConfig{
		Name: "Test",
		Type: "json",
		Search: &config.SearchRule{
			URL:         srv.URL + "/search?q={query}&page={page}",
			PageBase:    1,
			ResultsPath: "items",
			PerPage:     20,
			TotalPath:   "total_count",
			Fields:      config.FieldPaths{ID: "full_name", Title: "full_name", Author: "owner.login"},
		},
		Detail: &config.DetailRule{
			URL:             srv.URL + "/repos/{id}",
			BrowseURLPath:   "clone_url",
			DescriptionPath: "description",
		},
	}}

	if err := src.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	page, err := src.Search(context.Background(), "dialogue", "", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(page.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(page.Results))
	}
	if page.Results[0].ID != "owner/a" || page.Results[0].Author != "owner" {
		t.Errorf("result[0] = %+v", page.Results[0])
	}
	if page.TotalItems != 45 {
		t.Errorf("TotalItems = %d, want 45", page.TotalItems)
	}
	if page.Pages != 3 { // ceil(45/20)
		t.Errorf("Pages = %d, want 3", page.Pages)
	}

	d, err := src.Detail(context.Background(), "owner/a")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.BrowseURL != "https://github.com/owner/a.git" || d.Description != "hi" {
		t.Errorf("detail = %+v", d)
	}
}

func TestConfigSourceValidate(t *testing.T) {
	bad := []config.SourceConfig{
		{Name: "", Type: "json"},
		{Name: "x", Type: "html"},
		{Name: "x", Type: "json", Search: &config.SearchRule{URL: "u"}},                   // missing results_path
		{Name: "x", Type: "json", Search: &config.SearchRule{URL: "u", ResultsPath: "r"}}, // missing detail
	}
	for i, c := range bad {
		if err := (configSource{cfg: c}).validate(); err == nil {
			t.Errorf("case %d: expected validation error, got nil", i)
		}
	}
}
