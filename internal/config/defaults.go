package config

// DefaultConfig is the config dumped on first run: the default archive dir plus
// the built-in search sources. Add future defaults here so Ensure picks them up.
func DefaultConfig() *Config {
	return &Config{
		ArchiveDir:   "~/.gdaddon/archive",
		CurrentTheme: "mono",
		Sources:      DefaultSources(),
	}
}

// DefaultSources are the built-in provider rules. They use the same schema as
// user entries, so the dumped file doubles as a worked example. GitHub carries
// both a search rule and its github.com vcs rule; Codeberg is vcs-only (no search
// backend yet). The Asset Store stays a hard-coded Go backend (its HTML scrape
// can't be expressed here) and is appended by internal/search, not listed here.
func DefaultSources() []SourceConfig {
	return []SourceConfig{
		{
			Name: "GitHub",
			Type: "json",
			Auth: "github",
			Search: &SearchRule{
				URL:         "https://api.github.com/search/repositories?q={query}&per_page=20&page={page}",
				PageBase:    1,
				ResultsPath: "items",
				PerPage:     20,
				TotalPath:   "total_count",
				Fields: FieldPaths{
					ID:            "full_name",
					Title:         "full_name",
					Author:        "owner.login",
					VersionString: "default_branch",
				},
			},
			Detail: &DetailRule{
				URL:             "https://api.github.com/repos/{id}",
				BrowseURLPath:   "clone_url", // ends in .git → accepted by the installer
				DescriptionPath: "description",
				TitlePath:       "full_name",
				AuthorPath:      "owner.login",
			},
			VCS: &VCSRule{
				Host: "github.com",
				Auth: "github",
				Releases: ReleasesRule{
					URL:            "https://api.github.com/repos/{owner}/{repo}/releases?per_page=30",
					TagPath:        "tag_name",
					PrereleasePath: "prerelease",
					AssetsPath:     "assets",
					AssetNamePath:  "name",
					AssetURLPath:   "browser_download_url",
					AssetSuffix:    ".zip",
				},
				SourceArchive: ArchiveSpec{
					Name: "Source code.zip",
					URL:  "https://github.com/{owner}/{repo}/archive/refs/tags/{tag}.zip",
				},
				Branches: BranchesRule{
					URL:        "https://api.github.com/repos/{owner}/{repo}/branches?per_page=100",
					NamePath:   "name",
					ArchiveURL: "https://github.com/{owner}/{repo}/archive/refs/heads/{branch}.zip",
				},
				BranchArchiveURL: "https://github.com/{owner}/{repo}/archive/refs/heads/{branch}.zip",
			},
		},
		{
			Name: "Asset Library",
			Type: "json",
			Search: &SearchRule{
				URL:         "https://godotengine.org/asset-library/api/asset?filter={query}&type=addon&max_results=20&page={page}&sort=updated&godot_version={godot_version}",
				OmitIfEmpty: []string{"godot_version"},
				ResultsPath: "result",
				PagePath:    "page",
				PagesPath:   "pages",
				TotalPath:   "total_items",
				Fields: FieldPaths{
					ID:            "asset_id",
					Title:         "title",
					Author:        "author",
					Category:      "category",
					Cost:          "cost",
					GodotVersion:  "godot_version",
					VersionString: "version_string",
				},
			},
			Detail: &DetailRule{
				URL:             "https://godotengine.org/asset-library/api/asset/{id}",
				BrowseURLPath:   "browse_url",
				DownloadURLPath: "download_url",
				DescriptionPath: "description",
				TitlePath:       "title",
				AuthorPath:      "author",
			},
		},
		{
			Name: "Codeberg",
			Type: "json",
			Search: &SearchRule{
				URL:         "https://codeberg.org/api/v1/repos/search?q={query}&limit=50&page={page}",
				PageBase:    1,
				ResultsPath: "data",
				Fields: FieldPaths{
					ID:            "full_name",
					Title:         "full_name",
					Author:        "owner.login",
					VersionString: "default_branch",
				},
			},
			Detail: &DetailRule{
				URL:             "https://codeberg.org/api/v1/repos/{id}",
				BrowseURLPath:   "clone_url",
				DescriptionPath: "description",
				TitlePath:       "full_name",
				AuthorPath:      "owner.login",
			},
			VCS: &VCSRule{
				Host: "codeberg.org",
				Releases: ReleasesRule{
					URL:            "https://codeberg.org/api/v1/repos/{owner}/{repo}/releases?limit=30",
					TagPath:        "tag_name",
					PrereleasePath: "prerelease",
					AssetsPath:     "assets",
					AssetNamePath:  "name",
					AssetURLPath:   "browser_download_url",
					AssetSuffix:    ".zip",
				},
				SourceArchive: ArchiveSpec{
					Name: "Source code.zip",
					URL:  "https://codeberg.org/{owner}/{repo}/archive/{tag}.zip",
				},
				Branches: BranchesRule{
					URL:        "https://codeberg.org/api/v1/repos/{owner}/{repo}/branches?limit=100",
					NamePath:   "name",
					ArchiveURL: "https://codeberg.org/{owner}/{repo}/archive/{branch}.zip",
				},
				BranchArchiveURL: "https://codeberg.org/{owner}/{repo}/archive/{branch}.zip",
			},
		},
	}
}
