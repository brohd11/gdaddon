package search

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// assetStoreBase is the root of the new Godot Asset Store
// (https://store.godotengine.org), the eventual replacement for the legacy
// Asset Library.
const assetStoreBase = "https://store.godotengine.org"

// assetStore is the new Godot Asset Store backend.
//
// The store has no JSON search endpoint yet (its /api/v1/assets/ ignores text
// filters), so Search scrapes the server-rendered /search/ HTML — that's the only
// surface that actually ranks/filters by query. Detail uses the clean JSON
// /api/v1/assets/<publisher>/<slug>/ endpoint. The HTML parsing below is the
// fragile part and is intentionally contained to this file; if the store ships a
// real search API, only Search changes.
type assetStore struct{}

func (assetStore) Name() string { return "Asset Store" }

// Card structure (stable as of 2026-06): each result is
//
//	<div class="item"> … <h3 class="name"><a href="/asset/<pub>/<slug>/">Title</a>
//	  <span class="rating">…<span class="number">N</span></span></h3>
//	  <p class="details">by <a href="/publisher/<pub>/">Author</a> | LICENSE</p> …
var (
	storeAssetAnchorRe = regexp.MustCompile(`<a href="/asset/([^"/]+)/([^"/]+)/">([^<]*)</a>`)
	storePubAnchorRe   = regexp.MustCompile(`<a href="/publisher/[^"]*">([^<]*)</a>`)
	storeLicenseRe     = regexp.MustCompile(`\|\s*([^<|]+?)\s*</p>`)
)

// Search scrapes /search/?query=<q>. The store doesn't filter by engine version
// and its HTML pagination is JS-driven, so godotVersion and page are ignored and a
// single page (~24 results) is returned.
func (assetStore) Search(ctx context.Context, query, _ string, _ int) (*Page, error) {
	body, err := getText(ctx, assetStoreBase+"/search/?query="+url.QueryEscape(query))
	if err != nil {
		return nil, err
	}

	out := &Page{Pages: 1}
	// Split on item boundaries so author/license are read from the right card.
	for _, card := range strings.Split(body, `class="item"`)[1:] {
		m := storeAssetAnchorRe.FindStringSubmatch(card)
		if m == nil {
			continue
		}
		s := Summary{
			ID:    m[1] + "/" + m[2], // "<publisher>/<slug>" — the detail-endpoint key
			Title: cleanText(m[3]),
		}
		if a := storePubAnchorRe.FindStringSubmatch(card); a != nil {
			s.Author = cleanText(a[1])
		}
		if l := storeLicenseRe.FindStringSubmatch(card); l != nil {
			s.Cost = cleanText(l[1])
		}
		out.Results = append(out.Results, s)
	}
	out.TotalItems = len(out.Results)
	return out, nil
}

// Detail resolves the asset's repo URL via the clean JSON endpoint. id is the
// "<publisher>/<slug>" produced by Search. source may be empty (paid/direct-download
// assets); the caller handles that.
func (assetStore) Detail(ctx context.Context, id string) (*Detail, error) {
	var raw struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Source      string `json:"source"`
		LicenseType string `json:"license_type"`
		Publisher   struct {
			Name string `json:"name"`
		} `json:"publisher"`
	}
	if err := getJSON(ctx, assetStoreBase+"/api/v1/assets/"+id+"/", &raw); err != nil {
		return nil, err
	}
	return &Detail{
		Summary: Summary{
			ID:     id,
			Title:  raw.Name,
			Author: raw.Publisher.Name,
			Cost:   raw.LicenseType,
		},
		BrowseURL:   raw.Source,
		Description: raw.Description,
	}, nil
}

// cleanText unescapes HTML entities and trims whitespace from a scraped fragment.
func cleanText(s string) string {
	return strings.TrimSpace(html.UnescapeString(s))
}

// getText performs a GET and returns the response body as a string (for the
// server-rendered search page). Mirrors getJSON's timeout/User-Agent.
func getText(ctx context.Context, endpoint string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gdaddon")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("asset store returned %s", resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}
