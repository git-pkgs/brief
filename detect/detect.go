// Package detect implements the project detection engine.
package detect

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	"gopkg.in/yaml.v3"
)

// Engine runs detection against a project directory.
type Engine struct {
	KB           *kb.KnowledgeBase
	Root         string
	ScanDepth    int      // max directory depth for recursive detection (0 = default 4)
	SkipDirs     []string // additional directories to skip during walks
	filesChecked int
	toolsChecked int
	toolsMatched int

	detectedEcosystems map[string]bool // ecosystems whose language was detected

	// Lazily populated caches
	fileExts    map[string]int // cached file extension counts in the project
	depsLoaded  bool
	runtimeDeps map[string]bool // all runtime/unscoped dependency names
	devDeps     map[string]bool // development/test/build dependency names
	allDeps     map[string]bool // union of both
	parsedDeps  []brief.DepInfo // all parsed dependencies with PURLs
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

// skipDirs are directories that should never be walked during detection.
var skipDirs = map[string]bool{
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
	"deps":         true, // Elixir
	"Pods":         true, // iOS
	"third_party":  true,
	"thirdparty":   true,
	"external":     true,
	"tmp":          true,
	"temp":         true,
	"cache":        true,
}

// shouldSkipDir returns true if a directory should be skipped during walks.
func (e *Engine) shouldSkipDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	if skipDirs[name] {
		return true
	}
	for _, d := range e.SkipDirs {
		if name == d {
			return true
		}
	}
	return false
}

// New creates a detection engine for the given project root.
func New(knowledgeBase *kb.KnowledgeBase, root string) *Engine {
	return &Engine{KB: knowledgeBase, Root: root}
}

// Run performs full detection and returns a Report.
func (e *Engine) Run() (*brief.Report, error) {
	start := time.Now()

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

	report := &brief.Report{
		Version: brief.Version,
		Path:    abs,
		Tools:   make(map[string][]brief.Detection),
	}

	report.Languages = e.detectCategory("language")
	e.sortLanguagesByFileCount(report)

	// Build set of detected ecosystems from language results to filter
	// ecosystem-specific tools (prevents ExUnit matching in JS projects, etc.)
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

	report.PackageManagers = e.detectCategory("package_manager")
	report.Scripts = e.detectScripts()

	// Build a map of script names to their commands for linking
	scriptsByName := make(map[string]brief.Script)
	for _, s := range report.Scripts {
		scriptsByName[s.Name] = s
	}

	// Category names that map to common script names
	categoryScriptNames := map[string][]string{
		"test":      {"test", "spec"},
		"lint":      {"lint", "check"},
		"format":    {"format", "fmt"},
		"typecheck": {"typecheck", "types", "type-check"},
		"build":     {"build", "compile"},
		"docs":      {"docs", "doc"},
	}

	for _, cat := range e.KB.Categories() {
		if cat == "language" || cat == "package_manager" {
			continue
		}
		detections := e.detectCategory(cat)
		if len(detections) == 0 {
			continue
		}

		// Link project scripts to detected tools
		if scriptNames, ok := categoryScriptNames[cat]; ok {
			for _, sn := range scriptNames {
				script, exists := scriptsByName[sn]
				if !exists {
					continue
				}
				// Override the first tool's command with the project script
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

		report.Tools[cat] = detections
	}

	report.Style = e.detectStyle()
	report.Layout = e.detectLayout()
	report.Platforms = e.detectPlatforms()

	// Run slow detections concurrently.
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		report.Resources = e.detectResources()
	}()
	go func() {
		defer wg.Done()
		report.Git = e.detectGit(abs)
	}()
	go func() {
		defer wg.Done()
		report.Lines = e.detectLineCount(abs)
	}()
	wg.Wait()

	// Expose parsed dependencies (loadDeps was called lazily during tool matching)
	e.loadDeps()
	report.Dependencies = e.parsedDeps

	elapsed := time.Since(start)
	report.Stats = brief.Stats{
		Duration:     elapsed,
		DurationMS:   float64(elapsed.Microseconds()) / 1000.0,
		FilesChecked: e.filesChecked,
		ToolsMatched: e.toolsMatched,
		ToolsChecked: e.toolsChecked,
	}

	return report, nil
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

		if tool.Commands.Run != "" {
			d.Command = &brief.Command{
				Run:          tool.Commands.Run,
				Alternatives: tool.Commands.Alternatives,
				Source:       brief.SourceKnowledgeBase,
			}
		}

		d.ConfigFiles = e.findExisting(tool.Config.Files)

		if tool.Config.Lockfile != "" && e.exists(tool.Config.Lockfile) {
			d.Lockfile = tool.Config.Lockfile
		}

		detections = append(detections, d)
	}

	return detections
}

