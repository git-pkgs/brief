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

// Stats holds performance and coverage metrics from the detection run.
type Stats struct {
	Duration     time.Duration `json:"-"`
	DurationMS   float64       `json:"duration_ms"`
	FilesChecked int           `json:"files_checked"`
	ToolsMatched int           `json:"tools_matched"`
	ToolsChecked int           `json:"tools_checked"`
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
	Stats           Stats                  `json:"stats"`
}
