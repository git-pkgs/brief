// Package detect implements the project detection engine.
package detect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/kb"
	"github.com/git-pkgs/licensecheck"
	"github.com/git-pkgs/manifests"
	"github.com/git-pkgs/spdx"
	"go.yaml.in/yaml/v3"
)

const (
	// DefaultScanDepth bounds recursive detection unless the caller overrides it.
	DefaultScanDepth = 8
	// DefaultScanLimit bounds the filesystem entries visited by one detection run.
	DefaultScanLimit = 10000
	// DefaultLineCountTimeout bounds external line counters.
	DefaultLineCountTimeout = 2 * time.Second
	// contentGlobReadLimit bounds content inspected from each glob-matched file.
	contentGlobReadLimit = 1 << 20

	microsPerMS       = 1000.0
	scanReadBatchSize = 128
	cargoManifestFile = "Cargo.toml"
	cargoLockFile     = "Cargo.lock"
	categoryBuild     = "build"
	categoryDocs      = "docs"
	categoryFormat    = "format"
	categoryLint      = "lint"
	categoryTest      = "test"
	categoryTypecheck = "typecheck"
	lineCounterSCC    = "scc"

	rankHigh   = 3
	rankMedium = 2
	rankLow    = 1
)

// Engine runs detection against a project directory.
type Engine struct {
	KB                *kb.KnowledgeBase
	Root              string
	ScanDepth         int           // optional max directory depth for recursive detection (0 = unlimited)
	ScanLimit         int           // optional max filesystem entries per scan (0 = unlimited)
	LineCountTimeout  time.Duration // optional timeout for external line counters (0 = unlimited)
	IncludeSubmodules bool          // include initialized Git submodule contents
	SkipDirs          []string      // additional directories to skip during walks
	TrackedOnly       bool          // only consider files tracked by git
	filesChecked      int
	toolsChecked      int
	toolsMatched      int

	detectedEcosystems map[string]bool // ecosystems whose language was detected

	// Lazily populated caches
	tracked              map[string]bool // git-tracked files relative to Root, nil when TrackedOnly is off
	trackedDirs          map[string]bool // directories that contain at least one tracked file
	trackedDeps          map[string]bool // whether a deps directory contains git-tracked files
	fileExts             map[string]int  // cached file extension counts in the project
	dirCache             map[string][]string
	depsLoaded           bool
	runtimeDeps          map[string]bool // all runtime/unscoped dependency names
	devDeps              map[string]bool // development/test/build dependency names
	allDeps              map[string]bool // union of both
	parsedDeps           []brief.DepInfo // direct dependencies with PURLs
	manifests            []brief.ManifestInfo
	manifestPathsCache   []string
	manifestPathsLoaded  bool
	projectFiles         []string // broad detection candidates
	projectDirs          []string
	indexedFiles         []string // includes routed hidden roots
	indexedDirs          []string
	projectFilesLoaded   bool
	submodules           []submoduleInfo
	submodulesLoaded     bool
	submoduleEntries     int
	submoduleByPath      map[string]submoduleInfo
	submoduleRoutes      map[string]bool
	includedSubmodules   []string
	includedSubmoduleSet map[string]bool
	scanTruncated        bool
	scanDepthTruncated   bool
	scanEntries          int
}

// sortLanguagesByFileCount reorders detected languages so the one with
// the most source files appears first.
func (e *Engine) sortLanguagesByFileCount(report *brief.Report) {
	if len(report.Languages) <= 1 {
		return
	}

	e.loadFileExts()

	// Score each language by summing file counts for its extensions
	scores := make(map[string]int)
	for _, lang := range report.Languages {
		tool := e.KB.ByName[lang.Name]
		if tool == nil {
			continue
		}
		for _, pattern := range tool.Detect.Files {
			// Extract extension from patterns like "*.py" or "**/*.py"
			if idx := strings.LastIndex(pattern, "*."); idx >= 0 {
				ext := pattern[idx+1:] // ".py"
				scores[lang.Name] += e.fileExts[ext]
			}
		}
	}

	sort.SliceStable(report.Languages, func(i, j int) bool {
		return scores[report.Languages[i].Name] > scores[report.Languages[j].Name]
	})
}

// defaultSkipDirs are directories that should never be walked during detection.
var defaultSkipDirs = map[string]bool{
	"vendor":       true,
	"node_modules": true,
	"__pycache__":  true,
	".bundle":      true,
	".venv":        true,
	"venv":         true,
	"target":       true, // Rust/Maven build output
	"build":        true,
	"dist":         true,
	"_build":       true, // Elixir
	"Pods":         true, // iOS
	"third_party":  true,
	"thirdparty":   true,
	"external":     true,
	"testdata":     true,
	"tmp":          true,
	"temp":         true,
	"cache":        true,
	"coverage":     true,
}

var indexedHiddenRootDirs = map[string]bool{
	".claude":  true,
	".cursor":  true,
	".forgejo": true,
	".gitea":   true,
	".github":  true,
	".gitlab":  true,
	".junie":   true,
}

// loadTracked populates the set of git-tracked files under Root by running
// git ls-files once. Paths are stored relative to Root using the OS separator.
func (e *Engine) loadTracked(abs string) error {
	out, err := e.git(abs, "ls-files", "-z")
	if err != nil {
		return fmt.Errorf("-tracked: %s is not a git repository (or git is not installed)", abs)
	}
	e.tracked = make(map[string]bool)
	e.trackedDirs = make(map[string]bool)
	e.addTrackedFiles("", out)
	if !e.IncludeSubmodules {
		return nil
	}
	e.loadSubmodules()
	for _, submodule := range e.submodules {
		if !submodule.Initialized {
			continue
		}
		if e.ScanDepth > 0 && pathDepth(submodule.Path) > e.ScanDepth {
			continue
		}
		submoduleRoot := filepath.Join(abs, submodule.Path)
		out, err := e.git(submoduleRoot, "ls-files", "-z")
		if err != nil {
			continue
		}
		e.addTrackedFiles(submodule.Path, out)
	}
	return nil
}

func (e *Engine) addTrackedFiles(prefix string, output []byte) {
	for p := range strings.SplitSeq(string(output), "\x00") {
		if p == "" {
			continue
		}
		p = filepath.Join(prefix, filepath.FromSlash(p))
		e.tracked[p] = true
		for d := filepath.Dir(p); d != "."; d = filepath.Dir(d) {
			if e.trackedDirs[d] {
				break
			}
			e.trackedDirs[d] = true
		}
	}
}

// isTracked reports whether a path relative to Root should be considered.
// Always true when TrackedOnly is off. The root itself is always allowed.
func (e *Engine) isTracked(rel string) bool {
	if e.tracked == nil {
		return true
	}
	if rel == "" || rel == "." {
		return true
	}
	return e.tracked[rel] || e.trackedDirs[rel]
}

func (e *Engine) shouldSkipDirPath(dirPath string) bool {
	name := filepath.Base(dirPath)
	if strings.HasPrefix(name, ".") {
		return true
	}
	if rel, err := filepath.Rel(e.Root, dirPath); err == nil {
		if _, ok := e.submoduleForPath(rel); ok {
			return true
		}
	}
	if defaultSkipDirs[name] {
		return true
	}
	for _, d := range e.SkipDirs {
		if name == d {
			return true
		}
	}
	if name == "deps" {
		if e.exactFileAt(filepath.Join(filepath.Dir(dirPath), "mix.exs")) {
			return true
		}
		return !e.depsDirHasTrackedFiles(dirPath)
	}
	return false
}

func (e *Engine) shouldIndexHiddenRoot(dirPath string) bool {
	name := filepath.Base(dirPath)
	if !indexedHiddenRootDirs[name] || !e.isAnalysisRootPath(filepath.Dir(dirPath)) {
		return false
	}
	for _, dir := range e.SkipDirs {
		if name == dir {
			return false
		}
	}
	return true
}

func (e *Engine) exactFileAt(filePath string) bool {
	info, err := os.Stat(filePath)
	return err == nil && !info.IsDir()
}

func (e *Engine) depsDirHasTrackedFiles(dirPath string) bool {
	rel, err := filepath.Rel(e.Root, dirPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return true
	}
	rel = filepath.Clean(rel)
	if e.tracked != nil {
		return e.trackedDirs[rel]
	}
	if e.trackedDeps == nil {
		e.trackedDeps = make(map[string]bool)
	}
	if hasTracked, ok := e.trackedDeps[rel]; ok {
		return hasTracked
	}
	gitRoot := e.Root
	gitRel := rel
	if analysisRoot := e.analysisRootFor(rel); analysisRoot != "" {
		gitRoot = filepath.Join(e.Root, analysisRoot)
		gitRel, err = filepath.Rel(analysisRoot, rel)
		if err != nil {
			return true
		}
	}
	out, err := e.git(gitRoot, "ls-files", "-z", "--", filepath.ToSlash(gitRel))
	hasTracked := err != nil || len(out) > 0
	e.trackedDeps[rel] = hasTracked
	return hasTracked
}

