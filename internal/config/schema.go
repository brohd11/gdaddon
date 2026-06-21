package config

// SourceConfig is a declarative provider entry. It may carry a search rule
// (interpreted by internal/search to satisfy search.Source) and/or a vcs rule
// (interpreted by internal/source to list an addon's versions) — so one entry
// describes everything gdaddon knows about a provider, and a new store or VCS
// host can be added in YAML without a Go backend.
type SourceConfig struct {
	Name   string      `yaml:"name"`           // display label in the source picker
	Type   string      `yaml:"type,omitempty"` // "json" for search providers; omitted for vcs-only entries
	Auth   string      `yaml:"auth,omitempty"` // "" | "github" → send Bearer $GITHUB_TOKEN (search)
	Search *SearchRule `yaml:"search,omitempty"`
	Detail *DetailRule `yaml:"detail,omitempty"`
	VCS    *VCSRule    `yaml:"vcs,omitempty"`
}

// VCSRule tells internal/source how to list an addon's versions on one host. It
// is indexed by Host, so a repo URL (from a manifest entry or a search result)
// resolves to the rule whose Host matches the URL's domain. Templates use the
// placeholders {owner} {repo} {tag} {branch}, substituted verbatim.
type VCSRule struct {
	Host             string       `yaml:"host"`           // index key, e.g. "github.com"
	Auth             string       `yaml:"auth,omitempty"` // "github" → Bearer $GITHUB_TOKEN
	Releases         ReleasesRule `yaml:"releases"`
	Branches         BranchesRule `yaml:"branches,omitempty"`
	SourceArchive    ArchiveSpec  `yaml:"source_archive,omitempty"`     // appended to every release
	BranchArchiveURL string       `yaml:"branch_archive_url,omitempty"` // when a manifest URL tracks refs/heads/<branch>
}

// ReleasesRule extracts releases from a host's release-list endpoint. AssetsPath
// is relative to each release element. AssetSuffix (default ".zip") keeps only
// matching downloadable assets.
type ReleasesRule struct {
	URL            string `yaml:"url"`
	ResultsPath    string `yaml:"results_path,omitempty"` // "" = top-level array
	TagPath        string `yaml:"tag_path"`
	PrereleasePath string `yaml:"prerelease_path,omitempty"`
	AssetsPath     string `yaml:"assets_path,omitempty"`
	AssetNamePath  string `yaml:"asset_name_path,omitempty"`
	AssetURLPath   string `yaml:"asset_url_path,omitempty"`
	AssetSuffix    string `yaml:"asset_suffix,omitempty"`
}

// BranchesRule extracts branch names from a host's branch-list endpoint and maps
// each to a HEAD archive download. ArchiveURL templates {branch}.
type BranchesRule struct {
	URL         string `yaml:"url"`
	ResultsPath string `yaml:"results_path,omitempty"`
	NamePath    string `yaml:"name_path"`
	ArchiveURL  string `yaml:"archive_url"`
}

// ArchiveSpec is the generated source-archive download offered for every release.
// URL templates {tag}.
type ArchiveSpec struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// SearchRule describes how to fetch and parse a page of results. URL is a
// template; {query}, {page} and {godot_version} are substituted (see
// internal/search/template.go). Extraction is by dotted JSON paths.
type SearchRule struct {
	URL         string     `yaml:"url"`
	PageBase    int        `yaml:"page_base,omitempty"`     // value of {page} for the first page (0 or 1)
	OmitIfEmpty []string   `yaml:"omit_if_empty,omitempty"` // drop these query params when their value is empty
	ResultsPath string     `yaml:"results_path"`            // dotted path to the result array
	Fields      FieldPaths `yaml:"fields"`                  // dotted paths within each array element
	PagePath    string     `yaml:"page_path,omitempty"`     // dotted path → current page number
	PagesPath   string     `yaml:"pages_path,omitempty"`    // dotted path → total pages
	TotalPath   string     `yaml:"total_path,omitempty"`    // dotted path → total item count
	PerPage     int        `yaml:"per_page,omitempty"`      // used to derive Pages from TotalPath when PagesPath is unset
}

// FieldPaths maps each Summary field to a dotted JSON path within a result
// element. Empty paths are skipped.
type FieldPaths struct {
	ID            string `yaml:"id,omitempty"`
	Title         string `yaml:"title,omitempty"`
	Author        string `yaml:"author,omitempty"`
	Category      string `yaml:"category,omitempty"`
	Cost          string `yaml:"cost,omitempty"`
	GodotVersion  string `yaml:"godot_version,omitempty"`
	VersionString string `yaml:"version_string,omitempty"`
}

// DetailRule describes the per-asset fetch that yields the repo URL. URL is a
// template with {id} (the Summary.ID from search). BrowseURLPath is the only
// load-bearing field — it must resolve to a URL the installer accepts.
type DetailRule struct {
	URL             string `yaml:"url"`
	BrowseURLPath   string `yaml:"browse_url_path"`
	DownloadURLPath string `yaml:"download_url_path,omitempty"`
	DescriptionPath string `yaml:"description_path,omitempty"`
	TitlePath       string `yaml:"title_path,omitempty"`
	AuthorPath      string `yaml:"author_path,omitempty"`
}
