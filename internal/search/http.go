package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// getJSON performs a GET and decodes the JSON body into out, with a shared
// timeout and User-Agent (mirrors source.ghGetJSON).
func getJSON(ctx context.Context, endpoint string, out any) error {
	return getJSONAuth(ctx, endpoint, "", out)
}

// getJSONAuth is getJSON with an optional auth scheme. auth == "github" sends
// Bearer $GITHUB_TOKEN (when set) to raise GitHub's API rate limit, mirroring
// internal/source/github.go. The JSON is decoded with UseNumber so numeric ids
// and pagination fields both coerce cleanly.
func getJSONAuth(ctx context.Context, endpoint, auth string, out any) error {
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

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("source returned %s", resp.Status)
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	return dec.Decode(out)
}