// New creates a detection engine for the given project root.
func New(knowledgeBase *kb.KnowledgeBase, root string) *Engine {
	return &Engine{
		KB:               knowledgeBase,
		Root:             root,
		ScanDepth:        DefaultScanDepth,
		ScanLimit:        DefaultScanLimit,
		LineCountTimeout: DefaultLineCountTimeout,
	}
}

// Run performs full detection and returns a Report.
func (e *Engine) Run() (*brief.Report, error) {
	start := time.Now()
	if err := e.validateOptions(); err != nil {
		return nil, err
	}

	abs, err := filepath.Abs(e.Root)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("path does not exist: %s", abs)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", abs)
	}

	if e.TrackedOnly {
		if err := e.loadTracked(abs); err != nil {
			return nil, err
		}
	}

	report := &brief.Report{
		Version: brief.Version,
		Path:    abs,
		Tools:   make(map[string][]brief.Detection),
	}

	report.Languages = e.detectCategory("language")
	e.sortLanguagesByFileCount(report)
	e.buildEcosystemSet(report)

	report.PackageManagers = e.detectCategory("package_manager")
	report.Scripts = e.detectScripts()
	e.detectTools(report)
	e.detectSelf(abs, report)

	report.Style = e.detectStyle()
	report.Layout = e.detectLayout(report.Languages)
	report.Platforms = e.detectPlatforms()
	report.Skills = e.detectSkills()

	// Run slow detections concurrently.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		report.Resources = e.detectResources()
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		report.Git = e.detectGit(abs)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		report.Lines = e.detectLineCount(abs)
	}()
	wg.Wait()

	// Expose parsed dependencies (loadDeps was called lazily during tool matching)
	e.loadDeps()
	report.Manifests = e.manifests
	report.Dependencies = e.parsedDeps

	elapsed := time.Since(start)
	report.Stats = brief.Stats{
		Duration:      elapsed,
		DurationMS:    float64(elapsed.Microseconds()) / microsPerMS,
		FilesChecked:  e.filesChecked,
		ToolsMatched:  e.toolsMatched,
		ToolsChecked:  e.toolsChecked,
		ScanEntries:   e.scanEntries,
		ScanTruncated: e.scanTruncated || e.scanDepthTruncated,
	}

	return report, nil
}

func (e *Engine) validateOptions() error {
	if e.ScanDepth < 0 {
		return fmt.Errorf("scan depth must not be negative")
	}
	if e.ScanLimit < 0 {
		return fmt.Errorf("scan limit must not be negative")
	}
	if e.LineCountTimeout < 0 {
		return fmt.Errorf("line count timeout must not be negative")
	}
	return nil
}

const selfModulePath = "github.com/git-pkgs/brief"

func (e *Engine) detectSelf(root string, report *brief.Report) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module ")
		if !ok {
			continue
		}
		if strings.TrimSpace(mod) != selfModulePath {
			return
		}
		report.Tools["introspection"] = append(report.Tools["introspection"], brief.Detection{
			Name:        "brief",
			Category:    "introspection",
			Confidence:  brief.ConfidenceHigh,
			ConfigFiles: []string{"go.mod"},
			Homepage:    "https://" + selfModulePath,
			Repo:        "https://" + selfModulePath,
			Description: "Describes a project's toolchain. You're looking at it.",
			Command: &brief.Command{
				Run:    "brief .",
				Source: brief.SourceKnowledgeBase,
			},
		})
		e.toolsMatched++
		return
	}
}

// buildEcosystemSet populates detectedEcosystems from language results to filter
// ecosystem-specific tools (prevents ExUnit matching in JS projects, etc.)
func (e *Engine) buildEcosystemSet(report *brief.Report) {
	e.detectedEcosystems = make(map[string]bool)
	for _, lang := range report.Languages {
		for _, tool := range e.KB.Tools {
			if tool.Tool.Name == lang.Name && tool.Tool.Category == "language" {
				for _, eco := range tool.Detect.Ecosystems {
					e.detectedEcosystems[eco] = true
				}
			}
		}
	}
}

// categoryScriptNames maps tool categories to common script names.
var categoryScriptNames = map[string][]string{
	categoryTest:      {categoryTest, "spec"},
	categoryLint:      {categoryLint, "check"},
	categoryFormat:    {categoryFormat, "fmt"},
	categoryTypecheck: {categoryTypecheck, "types", "type-check"},
	categoryBuild:     {categoryBuild, "compile"},
	categoryDocs:      {categoryDocs, "doc"},
}

// detectTools detects all tool categories and links project scripts.
func (e *Engine) detectTools(report *brief.Report) {
	scriptsByName := make(map[string]brief.Script)
	for _, s := range report.Scripts {
		scriptsByName[s.Name] = s
	}

	for _, cat := range e.KB.Categories() {
		if cat == "language" || cat == "package_manager" {
			continue
		}
		detections := e.detectCategory(cat)
		if len(detections) == 0 {
			continue
		}
		linkScriptToTool(detections, cat, scriptsByName)
		report.Tools[cat] = detections
	}
}

// linkScriptToTool overrides a tool's command with a matching project script.
func linkScriptToTool(detections []brief.Detection, cat string, scriptsByName map[string]brief.Script) {
	scriptNames, ok := categoryScriptNames[cat]
	if !ok {
		return
	}
	for _, sn := range scriptNames {
		script, exists := scriptsByName[sn]
		if !exists {
			continue
		}
		if detections[0].Command != nil {
			detections[0].Command = &brief.Command{
				Run:          script.Run,
				Source:       brief.SourceProjectScript,
				InferredTool: detections[0].Name,
			}
		}
		break
	}
}

// detectCategory finds all tools in a given category that match the project.
func (e *Engine) detectCategory(category string) []brief.Detection {
	var detections []brief.Detection

	for _, tool := range e.KB.ToolsForCategory(category) {
		e.toolsChecked++

		// Skip ecosystem-specific tools when their language wasn't detected.
		// Tools without ecosystems (shared tools like Docker, CI) always run.
		if len(tool.Detect.Ecosystems) > 0 && e.detectedEcosystems != nil {
			relevant := false
			for _, eco := range tool.Detect.Ecosystems {
				if e.detectedEcosystems[eco] {
					relevant = true
					break
				}
			}
			if !relevant {
				continue
			}
		}

		confidence := e.matchTool(tool)
		if confidence == "" {
			continue
		}
		e.toolsMatched++

		d := brief.Detection{
			Name:        tool.Tool.Name,
			Category:    tool.Tool.Category,
			Confidence:  confidence,
			Homepage:    tool.Tool.Homepage,
			Docs:        tool.Tool.Docs,
			Repo:        tool.Tool.Repo,
			Description: tool.Tool.Description,
		}

		if !tool.Taxonomy.Empty() {
			d.Taxonomy = &brief.Taxonomy{
				Role:       tool.Taxonomy.Role,
				Function:   tool.Taxonomy.Function,
				Layer:      tool.Taxonomy.Layer,
				Domain:     tool.Taxonomy.Domain,
				Audience:   tool.Taxonomy.Audience,
				Technology: tool.Taxonomy.Technology,
			}
		}

		if tool.Commands.Run != "" {
			d.Command = &brief.Command{
				Run:          tool.Commands.Run,
				Alternatives: tool.Commands.Alternatives,
				Source:       brief.SourceKnowledgeBase,
			}
		}

		d.ConfigFiles = e.findExisting(tool.Config.Files)

		if tool.Config.Lockfile != "" {
			if lockfiles := e.findExisting([]string{tool.Config.Lockfile}); len(lockfiles) > 0 {
				d.Lockfile = lockfiles[0]
			}
		}

		detections = append(detections, d)
	}

	return detections
}

// matchTool checks if a tool definition matches the project.
// Returns the confidence level, or empty string if no match.
func (e *Engine) matchTool(tool *kb.ToolDef) brief.Confidence {
	for _, pattern := range tool.Detect.ExcludeFiles {
		if e.exists(pattern) {
			return ""
		}
	}

	best := brief.Confidence("")

	for _, pattern := range tool.Detect.Files {
		if e.exists(pattern) {
			conf := brief.ConfidenceMedium
			if strings.HasSuffix(pattern, "/") {
				conf = brief.ConfidenceLow
			}
			if best == "" || rank(conf) > rank(best) {
				best = conf
			}
		}
	}

	for file, patterns := range tool.Detect.FileContains {
		if e.contentSignalMatches(file, patterns, tool.Detect.ExcludeFileContains[file]) {
			best = brief.ConfidenceHigh
		}
	}

	for _, resource := range tool.Detect.YAMLResources {
		if e.hasYAMLResource(resource) {
			best = brief.ConfidenceHigh
			break
		}
	}

	if len(tool.Detect.Dependencies) > 0 || len(tool.Detect.DevDependencies) > 0 {
		if e.hasDependency(tool) {
			best = brief.ConfidenceHigh
		}
	}

	for file, keys := range tool.Detect.KeyExists {
		if e.hasKey(file, keys) {
			conf := brief.ConfidenceMedium
			if best == "" || rank(conf) > rank(best) {
				best = conf
			}
		}
	}

	return best
}

