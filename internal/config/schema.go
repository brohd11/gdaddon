package config

// SourceConfig is a declarative provider: a search rule (internal/search) and/or a vcs
// rule (internal/source), so a new host can be added in YAML.
type SourceConfig struct {
	Name   string      `yaml:"name"`           // display label in the source picker
	Type   string      `yaml:"type,omitempty"` // "json" for search providers; omitted for vcs-only entries
	Search *SearchRule `yaml:"search,omitempty"`
	Detail *DetailRule `yaml:"detail,omitempty"`
	VCS    *VCSRule    `yaml:"vcs,omitempty"`
}

// VCSRule tells internal/source how to list versions on one host, matched by Host.
// Templates use {owner} {repo} {tag} {branch} {commit}.
type VCSRule struct {
	Host             string       `yaml:"host"` // index key, e.g. "github.com"
	Releases         ReleasesRule `yaml:"releases"`
	Tags             TagsRule     `yaml:"tags,omitempty"`
	Branches         BranchesRule `yaml:"branches,omitempty"`
	SourceArchive    ArchiveSpec  `yaml:"source_archive,omitempty"`     // appended to every release
	BranchArchiveURL string       `yaml:"branch_archive_url,omitempty"` // when a manifest URL tracks refs/heads/<branch>
	CommitArchiveURL string       `yaml:"commit_archive_url,omitempty"` // archive for a specific commit; templates {commit} — pins a branch install to its HEAD sha
}

// TagsRule lists Git tags independently of published releases. Pagination follows
// the API's Link header, as it does for explicit release lookups.
type TagsRule struct {
	URL         string `yaml:"url"`
	ResultsPath string `yaml:"results_path,omitempty"`
	NamePath    string `yaml:"name_path"`
}

// ReleasesRule extracts releases from a release-list endpoint. AssetsPath is relative to
// each release; AssetSuffix (default ".zip") filters assets.
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

// BranchesRule extracts branches and maps each to an archive (ArchiveURL, {branch}).
// CommitPath locates the HEAD sha, used with CommitArchiveURL to pin a commit.
type BranchesRule struct {
	URL         string `yaml:"url"`
	ResultsPath string `yaml:"results_path,omitempty"`
	NamePath    string `yaml:"name_path"`
	ArchiveURL  string `yaml:"archive_url"`
	CommitPath  string `yaml:"commit_path,omitempty"`
}

// ArchiveSpec is the generated source-archive download offered for every release.
// URL templates {tag}.
type ArchiveSpec struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// SearchRule fetches and parses a results page. URL is a template ({query}, {page},
// {godot_version}); extraction uses dotted JSON paths.
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

// DetailRule fetches an asset's detail ({id}); BrowseURLPath must yield an installable
// url.
type DetailRule struct {
	URL             string `yaml:"url"`
	BrowseURLPath   string `yaml:"browse_url_path"`
	DownloadURLPath string `yaml:"download_url_path,omitempty"`
	DescriptionPath string `yaml:"description_path,omitempty"`
	TitlePath       string `yaml:"title_path,omitempty"`
	AuthorPath      string `yaml:"author_path,omitempty"`
}
