// Package detect implements the project detection engine.
package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/kb"
)

// Engine runs detection against a project directory.
type Engine struct {
	KB   *kb.KnowledgeBase
	Root string
}

// New creates a detection engine for the given project root.
func New(knowledgeBase *kb.KnowledgeBase, root string) *Engine {
	return &Engine{KB: knowledgeBase, Root: root}
}

// Run performs full detection and returns a Report.
func (e *Engine) Run() (*brief.Report, error) {
	abs, err := filepath.Abs(e.Root)
	if err != nil {
		return nil, err
	}

	report := &brief.Report{
		Version: brief.Version,
		Path:    abs,
		Tools:   make(map[string][]brief.Detection),
	}

	report.Languages = e.detectCategory("language")
	report.PackageManagers = e.detectCategory("package_manager")
	report.Scripts = e.detectScripts()

	for _, cat := range e.KB.Categories() {
		if cat == "language" || cat == "package_manager" {
			continue
		}
		if detections := e.detectCategory(cat); len(detections) > 0 {
			report.Tools[cat] = detections
		}
	}

	report.Style = e.detectStyle()
	report.Layout = e.detectLayout()
	report.Resources = e.detectResources()
	report.Platforms = e.detectPlatforms()

	return report, nil
}

// detectCategory finds all tools in a given category that match the project.
func (e *Engine) detectCategory(category string) []brief.Detection {
	var detections []brief.Detection

	for _, tool := range e.KB.ToolsForCategory(category) {
		confidence := e.matchTool(tool)
		if confidence == "" {
			continue
		}

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

	return best
}

// exists checks if a file, directory, or glob pattern matches something at the project root.
func (e *Engine) exists(pattern string) bool {
	if strings.HasSuffix(pattern, "/") {
		info, err := os.Stat(filepath.Join(e.Root, pattern))
		return err == nil && info.IsDir()
	}

	if kb.HasGlobPattern(pattern) {
		matches, err := filepath.Glob(filepath.Join(e.Root, pattern))
		return err == nil && len(matches) > 0
	}

	_, err := os.Stat(filepath.Join(e.Root, pattern))
	return err == nil
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

// hasDependency checks if any declared dependency names appear in the project's manifest files.
// Uses the manifest file list from the knowledge base config.
func (e *Engine) hasDependency(tool *kb.ToolDef) bool {
	for _, mf := range e.KB.ManifestFiles {
		data, err := os.ReadFile(filepath.Join(e.Root, mf))
		if err != nil {
			continue
		}
		content := string(data)
		for _, dep := range tool.Detect.Dependencies {
			if strings.Contains(content, dep) {
				return true
			}
		}
		for _, dep := range tool.Detect.DevDependencies {
			if strings.Contains(content, dep) {
				return true
			}
		}
	}
	return false
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
// from the knowledge base.
func (e *Engine) detectScripts() []brief.Script {
	var scripts []brief.Script

	for _, src := range e.KB.ScriptSources {
		path := filepath.Join(e.Root, src.Source.File)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		switch src.Source.Format {
		case "makefile":
			scripts = append(scripts, parseMakefile(data, src.Source.Name)...)
		case "json_scripts":
			scripts = append(scripts, parseJSONScripts(data, src.Source.Name)...)
		}
	}

	return scripts
}

// parseMakefile extracts phony targets from a Makefile.
func parseMakefile(data []byte, sourceName string) []brief.Script {
	var scripts []brief.Script
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Match lines like "target:" that aren't comments or variable assignments
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ".") {
			continue
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			target := strings.TrimSpace(line[:idx])
			// Skip targets with variables or spaces
			if strings.ContainsAny(target, " \t$%") {
				continue
			}
			scripts = append(scripts, brief.Script{
				Name:   target,
				Run:    "make " + target,
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
				style.IndentSource = cf.SourceName
			}
		}
	}

	if !found {
		// Infer from source files
		style = e.inferStyle()
	}

	if style != nil && style.Indentation == "" && style.IndentSource == "" {
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

	for _, ext := range e.KB.StyleConfig.Style.SampleExts {
		pattern := filepath.Join(e.Root, "**", "*"+ext)
		// Use a simple directory walk instead of doublestar glob
		_ = filepath.Walk(e.Root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || sampled >= limit {
				return err
			}
			if !strings.HasSuffix(path, ext) {
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
		_ = pattern // suppress unused warning
	}

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

// detectPlatforms checks for runtime version files defined in the knowledge base.
func (e *Engine) detectPlatforms() *brief.PlatformInfo {
	if len(e.KB.Runtimes) == 0 {
		return nil
	}

	platforms := &brief.PlatformInfo{
		RuntimeVersionFiles: make(map[string]string),
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

	if len(platforms.RuntimeVersionFiles) == 0 {
		return nil
	}

	return platforms
}

// globMatch returns files matching a glob pattern relative to the project root.
func (e *Engine) globMatch(pattern string) []string {
	matches, err := filepath.Glob(filepath.Join(e.Root, pattern))
	if err != nil {
		return nil
	}
	return matches
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