func (e *Engine) contentSignalMatches(file string, patterns, excluded []string) bool {
	return e.contains(file, patterns) && (len(excluded) == 0 || !e.contains(file, excluded))
}

// exists checks if a file, directory, or glob pattern matches something at the project root.
// A trailing "/" means the pattern must match a directory. Glob patterns without
// a trailing "/" only match regular files so that a NEWS.d/ directory does not
// register as D source.
func (e *Engine) exists(pattern string) bool {
	e.filesChecked++

	if dir, ok := strings.CutSuffix(pattern, "/"); ok {
		if kb.HasGlobPattern(dir) {
			return e.globMatches(dir, true)
		}
		for _, root := range e.analysisRoots() {
			candidate := filepath.Join(root, filepath.FromSlash(dir))
			info, err := os.Stat(filepath.Join(e.Root, candidate))
			if err == nil && info.IsDir() && e.isTracked(candidate) {
				return true
			}
		}
		return false
	}

	// Handle recursive glob patterns like "**/*.py"
	if strings.Contains(pattern, "**") {
		return e.recursiveGlob(pattern)
	}

	if kb.HasGlobPattern(pattern) {
		return e.globMatches(pattern, false)
	}

	for _, candidate := range e.rootCandidates(pattern) {
		if e.exactFileExists(candidate) {
			return true
		}
	}
	return false
}

func (e *Engine) exactFileExists(file string) bool {
	info, err := os.Stat(filepath.Join(e.Root, filepath.FromSlash(file)))
	return err == nil && info.Mode().IsRegular() && e.isTracked(filepath.FromSlash(file))
}

func (e *Engine) rootCandidates(file string) []string {
	file = filepath.ToSlash(filepath.Clean(file))
	var candidates []string
	seen := make(map[string]bool)
	add := func(candidate string) {
		candidate = filepath.ToSlash(filepath.Clean(candidate))
		if !seen[candidate] {
			seen[candidate] = true
			candidates = append(candidates, candidate)
		}
	}
	for _, root := range e.analysisRoots() {
		add(path.Join(filepath.ToSlash(root), file))
	}
	switch file {
	case cargoManifestFile, cargoLockFile, ".cargo/config.toml":
	default:
		return candidates
	}
	for _, analysisRoot := range e.analysisRoots() {
		root, found := e.cargoManifestRootFrom(analysisRoot)
		if found {
			add(path.Join(filepath.ToSlash(root), file))
		}
	}
	return candidates
}

// globMatches reports whether a root-level glob pattern matches at least one
// entry of the requested kind.
func (e *Engine) globMatches(pattern string, wantDir bool) bool {
	e.loadProjectFiles()
	candidates := e.projectFiles
	if wantDir {
		candidates = e.projectDirs
	}
	for _, rel := range candidates {
		if e.matchesProjectPattern(pattern, rel) {
			return true
		}
	}
	return false
}

// recursiveGlob matches recursive patterns against the bounded project file index.
func (e *Engine) recursiveGlob(pattern string) bool {
	e.loadProjectFiles()
	for _, rel := range e.projectFiles {
		if e.matchesProjectPattern(pattern, rel) {
			return true
		}
	}
	return false
}

func (e *Engine) loadProjectFiles() {
	if e.projectFilesLoaded {
		return
	}
	e.projectFilesLoaded = true
	visited := 0
	e.scanProjectDir(e.Root, "", &visited, false)
	e.scanEntries = visited
	sort.Strings(e.indexedDirs)
	sort.Strings(e.indexedFiles)
	sort.Strings(e.includedSubmodules)
	sort.Strings(e.projectDirs)
	sort.Strings(e.projectFiles)
}

func (e *Engine) scanProjectDir(dirPath, relDir string, visited *int, routeOnly bool) bool {
	dir, err := os.Open(dirPath)
	if err != nil {
		e.scanTruncated = true
		return false
	}
	defer func() { _ = dir.Close() }()

	for {
		entries, readErr := dir.ReadDir(scanReadBatchSize)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			e.scanTruncated = true
			return false
		}
		for _, entry := range entries {
			if e.scanProjectEntry(dirPath, relDir, entry, visited, routeOnly) {
				return true
			}
		}
		if errors.Is(readErr, io.EOF) {
			return false
		}
	}
}

func (e *Engine) scanProjectEntry(
	dirPath, relDir string,
	entry os.DirEntry,
	visited *int,
	routeOnly bool,
) bool {
	if e.ScanLimit > 0 && *visited >= e.ScanLimit {
		e.scanTruncated = true
		return true
	}
	*visited++
	rel := filepath.Join(relDir, entry.Name())
	submoduleRoute := e.IncludeSubmodules && e.initializedSubmoduleRoute(rel)
	hiddenRoute := e.indexedHiddenRoute(rel)
	if e.scanTruncated {
		return true
	}
	if routeOnly && !submoduleRoute && !hiddenRoute {
		return false
	}
	info, err := entry.Info()
	if err != nil {
		e.scanTruncated = true
		return false
	}
	if !info.IsDir() {
		if info.Mode().IsRegular() && e.isTracked(rel) {
			e.indexedFiles = append(e.indexedFiles, rel)
			if !routeOnly {
				e.projectFiles = append(e.projectFiles, rel)
			}
		}
		return false
	}

	filePath := filepath.Join(dirPath, entry.Name())
	nextRouteOnly := routeOnly
	if slices.Contains(e.SkipDirs, entry.Name()) {
		return false
	}
	skipDir := e.shouldSkipDirPath(filePath)
	if e.scanTruncated {
		return true
	}
	if skipDir {
		if !e.shouldIndexHiddenRoot(filePath) && !submoduleRoute {
			return false
		}
		nextRouteOnly = true
	}
	if !e.isTracked(rel) {
		return false
	}
	if e.ScanDepth > 0 && pathDepth(rel) > e.ScanDepth {
		e.scanDepthTruncated = true
		return false
	}
	submodule, isSubmodule := e.submoduleForPath(rel)
	if isSubmodule && submodule.Initialized && e.IncludeSubmodules {
		e.addIncludedSubmodule(rel)
		nextRouteOnly = false
	}
	e.indexedDirs = append(e.indexedDirs, rel)
	if !nextRouteOnly {
		e.projectDirs = append(e.projectDirs, rel)
	}
	return e.scanProjectDir(filePath, rel, visited, nextRouteOnly)
}

func (e *Engine) indexedHiddenRoute(rel string) bool {
	local := filepath.Clean(e.pathAtAnalysisRoot(rel))
	if local == "." || local == "" {
		return false
	}
	name, _, _ := strings.Cut(local, string(filepath.Separator))
	return indexedHiddenRootDirs[name]
}

func pathDepth(rel string) int {
	if rel == "" || rel == "." {
		return 0
	}
	return strings.Count(filepath.Clean(rel), string(filepath.Separator)) + 1
}

func (e *Engine) loadFileExts() {
	if e.fileExts != nil {
		return
	}
	e.loadProjectFiles()
	e.fileExts = make(map[string]int)
	for _, rel := range e.projectFiles {
		if ext := filepath.Ext(rel); ext != "" {
			e.fileExts[ext]++
		}
	}
}

// safeReadFile reads a file within the project root, rejecting symlinks
// that point outside the root to prevent file disclosure attacks.
// It opens the file via O_NOFOLLOW to avoid TOCTOU races between stat and read.
func (e *Engine) safeReadFile(file string) ([]byte, error) {
	return e.safeReadFileLimit(file, 0)
}

// safeReadFileLimit applies safeReadFile's path checks and reads at most limit
// bytes. A non-positive limit reads the complete file.
func (e *Engine) safeReadFileLimit(file string, limit int64) ([]byte, error) {
	path := filepath.Join(e.Root, file)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, err
		}
		absRoot, _ := filepath.Abs(e.Root)
		if !strings.HasPrefix(target, absRoot+string(filepath.Separator)) {
			return nil, fmt.Errorf("symlink escapes project root: %s -> %s", file, target)
		}
		targetInfo, err := os.Stat(target)
		if err != nil {
			return nil, err
		}
		if !targetInfo.Mode().IsRegular() {
			return nil, fmt.Errorf("path is not a regular file: %s", file)
		}
		// Safe symlink within root: open the resolved target without following
		// a symlink swapped into place after the checks above.
		f, err := openNoFollow(target)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		return readFileLimit(f, limit)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file: %s", file)
	}
	// Not a symlink: open with O_NOFOLLOW so a symlink swap between
	// the Lstat and Open is rejected by the kernel.
	f, err := openNoFollow(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return readFileLimit(f, limit)
}

func readFileLimit(r io.Reader, limit int64) ([]byte, error) {
	if limit > 0 {
		r = io.LimitReader(r, limit)
	}
	return io.ReadAll(r)
}

