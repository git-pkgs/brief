// Package brief detects a software project's toolchain, configuration,
// and conventions, then outputs a structured report.
package brief

import "time"

// Version is set at build time via ldflags.
var Version = "dev"

// Confidence indicates how reliable a detection signal is.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// Source indicates where a detected command came from.
type Source string

const (
	SourceProjectScript Source = "project_script"
	SourceKnowledgeBase Source = "knowledge_base"
	SourceConfigFile    Source = "config_file"
)

// Command is a runnable command with provenance.
type Command struct {
	Run          string   `json:"run"`
	Alternatives []string `json:"alternatives,omitempty"`
	Source       Source   `json:"source"`
	InferredTool string   `json:"inferred_tool,omitempty"`
}

// Detection is a single detected tool or feature.
type Detection struct {
	Name        string     `json:"name"`
	Category    string     `json:"category"`
	Confidence  Confidence `json:"confidence"`
	Command     *Command   `json:"command,omitempty"`
	ConfigFiles []string   `json:"config_files,omitempty"`
	Lockfile    string     `json:"lockfile,omitempty"`
	Homepage    string     `json:"homepage,omitempty"`
	Docs        string     `json:"docs,omitempty"`
	Repo        string     `json:"repo,omitempty"`
	Description string     `json:"description,omitempty"`
}

// Script is a project-defined task (Makefile target, package.json script, etc.).
type Script struct {
	Name   string `json:"name"`
	Run    string `json:"run"`
	Source string `json:"source"` // e.g. "Makefile", "package.json"
}

// StyleInfo describes detected coding style conventions.
type StyleInfo struct {
	Indentation     string `json:"indentation,omitempty"`   // e.g. "2-space", "4-space", "tabs"
	IndentSource    string `json:"indent_source,omitempty"` // e.g. "editorconfig", "inferred"
	LineEnding      string `json:"line_ending,omitempty"`   // "LF" or "CRLF"
	TrailingNewline *bool  `json:"trailing_newline,omitempty"`
}

// LayoutInfo describes the project's file layout conventions.
type LayoutInfo struct {
	SourceDirs    []string `json:"source_dirs,omitempty"` // e.g. ["src/", "lib/", "app/"]
	TestDirs      []string `json:"test_dirs,omitempty"`   // e.g. ["spec/", "test/"]
	MirrorsSource bool     `json:"mirrors_source,omitempty"`
}

// PlatformInfo describes detected platforms and runtime versions.
type PlatformInfo struct {
	RuntimeVersionFiles map[string]string   `json:"runtime_version_files,omitempty"`
	CIMatrixVersions    map[string][]string `json:"ci_matrix_versions,omitempty"`
	CIMatrixOS          []string            `json:"ci_matrix_os,omitempty"`
}

// ResourceInfo describes project resource files.
type ResourceInfo struct {
	Readme       string `json:"readme,omitempty"`
	Contributing string `json:"contributing,omitempty"`
	Changelog    string `json:"changelog,omitempty"`
	License      string `json:"license,omitempty"`
	LicenseType  string `json:"license_type,omitempty"`
	Security     string `json:"security,omitempty"`
}

// GitInfo describes the git repository state.
type GitInfo struct {
	Branch        string            `json:"branch,omitempty"`
	DefaultBranch string            `json:"default_branch,omitempty"`
	Remotes       map[string]string `json:"remotes,omitempty"` // name -> URL
	CommitCount   int               `json:"commit_count,omitempty"`
}

// LineCount holds line count information.
type LineCount struct {
	TotalFiles int            `json:"total_files"`
	TotalLines int            `json:"total_lines"`
	ByLanguage map[string]int `json:"by_language,omitempty"`
	Source     string         `json:"source"` // "scc", "tokei", or "fallback"
}

// Stats holds performance and coverage metrics from the detection run.
type Stats struct {
	Duration     time.Duration `json:"-"`
	DurationMS   float64       `json:"duration_ms"`
	FilesChecked int           `json:"files_checked"`
	ToolsMatched int           `json:"tools_matched"`
	ToolsChecked int           `json:"tools_checked"`
}

// DepInfo is a parsed dependency from a manifest file.
type DepInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	PURL    string `json:"purl"`
	Scope   string `json:"scope,omitempty"` // "runtime", "development", "test", "build"
	Direct  bool   `json:"direct"`
}

// EnrichmentInfo holds metadata fetched from external sources about the project itself.
type EnrichmentInfo struct {
	Repo       *RepoEnrichment       `json:"repo,omitempty"`
	Packages   []PublishedPackage     `json:"packages,omitempty"`
	RuntimeEOL map[string]*RuntimeEOL `json:"runtime_eol,omitempty"`
}

// RepoEnrichment holds metadata about the project's own repository.
type RepoEnrichment struct {
	Scorecard     float64 `json:"scorecard,omitempty"`
	ScorecardDate string  `json:"scorecard_date,omitempty"`
}

// PublishedPackage describes a package this repo publishes to a registry.
type PublishedPackage struct {
	Name                   string `json:"name"`
	Ecosystem              string `json:"ecosystem"`
	PURL                   string `json:"purl"`
	LatestVersion          string `json:"latest_version,omitempty"`
	License                string `json:"license,omitempty"`
	Description            string `json:"description,omitempty"`
	Downloads              int    `json:"downloads,omitempty"`
	DownloadsPeriod        string `json:"downloads_period,omitempty"`
	DependentPackagesCount int    `json:"dependent_packages_count,omitempty"`
	DependentReposCount    int    `json:"dependent_repos_count,omitempty"`
	RegistryURL            string `json:"registry_url,omitempty"`
}

// RuntimeEOL holds end-of-life status for a runtime version.
type RuntimeEOL struct {
	EOL       string `json:"eol,omitempty"`    // date string or "true"/"false"
	Supported bool   `json:"supported"`
	LTS       bool   `json:"lts,omitempty"`
	Latest    string `json:"latest,omitempty"` // latest patch version
}

// Report is the complete output of a brief analysis.
type Report struct {
	Version         string                 `json:"version"`
	Path            string                 `json:"path"`
	Languages       []Detection            `json:"languages"`
	PackageManagers []Detection            `json:"package_managers"`
	Scripts         []Script               `json:"scripts,omitempty"`
	Tools           map[string][]Detection `json:"tools"`
	Style           *StyleInfo             `json:"style,omitempty"`
	Layout          *LayoutInfo            `json:"layout,omitempty"`
	Platforms       *PlatformInfo          `json:"platforms,omitempty"`
	Resources       *ResourceInfo          `json:"resources,omitempty"`
	Git             *GitInfo               `json:"git,omitempty"`
	Lines           *LineCount             `json:"lines,omitempty"`
	Dependencies    []DepInfo              `json:"dependencies,omitempty"`
	Enrichment      *EnrichmentInfo        `json:"enrichment,omitempty"`
	Stats           Stats                  `json:"stats"`
}
