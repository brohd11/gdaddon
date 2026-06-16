package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// assetLibBase is the REST root of the legacy Godot Asset Library
// (https://godotengine.org/asset-library). See the godotengine/godot-asset-library
// API.md for the endpoints used here.
const assetLibBase = "https://godotengine.org/asset-library/api"

// assetLib is the legacy Godot Asset Library backend.
type assetLib struct{}

func (assetLib) Name() string { return "Asset Library" }

func (assetLib) Search(ctx context.Context, query, godotVersion string, page int) (*Page, error) {
	q := url.Values{}
	q.Set("filter", query)
	q.Set("type", "addon")
	q.Set("max_results", "20")
	q.Set("page", strconv.Itoa(page))
	q.Set("sort", "updated")
	// Without an engine version the API returns only a small legacy set.
	if godotVersion != "" {
		q.Set("godot_version", godotVersion)
	}

	// Numeric ids/versions come back as JSON strings; the pagination fields are
	// JSON numbers.
	var raw struct {
		Result []struct {
			AssetID       string `json:"asset_id"`
			Title         string `json:"title"`
			Author        string `json:"author"`
			Category      string `json:"category"`
			Cost          string `json:"cost"`
			GodotVersion  string `json:"godot_version"`
			VersionString string `json:"version_string"`
		} `json:"result"`
		Page       int `json:"page"`
		Pages      int `json:"pages"`
		TotalItems int `json:"total_items"`
	}
	if err := getJSON(ctx, assetLibBase+"/asset?"+q.Encode(), &raw); err != nil {
		return nil, err
	}

	out := &Page{Page: raw.Page, Pages: raw.Pages, TotalItems: raw.TotalItems}
	for _, r := range raw.Result {
		out.Results = append(out.Results, Summary{
			ID:            r.AssetID,
			Title:         r.Title,
			Author:        r.Author,
			Category:      r.Category,
			Cost:          r.Cost,
			GodotVersion:  r.GodotVersion,
			VersionString: r.VersionString,
		})
	}
	return out, nil
}

func (assetLib) Detail(ctx context.Context, id string) (*Detail, error) {
	var raw struct {
		AssetID       string `json:"asset_id"`
		Title         string `json:"title"`
		Author        string `json:"author"`
		Category      string `json:"category"`
		Cost          string `json:"cost"`
		GodotVersion  string `json:"godot_version"`
		VersionString string `json:"version_string"`
		BrowseURL     string `json:"browse_url"`
		DownloadURL   string `json:"download_url"`
		Description   string `json:"description"`
	}
	if err := getJSON(ctx, assetLibBase+"/asset/"+url.PathEscape(id), &raw); err != nil {
		return nil, err
	}
	return &Detail{
		Summary: Summary{
			ID:            raw.AssetID,
			Title:         raw.Title,
			Author:        raw.Author,
			Category:      raw.Category,
			Cost:          raw.Cost,
			GodotVersion:  raw.GodotVersion,
			VersionString: raw.VersionString,
		},
		BrowseURL:   raw.BrowseURL,
		DownloadURL: raw.DownloadURL,
		Description: raw.Description,
	}, nil
}

// getJSON performs a GET and decodes the JSON body into out, with a shared
// timeout and User-Agent (mirrors source.ghGetJSON).
func getJSON(ctx context.Context, endpoint string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "gdaddon")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("asset library returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