// contains checks if an exact file or any regular file matching a glob contains
// one of the given strings.
func (e *Engine) contains(file string, patterns []string) bool {
	if kb.HasGlobPattern(file) {
		return e.globContains(file, patterns)
	}

	files := e.rootCandidates(file)
	if filepath.ToSlash(file) == cargoManifestFile {
		for _, manifest := range e.manifestPaths() {
			if !slices.Contains(files, manifest) && path.Base(filepath.ToSlash(manifest)) == cargoManifestFile {
				files = append(files, manifest)
			}
		}
	}

	for _, candidate := range files {
		data, err := e.safeReadFile(candidate)
		if err == nil && containsAny(string(data), patterns) {
			return true
		}
	}
	return false
}

func (e *Engine) globContains(pattern string, contentPatterns []string) bool {
	e.loadProjectFiles()
	for _, rel := range e.projectFiles {
		if !e.matchesProjectPattern(pattern, rel) {
			continue
		}
		data, err := e.safeReadFileLimit(rel, contentGlobReadLimit)
		if err == nil && containsAny(string(data), contentPatterns) {
			return true
		}
	}
	return false
}

func containsAny(content string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(content, p) {
			return true
		}
	}
	return false
}

type yamlResource struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
}

func (e *Engine) hasYAMLResource(signal kb.YAMLResourceInfo) bool {
	e.loadProjectFiles()
	for _, rel := range e.projectFiles {
		ext := strings.ToLower(filepath.Ext(rel))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		data, err := e.safeReadFileLimit(rel, contentGlobReadLimit)
		if err != nil {
			continue
		}
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		for {
			var resource yamlResource
			if err := decoder.Decode(&resource); err != nil {
				break
			}
			group, _, found := strings.Cut(resource.APIVersion, "/")
			if !found || !slices.Contains(signal.APIGroups, group) {
				continue
			}
			if len(signal.Kinds) == 0 || slices.Contains(signal.Kinds, resource.Kind) {
				return true
			}
		}
	}
	return false
}

// matchPathPattern matches slash-separated paths and treats ** as zero or more
// complete path segments.
func matchPathPattern(pattern, name string) bool {
	pattern = filepath.ToSlash(pattern)
	name = filepath.ToSlash(name)
	return matchPathSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchPathSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		if matchPathSegments(pattern[1:], name) {
			return true
		}
		return len(name) > 0 && matchPathSegments(pattern, name[1:])
	}
	if len(name) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], name[0])
	return err == nil && matched && matchPathSegments(pattern[1:], name[1:])
}

// loadDeps parses all manifest files in the project using the manifests library
// and populates the dependency caches. Called lazily on first dependency check.
func (e *Engine) loadDeps() {
	if e.depsLoaded {
		return
	}
	e.depsLoaded = true
	e.runtimeDeps = make(map[string]bool)
	e.devDeps = make(map[string]bool)
	e.allDeps = make(map[string]bool)

	manifestPaths := e.manifestPaths()

	for _, mf := range manifestPaths {
		data, err := e.safeReadFile(mf)
		if err != nil {
			continue
		}

		result, err := manifests.Parse(mf, data)
		if err != nil {
			continue
		}
		e.manifests = append(e.manifests, brief.ManifestInfo{
			Ecosystem: result.Ecosystem,
			Path:      filepath.ToSlash(mf),
			Kind:      string(result.Kind),
		})

		for _, dep := range result.Dependencies {
			e.allDeps[dep.Name] = true
			switch dep.Scope {
			case manifests.Development, manifests.Test, manifests.Build:
				e.devDeps[dep.Name] = true
			default:
				e.runtimeDeps[dep.Name] = true
			}
			// Include transitive deps from lockfiles and Go manifests.
			// Go is special: go.mod acts as both manifest and lockfile,
			// so indirect deps there are real pinned transitive deps.
			if dep.PURL == "" {
				continue
			}
			isResolved := result.Kind == manifests.Lockfile || result.Ecosystem == "golang"
			if !dep.Direct && !isResolved {
				continue
			}
			scope := brief.ScopeRuntime
			switch dep.Scope {
			case manifests.Development:
				scope = brief.ScopeDevelopment
			case manifests.Test:
				scope = brief.ScopeTest
			case manifests.Build:
				scope = brief.ScopeBuild
			}
			e.parsedDeps = append(e.parsedDeps, brief.DepInfo{
				Name:    dep.Name,
				Version: dep.Version,
				PURL:    dep.PURL,
				Scope:   scope,
				Direct:  dep.Direct,
			})
		}
	}
}

// manifestPaths returns root manifest files plus workspace member manifests.
func (e *Engine) manifestPaths() []string {
	if e.manifestPathsLoaded {
		return e.manifestPathsCache
	}
	e.manifestPathsLoaded = true

	var paths []string
	seen := make(map[string]bool)
	add := func(p string) {
		p = filepath.ToSlash(filepath.Clean(p))
		if p == "." || strings.HasPrefix(p, "../") || filepath.IsAbs(p) {
			return
		}
		if seen[p] {
			return
		}
		seen[p] = true
		paths = append(paths, p)
	}

	roots := e.analysisRoots()
	for _, root := range roots {
		for _, mf := range e.KB.ManifestFiles {
			add(path.Join(filepath.ToSlash(root), mf))
		}
	}

	e.loadProjectFiles()
	for _, rel := range e.indexedFiles {
		slashRel := filepath.ToSlash(rel)
		if e.matchesProjectPattern(".github/workflows/*.yml", rel) ||
			e.matchesProjectPattern(".github/workflows/*.yaml", rel) {
			add(slashRel)
		}
	}

	for _, root := range roots {
		cargoRoot, found := e.cargoManifestRootFrom(root)
		if found {
			add(path.Join(cargoRoot, cargoManifestFile))
			add(path.Join(cargoRoot, cargoLockFile))
			e.addCargoWorkspaceManifestsFrom(cargoRoot, add)
		}
		e.addGoWorkspaceManifestsFrom(root, add)
		e.addPackageWorkspaceManifestsFrom(root, add)
		e.addPnpmWorkspaceManifestsFrom(root, add)
	}

	e.manifestPathsCache = paths
	return paths
}

func (e *Engine) addCargoWorkspaceManifestsFrom(cargoRoot string, add func(string)) {
	data, err := e.safeReadFile(path.Join(cargoRoot, cargoManifestFile))
	if err != nil {
		return
	}

	var root struct {
		Workspace struct {
			Members []string `toml:"members"`
			Exclude []string `toml:"exclude"`
		} `toml:"workspace"`
	}
	if err := toml.Unmarshal(data, &root); err != nil {
		return
	}

	excluded := e.workspacePatternSetFrom(cargoRoot, root.Workspace.Exclude)
	for _, member := range root.Workspace.Members {
		for _, dir := range e.expandWorkspacePatternFrom(cargoRoot, member) {
			if excluded[dir] {
				continue
			}
			add(path.Join(dir, cargoManifestFile))
			lockfile := path.Join(dir, cargoLockFile)
			if e.exactFileExists(lockfile) {
				add(lockfile)
			}
		}
	}
}

// cargoManifestRootFrom returns the root Cargo manifest directory below base.
// When base has no Cargo.toml but contains Rust source, it finds the shallowest
// Cargo.toml visible to the configured scan.
func (e *Engine) cargoManifestRootFrom(base string) (string, bool) {
	base = filepath.Clean(base)
	if base == "." {
		base = ""
	}
	manifest := filepath.Join(base, cargoManifestFile)
	if e.exactFileExists(manifest) {
		return filepath.ToSlash(base), true
	}

	e.loadProjectFiles()
	hasRust := false
	for _, rel := range e.projectFiles {
		if e.analysisRootFor(rel) == base && filepath.Ext(rel) == ".rs" {
			hasRust = true
			break
		}
	}
	if !hasRust {
		return "", false
	}

	best := ""
	bestDepth := 0
	for _, rel := range e.projectFiles {
		if e.analysisRootFor(rel) != base || filepath.Base(rel) != cargoManifestFile {
			continue
		}
		dir := filepath.Dir(rel)
		localDir, err := filepath.Rel(baseOrDot(base), dir)
		if err != nil {
			continue
		}
		depth := pathDepth(localDir)
		slashDir := filepath.ToSlash(dir)
		if best == "" || depth < bestDepth || (depth == bestDepth && slashDir < best) {
			best = slashDir
			bestDepth = depth
			if depth == 1 {
				break
			}
		}
	}
	return best, best != ""
}

func baseOrDot(base string) string {
	if base == "" {
		return "."
	}
	return base
}

func (e *Engine) addGoWorkspaceManifestsFrom(base string, add func(string)) {
	data, err := e.safeReadFile(path.Join(filepath.ToSlash(base), "go.work"))
	if err != nil {
		return
	}
	for _, member := range parseGoWorkUsePaths(string(data)) {
		for _, dir := range e.expandWorkspacePatternFrom(filepath.ToSlash(base), member) {
			add(path.Join(dir, "go.mod"))
		}
	}
}

