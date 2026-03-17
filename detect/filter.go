package detect

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/kb"
)

// FilterByChangedFiles takes a full report and returns a new report containing
// only detections relevant to the given set of changed files. A detection is
// relevant if a changed file matches one of its config files, lockfile,
// detection file patterns, or file extensions (for languages).
func FilterByChangedFiles(r *brief.Report, knowledgeBase *kb.KnowledgeBase, changedFiles []string) *brief.Report {
	changed := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		changed[f] = true
	}

	// Collect extensions from changed files.
	changedExts := make(map[string]bool)
	for _, f := range changedFiles {
		if ext := filepath.Ext(f); ext != "" {
			changedExts[ext] = true
		}
	}

	// Check if any manifest files changed (signals dependency changes).
	manifestChanged := false
	for _, mf := range knowledgeBase.ManifestFiles {
		if changed[mf] {
			manifestChanged = true
			break
		}
	}

	filtered := &brief.Report{
		Version:      r.Version,
		Path:         r.Path,
		DiffRef:      r.DiffRef,
		ChangedFiles: r.ChangedFiles,
		Tools:        make(map[string][]brief.Detection),
		Git:          r.Git,
		Stats:        r.Stats,
	}

	// Languages: keep if any changed file has a matching extension.
	for _, lang := range r.Languages {
		tool := knowledgeBase.ByName[lang.Name]
		if tool == nil {
			continue
		}
		if toolMatchesChangedFiles(tool, changed, changedExts) {
			filtered.Languages = append(filtered.Languages, lang)
		}
	}

	// Package managers: keep if config/lockfile changed or manifests changed.
	for _, pm := range r.PackageManagers {
		tool := knowledgeBase.ByName[pm.Name]
		if tool == nil {
			continue
		}
		if toolMatchesChangedFiles(tool, changed, changedExts) || manifestChanged {
			filtered.PackageManagers = append(filtered.PackageManagers, pm)
		}
	}

	// Scripts: keep if the script source file changed.
	scriptSourceFiles := make(map[string]bool)
	for _, src := range knowledgeBase.ScriptSources {
		scriptSourceFiles[src.Source.File] = true
	}
	for _, s := range r.Scripts {
		// Find the source file for this script's source name.
		for _, src := range knowledgeBase.ScriptSources {
			if src.Source.Name == s.Source && changed[src.Source.File] {
				filtered.Scripts = append(filtered.Scripts, s)
				break
			}
		}
	}

	// Tools: keep if config files, detection files, or dependencies changed.
	for cat, tools := range r.Tools {
		for _, det := range tools {
			tool := knowledgeBase.ByName[det.Name]
			if tool == nil {
				continue
			}
			keep := toolMatchesChangedFiles(tool, changed, changedExts)
			// Also keep if this tool is dependency-detected and a manifest changed.
			if !keep && manifestChanged && (len(tool.Detect.Dependencies) > 0 || len(tool.Detect.DevDependencies) > 0) {
				keep = true
			}
			if keep {
				filtered.Tools[cat] = append(filtered.Tools[cat], det)
			}
		}
	}

	// Style: keep if any style config file changed.
	if r.Style != nil && knowledgeBase.StyleConfig != nil {
		for _, cf := range knowledgeBase.StyleConfig.Style.ConfigFiles {
			if changed[cf.File] {
				filtered.Style = r.Style
				break
			}
		}
		// Also keep if source files changed (style could be re-inferred).
		if filtered.Style == nil && r.Style.IndentSource == "inferred" {
			for ext := range changedExts {
				if slices.Contains(knowledgeBase.StyleConfig.Style.SampleExts, ext) {
					filtered.Style = r.Style
					break
				}
			}
		}
	}

	// Resources: keep if the resource file itself changed.
	if r.Resources != nil {
		res := &brief.ResourceInfo{}
		found := false
		for _, f := range changedFiles {
			base := filepath.Base(f)
			baseLower := strings.ToLower(base)
			if r.Resources.Readme != "" && strings.ToLower(r.Resources.Readme) == baseLower {
				res.Readme = r.Resources.Readme
				found = true
			}
			if r.Resources.Contributing != "" && strings.ToLower(r.Resources.Contributing) == baseLower {
				res.Contributing = r.Resources.Contributing
				found = true
			}
			if r.Resources.Changelog != "" && strings.ToLower(r.Resources.Changelog) == baseLower {
				res.Changelog = r.Resources.Changelog
				found = true
			}
			if r.Resources.License != "" && strings.ToLower(r.Resources.License) == baseLower {
				res.License = r.Resources.License
				res.LicenseType = r.Resources.LicenseType
				found = true
			}
			if r.Resources.Security != "" && strings.ToLower(r.Resources.Security) == baseLower {
				res.Security = r.Resources.Security
				found = true
			}
		}
		if found {
			filtered.Resources = res
		}
	}

	// Platforms: keep runtime version files that changed, CI config that changed.
	if r.Platforms != nil {
		plat := &brief.PlatformInfo{
			RuntimeVersionFiles: make(map[string]string),
			CIMatrixVersions:    make(map[string][]string),
		}
		found := false
		for file, version := range r.Platforms.RuntimeVersionFiles {
			if changed[file] {
				plat.RuntimeVersionFiles[file] = version
				found = true
			}
		}
		// CI matrix: keep if any workflow file changed.
		for _, f := range changedFiles {
			if strings.HasPrefix(f, ".github/workflows/") {
				plat.CIMatrixVersions = r.Platforms.CIMatrixVersions
				plat.CIMatrixOS = r.Platforms.CIMatrixOS
				found = true
				break
			}
		}
		if found {
			filtered.Platforms = plat
		}
	}

	// Dependencies: keep if manifests changed.
	if manifestChanged {
		filtered.Dependencies = r.Dependencies
	}

	return filtered
}

// toolMatchesChangedFiles checks whether any changed file is relevant to a tool's
// detection signals: config files, lockfile, detection file patterns, or
// file_contains targets.
func toolMatchesChangedFiles(tool *kb.ToolDef, changed map[string]bool, changedExts map[string]bool) bool {
	// Check config files.
	for _, cf := range tool.Config.Files {
		if changed[cf] {
			return true
		}
	}

	// Check lockfile.
	if tool.Config.Lockfile != "" && changed[tool.Config.Lockfile] {
		return true
	}

	// Check detection file patterns.
	for _, pattern := range tool.Detect.Files {
		// Direct file match.
		if changed[pattern] {
			return true
		}
		// Extension-based pattern like "*.py" or "**/*.py".
		if idx := strings.LastIndex(pattern, "*."); idx >= 0 {
			ext := pattern[idx+1:] // ".py"
			if changedExts[ext] {
				return true
			}
		}
		// Directory pattern like "src/" - check if any changed file is under it.
		if strings.HasSuffix(pattern, "/") {
			for f := range changed {
				if strings.HasPrefix(f, pattern) {
					return true
				}
			}
		}
		// Glob match individual changed files.
		if strings.ContainsAny(pattern, "*?[") {
			for f := range changed {
				if matched, _ := filepath.Match(pattern, f); matched {
					return true
				}
			}
		}
	}

	// Check file_contains targets.
	for file := range tool.Detect.FileContains {
		if changed[file] {
			return true
		}
	}

	// Check key_exists targets.
	for file := range tool.Detect.KeyExists {
		if changed[file] {
			return true
		}
	}

	return false
}