// matchTool checks if a tool definition matches the project.
// Returns the confidence level, or empty string if no match.
func (e *Engine) matchTool(tool *kb.ToolDef) brief.Confidence {
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
		if e.contains(file, patterns) {
			best = brief.ConfidenceHigh
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

// exists checks if a file, directory, or glob pattern matches something at the project root.
func (e *Engine) exists(pattern string) bool {
	e.filesChecked++

	if strings.HasSuffix(pattern, "/") {
		info, err := os.Stat(filepath.Join(e.Root, pattern))
		return err == nil && info.IsDir()
	}

	// Handle recursive glob patterns like "**/*.py"
	if strings.Contains(pattern, "**") {
		return e.recursiveGlob(pattern)
	}

	if kb.HasGlobPattern(pattern) {
		matches, err := filepath.Glob(filepath.Join(e.Root, pattern))
		return err == nil && len(matches) > 0
	}

	_, err := os.Stat(filepath.Join(e.Root, pattern))
	return err == nil
}

// recursiveGlob handles ** patterns by checking against the cached file extension set.
// Falls back to a bounded walk if the cache isn't populated.
func (e *Engine) recursiveGlob(pattern string) bool {
	parts := strings.SplitN(pattern, "**/", 2)
	if len(parts) != 2 {
		return false
	}
	suffix := parts[1] // e.g. "*.py"

	// Use the cached extension set for simple "**/*.ext" patterns
	if strings.HasPrefix(suffix, "*.") {
		ext := suffix[1:] // ".py"
		e.loadFileExts()
		return e.fileExts[ext] > 0
	}

	// Fall back to walk for complex patterns
	root := filepath.Join(e.Root, parts[0])
	found := false
	errDone := errors.New("found")
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name != "." && e.shouldSkipDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		matched, _ := filepath.Match(suffix, info.Name())
		if matched {
			found = true
			return errDone
		}
		return nil
	})
	return found
}

// loadFileExts walks the project to a bounded depth to collect file extensions.
// Cached for the lifetime of the engine. Default depth of 4 covers most layouts
// (src/main/java/*.java, lib/something/*.rb).
func (e *Engine) loadFileExts() {
	if e.fileExts != nil {
		return
	}
	e.fileExts = make(map[string]int)
	maxDepth := e.ScanDepth
	if maxDepth == 0 {
		maxDepth = 4
	}
	rootLen := len(e.Root)
	_ = filepath.Walk(e.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name != "." && e.shouldSkipDir(name) {
				return filepath.SkipDir
			}
			// Check depth
			rel := path[rootLen:]
			depth := strings.Count(rel, string(filepath.Separator))
			if depth > maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(info.Name())
		if ext != "" {
			e.fileExts[ext]++
		}
		return nil
	})
}