func (e *Engine) addPackageWorkspaceManifestsFrom(base string, add func(string)) {
	data, err := e.safeReadFile(path.Join(filepath.ToSlash(base), "package.json"))
	if err != nil {
		return
	}

	var root struct {
		Workspaces any `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return
	}
	for _, pattern := range packageWorkspacePatterns(root.Workspaces) {
		for _, dir := range e.expandWorkspacePatternFrom(filepath.ToSlash(base), pattern) {
			add(path.Join(dir, "package.json"))
		}
	}
}

func (e *Engine) addPnpmWorkspaceManifestsFrom(base string, add func(string)) {
	data, err := e.safeReadFile(path.Join(filepath.ToSlash(base), "pnpm-workspace.yaml"))
	if err != nil {
		return
	}

	var root struct {
		Packages []string `yaml:"packages"`
	}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return
	}

	var includes []string
	var excludes []string
	for _, pattern := range root.Packages {
		if strings.HasPrefix(pattern, "!") {
			excludes = append(excludes, strings.TrimPrefix(pattern, "!"))
			continue
		}
		includes = append(includes, pattern)
	}
	excluded := e.workspacePatternSetFrom(filepath.ToSlash(base), excludes)
	for _, pattern := range includes {
		for _, dir := range e.expandWorkspacePatternFrom(filepath.ToSlash(base), pattern) {
			if excluded[dir] {
				continue
			}
			add(path.Join(dir, "package.json"))
		}
	}
}

func packageWorkspacePatterns(workspaces any) []string {
	switch v := workspaces.(type) {
	case []any:
		var patterns []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				patterns = append(patterns, s)
			}
		}
		return patterns
	case map[string]any:
		packages, ok := v["packages"].([]any)
		if !ok {
			return nil
		}
		var patterns []string
		for _, item := range packages {
			if s, ok := item.(string); ok {
				patterns = append(patterns, s)
			}
		}
		return patterns
	default:
		return nil
	}
}

func parseGoWorkUsePaths(content string) []string {
	var paths []string
	inUseBlock := false
	for _, line := range strings.Split(content, "\n") {
		line = stripGoWorkComment(line)
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		if inUseBlock {
			var closed bool
			paths, closed = appendGoWorkUseFields(paths, fields)
			inUseBlock = !closed
			continue
		}

		if fields[0] != "use" {
			continue
		}
		if len(fields) >= 2 && fields[1] == "(" {
			var closed bool
			paths, closed = appendGoWorkUseFields(paths, fields[2:])
			inUseBlock = !closed
			continue
		}
		paths, _ = appendGoWorkUseFields(paths, fields[1:])
	}
	return paths
}

func appendGoWorkUseFields(paths []string, fields []string) ([]string, bool) {
	for _, field := range fields {
		switch field {
		case "(":
			continue
		case ")":
			return paths, true
		default:
			paths = append(paths, cleanWorkspaceMember(field))
		}
	}
	return paths, false
}

func stripGoWorkComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

func cleanWorkspaceMember(member string) string {
	member = strings.Trim(member, `"'`)
	member = strings.TrimPrefix(member, "./")
	return filepath.ToSlash(filepath.Clean(member))
}

func (e *Engine) workspacePatternSetFrom(base string, patterns []string) map[string]bool {
	set := make(map[string]bool)
	for _, pattern := range patterns {
		for _, dir := range e.expandWorkspacePatternFrom(base, pattern) {
			set[dir] = true
		}
	}
	return set
}

func (e *Engine) expandWorkspacePatternFrom(base, pattern string) []string {
	pattern = cleanWorkspaceMember(pattern)
	if pattern == "." || pattern == "" || strings.HasPrefix(pattern, "../") || filepath.IsAbs(pattern) {
		return nil
	}
	if base != "" {
		pattern = path.Join(base, pattern)
	}
	e.loadProjectFiles()
	var dirs []string
	for _, rel := range e.projectDirs {
		slashRel := filepath.ToSlash(rel)
		if matchPathPattern(pattern, slashRel) {
			dirs = append(dirs, slashRel)
		}
	}
	return dirs
}

// hasDependency checks if any of the tool's declared dependencies exist
// in the project's parsed manifests.
func (e *Engine) hasDependency(tool *kb.ToolDef) bool {
	e.loadDeps()

	for _, dep := range tool.Detect.Dependencies {
		if e.allDeps[dep] {
			return true
		}
	}
	for _, dep := range tool.Detect.DevDependencies {
		if e.devDeps[dep] {
			return true
		}
	}
	return false
}

// hasKey checks if a structured file (JSON, TOML) contains any of the given
// dot-separated key paths (e.g. "scripts.test" in package.json).
func (e *Engine) hasKey(file string, keys []string) bool {
	for _, candidate := range e.rootCandidates(file) {
		data, err := e.safeReadFile(candidate)
		if err != nil {
			continue
		}

		ext := filepath.Ext(candidate)
		var parsed map[string]any

		switch ext {
		case ".json":
			if err := json.Unmarshal(data, &parsed); err != nil {
				continue
			}
		case ".toml":
			if _, err := toml.Decode(string(data), &parsed); err != nil {
				continue
			}
		default:
			continue
		}

		for _, key := range keys {
			if lookupKeyPath(parsed, key) {
				return true
			}
		}
	}
	return false
}

// lookupKeyPath checks if a dot-separated key path exists in a nested map.
func lookupKeyPath(m map[string]any, path string) bool {
	parts := strings.Split(path, ".")
	current := any(m)

	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return false
		}
		current, ok = obj[part]
		if !ok {
			return false
		}
	}
	return true
}

// findExisting returns which of the given paths actually exist in the project.
func (e *Engine) findExisting(paths []string) []string {
	var found []string
	for _, p := range paths {
		if kb.HasGlobPattern(p) || strings.HasSuffix(p, "/") {
			if e.exists(p) {
				found = append(found, p)
			}
			continue
		}
		for _, candidate := range e.rootCandidates(p) {
			if e.exactFileExists(candidate) {
				found = append(found, candidate)
			}
		}
	}
	return found
}

// detectScripts finds project-defined scripts using the script source definitions
// from the knowledge base. Results are grouped by source so the human report can
// print section headers without interleaving.
func (e *Engine) detectScripts() []brief.Script {
	var scripts []brief.Script

	for _, src := range e.KB.ScriptSources {
		data, err := e.safeReadFile(src.Source.File)
		if err != nil {
			continue
		}

		cmd := src.Source.Command
		switch src.Source.Format {
		case "makefile":
			if cmd == "" {
				cmd = "make"
			}
			scripts = append(scripts, e.parseMakefile(data, src.Source.Name, cmd)...)
		case "targets":
			if cmd == "" {
				cmd = src.Source.Name
			}
			scripts = append(scripts, parseTargets(data, src.Source.Name, cmd)...)
		case "json_scripts":
			scripts = append(scripts, parseJSONScripts(data, src.Source.Name)...)
		case "yaml_tasks":
			if cmd == "" {
				cmd = "task"
			}
			scripts = append(scripts, parseYAMLTasks(data, src.Source.Name, cmd)...)
		}
	}

	sort.SliceStable(scripts, func(i, j int) bool {
		return scripts[i].Source < scripts[j].Source
	})

	return scripts
}

// parseMakefile extracts targets from a Makefile using static parsing.
// We intentionally avoid running make -qp because it executes $(shell ...)
// directives, which is an RCE vector when scanning untrusted repositories.
func (e *Engine) parseMakefile(data []byte, sourceName string, cmd string) []brief.Script {
	return parseTargets(data, sourceName, cmd)
}

// parseYAMLTasks extracts task names from Taskfile.yml format.
func parseYAMLTasks(data []byte, sourceName string, cmd string) []brief.Script {
	var taskfile struct {
		Tasks map[string]any `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(data, &taskfile); err != nil {
		return nil
	}

	var scripts []brief.Script
	for name := range taskfile.Tasks {
		scripts = append(scripts, brief.Script{
			Name:   name,
			Run:    cmd + " " + name,
			Source: sourceName,
		})
	}
	return scripts
}

// parseTargets extracts targets from files with "target:" syntax (Makefile, Justfile).
func parseTargets(data []byte, sourceName string, cmd string) []brief.Script {
	var scripts []brief.Script
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ".") {
			continue
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			target := strings.TrimSpace(line[:idx])
			if strings.ContainsAny(target, " \t$%") {
				continue
			}
			scripts = append(scripts, brief.Script{
				Name:   target,
				Run:    cmd + " " + target,
				Source: sourceName,
			})
		}
	}

	return scripts
}

// parseJSONScripts extracts scripts from a package.json-style file.
func parseJSONScripts(data []byte, sourceName string) []brief.Script {
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	var scripts []brief.Script
	for name, cmd := range pkg.Scripts {
		scripts = append(scripts, brief.Script{
			Name:   name,
			Run:    cmd,
			Source: sourceName,
		})
	}
	return scripts
}

// detectStyle checks for style configuration files defined in the knowledge base.
func (e *Engine) detectStyle() *brief.StyleInfo {
	if e.KB.StyleConfig == nil {
		return nil
	}

	style := &brief.StyleInfo{}
	found := false

	for _, cf := range e.KB.StyleConfig.Style.ConfigFiles {
		if e.exists(cf.File) {
			found = true
			if cf.Provides == "indentation" || cf.Provides == "all" {
				style.Indentation = "configured"
				style.IndentSource = cf.SourceName
			}
		}
	}

	if !found {
		style = e.inferStyle()
	}

	if style == nil {
		return nil
	}
	if style.Indentation == "" && style.LineEnding == "" && style.TrailingNewline == nil {
		return nil
	}

	return style
}

// styleCounts tracks indentation and line ending counts during sampling.
type styleCounts struct {
	tabs, spaces2, spaces4 int
	lf, crlf               int
	sampled                int
}

func (sc *styleCounts) addFile(data []byte) {
	sc.sampled++
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if len(line) == 0 {
			continue
		}
		switch {
		case line[0] == '\t':
			sc.tabs++
		case strings.HasPrefix(line, "    "):
			sc.spaces4++
		case strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   "):
			sc.spaces2++
		}
	}
	if strings.Contains(content, "\r\n") {
		sc.crlf++
	} else {
		sc.lf++
	}
}

func (sc *styleCounts) toStyleInfo() *brief.StyleInfo {
	if sc.sampled == 0 {
		return nil
	}
	style := &brief.StyleInfo{IndentSource: "inferred"}
	switch {
	case sc.tabs > sc.spaces2 && sc.tabs > sc.spaces4:
		style.Indentation = "tabs"
	case sc.spaces2 > sc.spaces4:
		style.Indentation = "2-space"
	case sc.spaces4 > 0:
		style.Indentation = "4-space"
	}
	if sc.crlf > sc.lf {
		style.LineEnding = "CRLF"
	} else if sc.lf > 0 {
		style.LineEnding = "LF"
	}
	return style
}

// inferStyle samples source files to detect indentation style.
func (e *Engine) inferStyle() *brief.StyleInfo {
	if e.KB.StyleConfig == nil {
		return nil
	}

	limit := e.KB.StyleConfig.Style.SampleLimit
	if limit == 0 {
		limit = 10
	}

	exts := make(map[string]bool, len(e.KB.StyleConfig.Style.SampleExts))
	for _, ext := range e.KB.StyleConfig.Style.SampleExts {
		exts[ext] = true
	}

	var sc styleCounts
	e.loadProjectFiles()
	for _, rel := range e.projectFiles {
		if sc.sampled >= limit {
			break
		}
		if !exts[filepath.Ext(rel)] {
			continue
		}
		data, err := e.safeReadFile(rel)
		if err != nil {
			continue
		}
		sc.addFile(data)
	}

	return sc.toStyleInfo()
}

// detectLayout checks for source and test directory patterns from the knowledge base.
func (e *Engine) detectLayout(languages []brief.Detection) *brief.LayoutInfo {
	if e.KB.Layouts == nil {
		return nil
	}

	layout := &brief.LayoutInfo{}

	for _, dir := range e.KB.Layouts.Layout.SourceDirs {
		layout.SourceDirs = append(layout.SourceDirs, e.layoutDirsNamed(dir)...)
	}

	for _, dir := range e.KB.Layouts.Layout.TestDirs {
		layout.TestDirs = append(layout.TestDirs, e.layoutDirsNamed(dir)...)
	}

	if len(layout.SourceDirs) == 0 {
		layout.SourceDirs = e.inferFlatLayout(languages, layout.TestDirs)
	}

	if len(layout.SourceDirs) == 0 && len(layout.TestDirs) == 0 {
		return nil
	}

	return layout
}

// inferFlatLayout finds top-level directories that hold source for the primary
// detected language when no conventional source directory (src/, lib/, etc.)
// exists. This covers projects where the package directory is named after the
// project rather than a generic name.
func (e *Engine) inferFlatLayout(languages []brief.Detection, testDirs []string) []string {
	if len(languages) == 0 {
		return nil
	}

	exts := e.languageExtensions(languages[0].Name)
	if len(exts) == 0 {
		return nil
	}

	skip := make(map[string]bool)
	for _, d := range e.KB.Layouts.Layout.ExcludeDirs {
		skip[d] = true
	}
	for _, d := range e.KB.Layouts.Layout.TestDirs {
		skip[d] = true
	}
	for _, d := range testDirs {
		skip[d] = true
	}

	e.loadProjectFiles()
	var found []string
	for _, rel := range e.projectDirs {
		local := e.pathAtAnalysisRoot(rel)
		if local == "." || strings.Contains(local, string(filepath.Separator)) {
			continue
		}
		name := filepath.Base(local)
		if skip[name] {
			continue
		}
		if e.projectDirHasExtension(rel, exts) {
			found = append(found, filepath.ToSlash(rel))
		}
	}
	return found
}

func (e *Engine) layoutDirsNamed(name string) []string {
	e.loadProjectFiles()
	var found []string
	for _, rel := range e.projectDirs {
		if filepath.ToSlash(e.pathAtAnalysisRoot(rel)) == name {
			found = append(found, filepath.ToSlash(rel))
		}
	}
	return found
}

// languageExtensions returns the file extensions a language tool definition
// matches on, derived from its "*.ext" detection patterns.
func (e *Engine) languageExtensions(name string) []string {
	tool := e.KB.ByName[name]
	if tool == nil {
		return nil
	}
	seen := make(map[string]bool)
	var exts []string
	for _, pattern := range tool.Detect.Files {
		if idx := strings.LastIndex(pattern, "*."); idx >= 0 {
			ext := pattern[idx+1:]
			if !seen[ext] {
				seen[ext] = true
				exts = append(exts, ext)
			}
		}
	}
	return exts
}

func (e *Engine) projectDirHasExtension(dir string, exts []string) bool {
	for _, file := range e.projectFiles {
		if filepath.Dir(file) != dir {
			continue
		}
		for _, want := range exts {
			if filepath.Ext(file) == want {
				return true
			}
		}
	}
	return false
}

// detectResources checks for project resource files defined in the knowledge base.
func (e *Engine) detectResources() *brief.ResourceInfo {
	if len(e.KB.Resources) == 0 {
		return nil
	}

	res := &brief.ResourceInfo{}

	for _, rd := range e.KB.Resources {
		abs, rel := e.findResource(rd.Resource)
		if rel == "" {
			continue
		}
		if rd.Resource.Group != "" {
			if g := res.Group(rd.Resource.Group); g != nil {
				g[rd.Resource.Field] = rel
			}
			continue
		}
		switch rd.Resource.Field {
		case "readme":
			res.Readme = rel
		case "changelog":
			res.Changelog = rel
		case "roadmap":
			res.Roadmap = rel
		case "license":
			res.License = rel
			res.LicenseType = detectLicenseType(abs)
		}
	}

	res.Templates = e.detectTemplates()

	if res.Empty() {
		return nil
	}
	return res
}

var (
	templateBaseDirs = []string{".", ".github", ".gitea", ".forgejo", ".gitlab", "docs"}
	templateExts     = map[string]bool{".md": true, ".yaml": true, ".yml": true, ".txt": true}
)

// detectTemplates finds issue and pull/merge request templates across the
// locations recognised by GitHub, GitLab, Gitea and Forgejo. Both single-file
// templates and template directories are checked.
func (e *Engine) detectTemplates() *brief.TemplateInfo {
	t := &brief.TemplateInfo{}
	for _, base := range templateBaseDirs {
		for _, name := range e.dirDirs(base) {
			lower := strings.ToLower(name)
			rel := name
			if base != "." {
				rel = path.Join(base, name)
			}
			switch lower {
			case "issue_template", "issue_templates":
				e.collectTemplates(rel, &t.Issue, &t.Config)
			case "pull_request_template", "merge_request_templates":
				e.collectTemplates(rel, &t.PullRequest, nil)
			}
		}
		for _, name := range e.dirFiles(base) {
			lower := strings.ToLower(name)
			if !templateExts[path.Ext(lower)] {
				continue
			}
			rel := name
			if base != "." {
				rel = path.Join(base, name)
			}
			switch strings.TrimSuffix(lower, path.Ext(lower)) {
			case "issue_template":
				t.Issue = append(t.Issue, rel)
			case "pull_request_template":
				t.PullRequest = append(t.PullRequest, rel)
			}
		}
	}
	sort.Strings(t.Issue)
	sort.Strings(t.PullRequest)
	if t.Empty() {
		return nil
	}
	return t
}

// collectTemplates lists template files in dir, separating the issue chooser
// config.yml from actual templates.
func (e *Engine) collectTemplates(dir string, into *[]string, config *string) {
	for _, name := range e.dirFiles(dir) {
		lower := strings.ToLower(name)
		if !templateExts[path.Ext(lower)] {
			continue
		}
		rel := path.Join(dir, name)
		if config != nil && (lower == "config.yml" || lower == "config.yaml") {
			if *config == "" {
				*config = rel
			}
			continue
		}
		*into = append(*into, rel)
	}
}

// findResource searches for the first file matching any of the resource's
// patterns, in the repo root and then each configured subdirectory. Matching
// is case-insensitive. It returns the absolute path and the path relative to
// the repo root.
func (e *Engine) findResource(r kb.ResourceInfo) (abs, rel string) {
	dirs := append([]string{"."}, r.Dirs...)
	for _, dir := range dirs {
		entries := e.dirFiles(dir)
		for _, pattern := range r.Patterns {
			lp := strings.ToLower(pattern)
			for _, name := range entries {
				if ok, _ := filepath.Match(lp, strings.ToLower(name)); !ok {
					continue
				}
				relPath := name
				if dir != "." {
					relPath = path.Join(dir, name)
				}
				return filepath.Join(e.Root, filepath.FromSlash(relPath)), relPath
			}
		}
	}
	return "", ""
}

var skillFrontmatterDelim = []byte("---")

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// detectSkills looks for agent skill definitions the project provides.
func (e *Engine) detectSkills() []brief.Skill {
	var skills []brief.Skill
	e.loadProjectFiles()
	for _, glob := range []string{"skills/*/SKILL.md", ".claude/skills/*/SKILL.md"} {
		for _, rel := range e.indexedFiles {
			if !e.matchesProjectPattern(glob, rel) {
				continue
			}
			skills = append(skills, e.parseSkill(filepath.ToSlash(rel)))
		}
	}
	return skills
}

// parseSkill reads a SKILL.md file and extracts name/description from its
// YAML frontmatter. Falls back to the parent directory name if frontmatter is
// missing or unparseable.
func (e *Engine) parseSkill(rel string) brief.Skill {
	skill := brief.Skill{
		Name:   path.Base(path.Dir(rel)),
		Path:   rel,
		Format: "claude",
	}
	data, err := e.safeReadFile(rel)
	if err != nil {
		return skill
	}
	if !bytes.HasPrefix(data, skillFrontmatterDelim) {
		return skill
	}
	rest := bytes.TrimLeft(data[len(skillFrontmatterDelim):], "\r\n")
	end := bytes.Index(rest, []byte("\n---"))
	if end == -1 {
		return skill
	}
	var fm skillFrontmatter
	if yaml.Unmarshal(rest[:end], &fm) == nil {
		if fm.Name != "" {
			skill.Name = fm.Name
		}
		skill.Description = fm.Description
	}
	return skill
}

// dirFiles returns the regular file names in dir (relative to e.Root),
// caching results per directory.
func (e *Engine) dirFiles(dir string) []string {
	if e.dirCache == nil {
		e.dirCache = map[string][]string{}
	}
	if cached, ok := e.dirCache[dir]; ok {
		return cached
	}
	e.loadProjectFiles()
	names := directProjectNames(e.indexedFiles, dir)
	e.dirCache[dir] = names
	return names
}

func (e *Engine) dirDirs(dir string) []string {
	e.loadProjectFiles()
	return directProjectNames(e.indexedDirs, dir)
}

func directProjectNames(entries []string, dir string) []string {
	dir = path.Clean(filepath.ToSlash(dir))
	var names []string
	for _, entry := range entries {
		entry = filepath.ToSlash(entry)
		if path.Dir(entry) == dir {
			names = append(names, path.Base(entry))
		}
	}
	return names
}

// detectPlatforms checks for runtime version files and CI matrices.
func (e *Engine) detectPlatforms() *brief.PlatformInfo {
	platforms := &brief.PlatformInfo{
		RuntimeVersionFiles: make(map[string]string),
		CIMatrixVersions:    make(map[string][]string),
	}

	for _, rt := range e.KB.Runtimes {
		for _, file := range rt.Runtime.Files {
			data, err := e.safeReadFile(file)
			if err != nil {
				continue
			}
			version := strings.TrimSpace(string(data))
			if version != "" {
				platforms.RuntimeVersionFiles[file] = version
			}
		}
	}

	// Parse CI matrices
	if e.KB.CIConfig != nil {
		e.parseCIMatrices(platforms)
	}

	if len(platforms.RuntimeVersionFiles) == 0 &&
		len(platforms.CIMatrixVersions) == 0 &&
		len(platforms.CIMatrixOS) == 0 {
		return nil
	}

	return platforms
}

// parseCIMatrices extracts version matrices from CI configuration files.
func (e *Engine) parseCIMatrices(platforms *brief.PlatformInfo) {
	ci := e.KB.CIConfig.CI

	e.loadProjectFiles()
	for _, fp := range ci.Files {
		for _, rel := range e.indexedFiles {
			rel = filepath.ToSlash(rel)
			if matchPathPattern(fp.Pattern, rel) {
				e.parseCIWorkflow(filepath.Join(e.Root, filepath.FromSlash(rel)), ci.MatrixKeys, platforms)
			}
		}
	}
}

func (e *Engine) parseCIWorkflow(path string, matrixKeys map[string]string, platforms *brief.PlatformInfo) {
	rel, err := filepath.Rel(e.Root, path)
	if err != nil {
		return
	}
	data, err := e.safeReadFile(rel)
	if err != nil {
		return
	}

	var workflow map[string]any
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return
	}

	jobs, ok := workflow["jobs"].(map[string]any)
	if !ok {
		return
	}

	for _, job := range jobs {
		matrix := extractJobMatrix(job)
		if matrix == nil {
			continue
		}
		for ourKey, ciKey := range matrixKeys {
			values, ok := matrix[ciKey]
			if !ok {
				continue
			}
			versions := toStringSlice(values)
			if len(versions) == 0 {
				continue
			}
			if ourKey == "os" {
				platforms.CIMatrixOS = append(platforms.CIMatrixOS, versions...)
			} else {
				platforms.CIMatrixVersions[ourKey] = append(
					platforms.CIMatrixVersions[ourKey], versions...,
				)
			}
		}
	}
}

// extractJobMatrix pulls the strategy.matrix map from a job definition.
func extractJobMatrix(job any) map[string]any {
	jobMap, ok := job.(map[string]any)
	if !ok {
		return nil
	}
	strategy, ok := jobMap["strategy"].(map[string]any)
	if !ok {
		return nil
	}
	matrix, ok := strategy["matrix"].(map[string]any)
	if !ok {
		return nil
	}
	return matrix
}

// toStringSlice converts a YAML value (string or []any) to []string.
func toStringSlice(v any) []string {
	switch val := v.(type) {
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			result = append(result, fmt.Sprint(item))
		}
		return result
	case string:
		return []string{val}
	default:
		return []string{fmt.Sprint(val)}
	}
}

// detectGit extracts git repository metadata by shelling out to git.
// Returns nil if git is not installed or the directory is not a git repo.
func (e *Engine) detectGit(absPath string) *brief.GitInfo {
	if _, err := exec.LookPath("git"); err != nil {
		return nil
	}

	// Check if this is a git repo
	if out, err := e.git(absPath, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil
	}

	info := &brief.GitInfo{
		Remotes: make(map[string]string),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	wg.Add(1)
	go func() {
		defer wg.Done()
		if out, err := e.git(absPath, "branch", "--show-current"); err == nil {
			mu.Lock()
			info.Branch = strings.TrimSpace(string(out))
			mu.Unlock()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if out, err := e.git(absPath, "rev-parse", "--abbrev-ref", "origin/HEAD"); err == nil {
			ref := strings.TrimSpace(string(out))
			if after, ok := strings.CutPrefix(ref, "origin/"); ok {
				mu.Lock()
				info.DefaultBranch = after
				mu.Unlock()
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if out, err := e.git(absPath, "remote"); err == nil {
			for _, name := range strings.Fields(string(out)) {
				if url, err := e.git(absPath, "remote", "get-url", name); err == nil {
					mu.Lock()
					info.Remotes[name] = redactURL(strings.TrimSpace(string(url)))
					mu.Unlock()
				}
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if out, err := e.git(absPath, "rev-list", "--count", "HEAD"); err == nil {
			var count int
			if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &count); err == nil {
				mu.Lock()
				info.CommitCount = count
				mu.Unlock()
			}
		}
	}()

	wg.Wait()

	if info.Branch == "" && len(info.Remotes) == 0 {
		return nil
	}

	return info
}

const redactedPlaceholder = "REDACTED"

var scpURLUserinfo = regexp.MustCompile(`^[^@/]+(:[^@/]*)?@`)

// redactURL strips embedded credentials from a git remote URL so they don't
// end up in reports or terminal scrollback. Tokens can appear as either the
// password or the username (e.g. https://<pat>@github.com/...), so the whole
// userinfo section is replaced rather than relying on url.Redacted.
func redactURL(raw string) string {
	if !strings.Contains(raw, "@") {
		return raw
	}

	if u, err := url.Parse(raw); err == nil && u.User != nil {
		if redactUserinfo(u.User) {
			u.User = url.User(redactedPlaceholder)
			return u.String()
		}
		return raw
	}

	// scp-like syntax (user@host:path) that url.Parse can't handle.
	if loc := scpURLUserinfo.FindStringIndex(raw); loc != nil {
		userinfo := raw[:loc[1]-1]
		if strings.Contains(userinfo, ":") || looksLikeToken(userinfo) {
			return redactedPlaceholder + "@" + raw[loc[1]:]
		}
	}

	return raw
}

func redactUserinfo(u *url.Userinfo) bool {
	if _, hasPassword := u.Password(); hasPassword {
		return true
	}
	return looksLikeToken(u.Username())
}

var tokenPrefixes = []string{
	"github_pat_", "ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_",
	"glpat-", "gldt-", "glrt-", "glsoat-", "glcbt-",
	"ATCTT", "BBDC-",
}

func looksLikeToken(s string) bool {
	if s == "" {
		return false
	}
	for _, p := range tokenPrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	// Heuristic: long alphanumeric blobs used as bare usernames are almost
	// certainly access tokens rather than real account names.
	const suspiciousLen = 24
	if len(s) < suspiciousLen {
		return false
	}
	for _, r := range s {
		if r == '-' || r == '_' || r == '.' {
			continue
		}
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

// git runs a git command in the given directory and returns its output.
func (e *Engine) git(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Output()
}

// detectLineCount gets line counts using scc or tokei if available.
func (e *Engine) detectLineCount(absPath string) *brief.LineCount {
	e.loadProjectFiles()
	if e.scanTruncated || e.scanDepthTruncated {
		return nil
	}
	roots := e.analysisRoots()
	ctx := context.Background()
	cancel := func() {}
	if e.LineCountTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, e.LineCountTimeout)
	}
	defer cancel()

	if _, err := exec.LookPath(lineCounterSCC); err == nil {
		if count, timedOut, ok := e.countRoots(ctx, lineCounterSCC, absPath, roots); ok {
			return count
		} else if timedOut {
			return nil
		}
	}

	if _, err := exec.LookPath("tokei"); err == nil {
		if count, _, ok := e.countRoots(ctx, "tokei", absPath, roots); ok {
			return count
		}
	}

	return nil
}

func (e *Engine) countRoots(
	ctx context.Context,
	name, absPath string,
	roots []string,
) (*brief.LineCount, bool, bool) {
	var total *brief.LineCount
	for _, root := range roots {
		rootPath := filepath.Join(absPath, root)
		var args []string
		switch name {
		case lineCounterSCC:
			args = e.sccArgs(rootPath, root)
		case "tokei":
			args = e.tokeiArgs(rootPath, root)
		}
		out, err := exec.CommandContext(ctx, name, args...).Output()
		if err != nil {
			return nil, ctx.Err() != nil, false
		}
		var count *brief.LineCount
		if name == lineCounterSCC {
			count = parseSCCOutput(out)
		} else {
			count = parseTokeiOutput(out)
		}
		if count == nil {
			return nil, false, false
		}
		total = mergeLineCounts(total, count)
	}
	return total, false, true
}

func mergeLineCounts(total, count *brief.LineCount) *brief.LineCount {
	if total == nil {
		total = &brief.LineCount{ByLanguage: make(map[string]int), Source: count.Source}
	}
	total.TotalFiles += count.TotalFiles
	total.TotalLines += count.TotalLines
	for language, lines := range count.ByLanguage {
		total.ByLanguage[language] += lines
	}
	return total
}

func (e *Engine) sccArgs(absPath, root string) []string {
	dirs := e.lineCountExcludedDirs(absPath, root)
	return []string{"--format", "json", "--exclude-dir", strings.Join(dirs, ","), absPath}
}

func (e *Engine) lineCountExcludedDirs(absPath, root string) []string {
	excluded := make(map[string]bool)
	for dir := range defaultSkipDirs {
		excluded[dir] = true
	}
	for _, dir := range e.SkipDirs {
		if dir != "" {
			excluded[dir] = true
		}
	}
	for _, dir := range []string{".git", ".hg", ".svn"} {
		excluded[dir] = true
	}
	depsPath := filepath.Join(absPath, "deps")
	logicalDepsPath := filepath.Join(e.Root, root, "deps")
	if info, err := os.Stat(depsPath); err == nil && info.IsDir() && e.shouldSkipDirPath(logicalDepsPath) {
		excluded["deps"] = true
	}

	dirs := make([]string, 0, len(excluded))
	for dir := range excluded {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs
}

func (e *Engine) tokeiArgs(absPath, root string) []string {
	args := []string{"--output", "json"}
	excluded := make(map[string]bool)
	for _, dir := range e.lineCountExcludedDirs(absPath, root) {
		excluded[filepath.ToSlash(dir)+"/"] = true
	}
	for _, submodule := range e.directSubmodulePaths(root) {
		excluded[submodule+"/"] = true
	}
	patterns := make([]string, 0, len(excluded))
	for pattern := range excluded {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	for _, pattern := range patterns {
		args = append(args, "--exclude", pattern)
	}
	return append(args, absPath)
}

// parseSCCOutput parses scc --format json output.
func parseSCCOutput(data []byte) *brief.LineCount {
	var results []struct {
		Name  string `json:"Name"`
		Lines int    `json:"Lines"`
		Code  int    `json:"Code"`
		Count int    `json:"Count"`
	}
	if err := json.Unmarshal(data, &results); err != nil {
		return nil
	}

	lc := &brief.LineCount{
		ByLanguage: make(map[string]int),
		Source:     lineCounterSCC,
	}
	for _, r := range results {
		lc.TotalFiles += r.Count
		lc.TotalLines += r.Code
		if r.Code > 0 {
			lc.ByLanguage[r.Name] = r.Code
		}
	}
	return lc
}

// parseTokeiOutput parses tokei --output json output.
func parseTokeiOutput(data []byte) *brief.LineCount {
	var results map[string]struct {
		Code    int `json:"code"`
		Blanks  int `json:"blanks"`
		Reports []struct {
			Stats struct {
				Code int `json:"code"`
			} `json:"stats"`
		} `json:"reports"`
	}
	if err := json.Unmarshal(data, &results); err != nil {
		return nil
	}

	lc := &brief.LineCount{
		ByLanguage: make(map[string]int),
		Source:     "tokei",
	}
	for lang, info := range results {
		lc.TotalFiles += len(info.Reports)
		lc.TotalLines += info.Code
		if info.Code > 0 {
			lc.ByLanguage[lang] = info.Code
		}
	}
	return lc
}

// detectLicenseType reads a license file and identifies its SPDX license type.
func detectLicenseType(path string) string {
	// Reject symlinks to prevent file disclosure
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}

	cov := licensecheck.Scan(data)
	if len(cov.Match) == 0 {
		return ""
	}

	id := cov.Match[0].ID
	// Normalize to a valid SPDX identifier
	normalized, err := spdx.Normalize(id)
	if err != nil {
		return id // return raw ID if normalization fails
	}
	return normalized
}

// recommendedCategories are tool categories that every project benefits from.
var recommendedCategories = map[string]string{
	categoryTest:      "Test",
	categoryLint:      "Lint",
	categoryFormat:    "Format",
	categoryTypecheck: "Typecheck",
	categoryDocs:      "Docs",
}

// Missing computes which recommended tool categories have no detected tools
// for the project's ecosystems. It requires Run() to have been called first
// so that detectedEcosystems is populated.
func (e *Engine) Missing(r *brief.Report) *brief.MissingReport {
	mr := &brief.MissingReport{
		Version: brief.Version,
		Path:    r.Path,
	}

	for eco := range e.detectedEcosystems {
		mr.Ecosystems = append(mr.Ecosystems, eco)
	}
	sort.Strings(mr.Ecosystems)

	// Build set of categories that were actually detected.
	detected := make(map[string]bool)
	for cat := range r.Tools {
		detected[cat] = true
	}

	// Check each recommended category against each detected ecosystem.
	categoryOrder := []string{categoryTest, categoryLint, categoryFormat, categoryTypecheck, categoryDocs}
	for _, cat := range categoryOrder {
		if detected[cat] {
			continue
		}

		label := recommendedCategories[cat]
		best, bestEco := e.findBestTool(cat, mr.Ecosystems)
		if best == nil {
			continue
		}
		mc := brief.MissingCategory{
			Category:    cat,
			Label:       label,
			Ecosystem:   bestEco,
			Suggested:   best.Tool.Name,
			Description: best.Tool.Description,
			Docs:        best.Tool.Docs,
		}
		if best.Commands.Run != "" {
			mc.SuggestedCmd = best.Commands.Run
		}
		mr.Missing = append(mr.Missing, mc)
	}

	return mr
}

// findBestTool returns the best tool for a category across ecosystems.
// Prefers tools marked as default, falls back to the first match.
func (e *Engine) findBestTool(category string, ecosystems []string) (*kb.ToolDef, string) {
	var best *kb.ToolDef
	var bestEco string
	for _, eco := range ecosystems {
		for _, tool := range e.KB.ToolsForEcosystem(eco) {
			if tool.Tool.Category != category {
				continue
			}
			if best == nil {
				best = tool
				bestEco = eco
			}
			if tool.Tool.Default {
				return tool, eco
			}
		}
	}
	return best, bestEco
}

var confidenceRank = map[brief.Confidence]int{
	brief.ConfidenceHigh:   rankHigh,
	brief.ConfidenceMedium: rankMedium,
	brief.ConfidenceLow:    rankLow,
}

func rank(c brief.Confidence) int {
	return confidenceRank[c]
}