// contains checks if a file contains any of the given strings.
func (e *Engine) contains(file string, patterns []string) bool {
	data, err := os.ReadFile(filepath.Join(e.Root, file))
	if err != nil {
		return false
	}
	content := string(data)
	for _, p := range patterns {
		if strings.Contains(content, p) {
			return true
		}
	}
	return false
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

	for _, mf := range e.KB.ManifestFiles {
		path := filepath.Join(e.Root, mf)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		result, err := manifests.Parse(mf, data)
		if err != nil {
			continue
		}

		for _, dep := range result.Dependencies {
			e.allDeps[dep.Name] = true
			switch dep.Scope {
			case manifests.Development, manifests.Test, manifests.Build:
				e.devDeps[dep.Name] = true
			default:
				e.runtimeDeps[dep.Name] = true
			}
			if dep.PURL != "" {
				scope := "runtime"
				switch dep.Scope {
				case manifests.Development:
					scope = "development"
				case manifests.Test:
					scope = "test"
				case manifests.Build:
					scope = "build"
				}
				e.parsedDeps = append(e.parsedDeps, brief.DepInfo{
					Name:    dep.Name,
					Version: dep.Version,
					PURL:    dep.PURL,
					Scope:   scope,
				})
			}
		}
	}
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
	data, err := os.ReadFile(filepath.Join(e.Root, file))
	if err != nil {
		return false
	}

	ext := filepath.Ext(file)
	var parsed map[string]any

	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &parsed); err != nil {
			return false
		}
	case ".toml":
		if _, err := toml.Decode(string(data), &parsed); err != nil {
			return false
		}
	default:
		return false
	}

	for _, key := range keys {
		if lookupKeyPath(parsed, key) {
			return true
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
		if e.exists(p) {
			found = append(found, p)
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
		path := filepath.Join(e.Root, src.Source.File)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		cmd := src.Source.Command
		switch src.Source.Format {
		case "makefile":
			if cmd == "" {
				cmd = "make"
			}
			scripts = append(scripts, e.parseMakefile(data, src.Source.File, src.Source.Name, cmd)...)
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

// parseMakefile extracts targets from a Makefile. Tries make -qp for accurate
// parsing (handles includes, generated targets), falls back to regex.
func (e *Engine) parseMakefile(data []byte, file string, sourceName string, cmd string) []brief.Script {
	if _, err := exec.LookPath("make"); err == nil {
		if scripts := e.parseMakefileWithMake(file, sourceName); len(scripts) > 0 {
			return scripts
		}
	}
	return parseTargets(data, sourceName, cmd)
}

// parseMakefileWithMake uses make -qp to get an accurate list of targets.
func (e *Engine) parseMakefileWithMake(file string, sourceName string) []brief.Script {
	// make -qp exits non-zero when targets are not up to date, but
	// stdout still contains the database dump we need.
	cmd := exec.Command("make", "-qp", "-f", file)
	cmd.Dir = e.Root
	out, _ := cmd.Output()
	if len(out) == 0 {
		return nil
	}

	var scripts []brief.Script
	seen := make(map[string]bool)
	inTargets := false

	for _, line := range strings.Split(string(out), "\n") {
		// Targets appear after "# Files" section
		if strings.HasPrefix(line, "# Files") {
			inTargets = true
			continue
		}
		if !inTargets {
			continue
		}
		// Skip comments and non-target lines
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, ".") {
			continue
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			target := strings.TrimSpace(line[:idx])
			if strings.ContainsAny(target, " \t$%/") || seen[target] || target == "Makefile" || target == "makefile" || target == "GNUmakefile" {
				continue
			}
			seen[target] = true
			scripts = append(scripts, brief.Script{
				Name:   target,
				Run:    "make " + target,
				Source: sourceName,
			})
		}
	}
	return scripts
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

// inferStyle samples source files to detect indentation style.
func (e *Engine) inferStyle() *brief.StyleInfo {
	if e.KB.StyleConfig == nil {
		return nil
	}

	limit := e.KB.StyleConfig.Style.SampleLimit
	if limit == 0 {
		limit = 10
	}

	tabs, spaces2, spaces4 := 0, 0, 0
	lf, crlf := 0, 0
	sampled := 0

	exts := make(map[string]bool, len(e.KB.StyleConfig.Style.SampleExts))
	for _, ext := range e.KB.StyleConfig.Style.SampleExts {
		exts[ext] = true
	}

	errDone := errors.New("done")
	_ = filepath.Walk(e.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Skip hidden and vendor directories
			name := info.Name()
			if name != "." && e.shouldSkipDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if sampled >= limit {
			return errDone
		}

		ext := filepath.Ext(path)
		if !exts[ext] {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		sampled++
		content := string(data)
		lines := strings.Split(content, "\n")

		for _, line := range lines {
			if len(line) == 0 {
				continue
			}
			if line[0] == '\t' {
				tabs++
			} else if strings.HasPrefix(line, "    ") {
				spaces4++
			} else if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
				spaces2++
			}
		}

		if strings.Contains(content, "\r\n") {
			crlf++
		} else {
			lf++
		}

		return nil
	})

	if sampled == 0 {
		return nil
	}

	style := &brief.StyleInfo{IndentSource: "inferred"}

	switch {
	case tabs > spaces2 && tabs > spaces4:
		style.Indentation = "tabs"
	case spaces2 > spaces4:
		style.Indentation = "2-space"
	case spaces4 > 0:
		style.Indentation = "4-space"
	}

	if crlf > lf {
		style.LineEnding = "CRLF"
	} else if lf > 0 {
		style.LineEnding = "LF"
	}

	return style
}

// detectLayout checks for source and test directory patterns from the knowledge base.
func (e *Engine) detectLayout() *brief.LayoutInfo {
	if e.KB.Layouts == nil {
		return nil
	}

	layout := &brief.LayoutInfo{}

	for _, dir := range e.KB.Layouts.Layout.SourceDirs {
		if e.exists(dir + "/") {
			layout.SourceDirs = append(layout.SourceDirs, dir)
		}
	}

	for _, dir := range e.KB.Layouts.Layout.TestDirs {
		if e.exists(dir + "/") {
			layout.TestDirs = append(layout.TestDirs, dir)
		}
	}

	if len(layout.SourceDirs) == 0 && len(layout.TestDirs) == 0 {
		return nil
	}

	return layout
}

// detectResources checks for project resource files defined in the knowledge base.
func (e *Engine) detectResources() *brief.ResourceInfo {
	if len(e.KB.Resources) == 0 {
		return nil
	}

	res := &brief.ResourceInfo{}
	found := false

	for _, rd := range e.KB.Resources {
		for _, pattern := range rd.Resource.Patterns {
			if matches := e.globMatch(pattern); len(matches) > 0 {
				found = true
				match := filepath.Base(matches[0])
				switch rd.Resource.Field {
				case "readme":
					res.Readme = match
				case "contributing":
					res.Contributing = match
				case "changelog":
					res.Changelog = match
				case "license":
					res.License = match
					res.LicenseType = detectLicenseType(matches[0])
				case "security":
					res.Security = match
				}
				break
			}
		}
	}

	if !found {
		return nil
	}

	return res
}

// detectPlatforms checks for runtime version files and CI matrices.
func (e *Engine) detectPlatforms() *brief.PlatformInfo {
	platforms := &brief.PlatformInfo{
		RuntimeVersionFiles: make(map[string]string),
		CIMatrixVersions:    make(map[string][]string),
	}

	for _, rt := range e.KB.Runtimes {
		for _, file := range rt.Runtime.Files {
			path := filepath.Join(e.Root, file)
			data, err := os.ReadFile(path)
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

	for _, fp := range ci.Files {
		matches, err := filepath.Glob(filepath.Join(e.Root, fp.Pattern))
		if err != nil {
			continue
		}

		for _, path := range matches {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			var workflow map[string]any
			if err := yaml.Unmarshal(data, &workflow); err != nil {
				continue
			}

			jobs, ok := workflow["jobs"].(map[string]any)
			if !ok {
				continue
			}

			for _, job := range jobs {
				jobMap, ok := job.(map[string]any)
				if !ok {
					continue
				}

				strategy, ok := jobMap["strategy"].(map[string]any)
				if !ok {
					continue
				}

				matrix, ok := strategy["matrix"].(map[string]any)
				if !ok {
					continue
				}

				for ourKey, ciKey := range ci.MatrixKeys {
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
	}
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

// globMatch returns files matching a glob pattern relative to the project root.
func (e *Engine) globMatch(pattern string) []string {
	matches, err := filepath.Glob(filepath.Join(e.Root, pattern))
	if err != nil {
		return nil
	}
	return matches
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

	wg.Add(4)

	go func() {
		defer wg.Done()
		if out, err := e.git(absPath, "branch", "--show-current"); err == nil {
			mu.Lock()
			info.Branch = strings.TrimSpace(string(out))
			mu.Unlock()
		}
	}()

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

	go func() {
		defer wg.Done()
		if out, err := e.git(absPath, "remote"); err == nil {
			for _, name := range strings.Fields(string(out)) {
				if url, err := e.git(absPath, "remote", "get-url", name); err == nil {
					mu.Lock()
					info.Remotes[name] = strings.TrimSpace(string(url))
					mu.Unlock()
				}
			}
		}
	}()

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

// git runs a git command in the given directory and returns its output.
func (e *Engine) git(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Output()
}

// detectLineCount gets line counts using scc or tokei if available.
func (e *Engine) detectLineCount(absPath string) *brief.LineCount {
	// Try scc first
	if _, err := exec.LookPath("scc"); err == nil {
		cmd := exec.Command("scc", "--format", "json", absPath)
		if out, err := cmd.Output(); err == nil {
			return parseSCCOutput(out)
		}
	}

	// Try tokei
	if _, err := exec.LookPath("tokei"); err == nil {
		cmd := exec.Command("tokei", "--output", "json", absPath)
		if out, err := cmd.Output(); err == nil {
			return parseTokeiOutput(out)
		}
	}

	return nil
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
		Source:     "scc",
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

func rank(c brief.Confidence) int {
	switch c {
	case brief.ConfidenceHigh:
		return 3
	case brief.ConfidenceMedium:
		return 2
	case brief.ConfidenceLow:
		return 1
	default:
		return 0
	}
}
