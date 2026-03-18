package detect

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/kb"
)

// filterContext holds precomputed data used by filter helpers.
type filterContext struct {
	changed         map[string]bool
	changedExts     map[string]bool
	manifestChanged bool
	kb              *kb.KnowledgeBase
}

func newFilterContext(knowledgeBase *kb.KnowledgeBase, changedFiles []string) *filterContext {
	changed := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		changed[f] = true
	}

	changedExts := make(map[string]bool)
	for _, f := range changedFiles {
		if ext := filepath.Ext(f); ext != "" {
			changedExts[ext] = true
		}
	}

	manifestChanged := false
	for _, mf := range knowledgeBase.ManifestFiles {
		if changed[mf] {
			manifestChanged = true
			break
		}
	}

	return &filterContext{
		changed:         changed,
		changedExts:     changedExts,
		manifestChanged: manifestChanged,
		kb:              knowledgeBase,
	}
}

// FilterByChangedFiles takes a full report and returns a new report containing
// only detections relevant to the given set of changed files.
func FilterByChangedFiles(r *brief.Report, knowledgeBase *kb.KnowledgeBase, changedFiles []string) *brief.Report {
	fc := newFilterContext(knowledgeBase, changedFiles)

	filtered := &brief.Report{
		Version:      r.Version,
		Path:         r.Path,
		DiffRef:      r.DiffRef,
		DiffCommits:  r.DiffCommits,
		ChangedFiles: r.ChangedFiles,
		Tools:        make(map[string][]brief.Detection),
		Git:          r.Git,
		Stats:        r.Stats,
	}

	filtered.Languages = fc.filterDetections(r.Languages)
	filtered.PackageManagers = fc.filterPackageManagers(r.PackageManagers)
	filtered.Scripts = fc.filterScripts(r.Scripts)
	fc.filterTools(r.Tools, filtered.Tools)
	filtered.Style = fc.filterStyle(r.Style)
	filtered.Resources = fc.filterResources(r.Resources, changedFiles)
	filtered.Platforms = fc.filterPlatforms(r.Platforms, changedFiles)

	if fc.manifestChanged {
		filtered.Dependencies = r.Dependencies
	}

	return filtered
}

func (fc *filterContext) filterDetections(dets []brief.Detection) []brief.Detection {
	var result []brief.Detection
	for _, det := range dets {
		tool := fc.kb.ByName[det.Name]
		if tool != nil && toolMatchesChangedFiles(tool, fc.changed, fc.changedExts) {
			result = append(result, det)
		}
	}
	return result
}

func (fc *filterContext) filterPackageManagers(managers []brief.Detection) []brief.Detection {
	var result []brief.Detection
	for _, pm := range managers {
		tool := fc.kb.ByName[pm.Name]
		if tool == nil {
			continue
		}
		if toolMatchesChangedFiles(tool, fc.changed, fc.changedExts) || fc.manifestChanged {
			result = append(result, pm)
		}
	}
	return result
}

func (fc *filterContext) filterScripts(scripts []brief.Script) []brief.Script {
	var result []brief.Script
	for _, s := range scripts {
		for _, src := range fc.kb.ScriptSources {
			if src.Source.Name == s.Source && fc.changed[src.Source.File] {
				result = append(result, s)
				break
			}
		}
	}
	return result
}

func (fc *filterContext) filterTools(tools map[string][]brief.Detection, out map[string][]brief.Detection) {
	for cat, dets := range tools {
		for _, det := range dets {
			tool := fc.kb.ByName[det.Name]
			if tool == nil {
				continue
			}
			keep := toolMatchesChangedFiles(tool, fc.changed, fc.changedExts)
			if !keep && fc.manifestChanged && (len(tool.Detect.Dependencies) > 0 || len(tool.Detect.DevDependencies) > 0) {
				keep = true
			}
			if keep {
				out[cat] = append(out[cat], det)
			}
		}
	}
}

func (fc *filterContext) filterStyle(style *brief.StyleInfo) *brief.StyleInfo {
	if style == nil || fc.kb.StyleConfig == nil {
		return nil
	}
	for _, cf := range fc.kb.StyleConfig.Style.ConfigFiles {
		if fc.changed[cf.File] {
			return style
		}
	}
	if style.IndentSource == "inferred" {
		for ext := range fc.changedExts {
			if slices.Contains(fc.kb.StyleConfig.Style.SampleExts, ext) {
				return style
			}
		}
	}
	return nil
}

func (fc *filterContext) filterResources(res *brief.ResourceInfo, changedFiles []string) *brief.ResourceInfo {
	if res == nil {
		return nil
	}
	out := &brief.ResourceInfo{}
	found := false
	for _, f := range changedFiles {
		baseLower := strings.ToLower(filepath.Base(f))
		if res.Readme != "" && strings.ToLower(res.Readme) == baseLower {
			out.Readme = res.Readme
			found = true
		}
		if res.Contributing != "" && strings.ToLower(res.Contributing) == baseLower {
			out.Contributing = res.Contributing
			found = true
		}
		if res.Changelog != "" && strings.ToLower(res.Changelog) == baseLower {
			out.Changelog = res.Changelog
			found = true
		}
		if res.License != "" && strings.ToLower(res.License) == baseLower {
			out.License = res.License
			out.LicenseType = res.LicenseType
			found = true
		}
		if res.Security != "" && strings.ToLower(res.Security) == baseLower {
			out.Security = res.Security
			found = true
		}
	}
	if !found {
		return nil
	}
	return out
}

func (fc *filterContext) filterPlatforms(plat *brief.PlatformInfo, changedFiles []string) *brief.PlatformInfo {
	if plat == nil {
		return nil
	}
	out := &brief.PlatformInfo{
		RuntimeVersionFiles: make(map[string]string),
		CIMatrixVersions:    make(map[string][]string),
	}
	found := false
	for file, version := range plat.RuntimeVersionFiles {
		if fc.changed[file] {
			out.RuntimeVersionFiles[file] = version
			found = true
		}
	}
	for _, f := range changedFiles {
		if strings.HasPrefix(f, ".github/workflows/") {
			out.CIMatrixVersions = plat.CIMatrixVersions
			out.CIMatrixOS = plat.CIMatrixOS
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	return out
}

// toolMatchesChangedFiles checks whether any changed file is relevant to a tool's
// detection signals: config files, lockfile, detection file patterns, or
// file_contains targets.
func toolMatchesChangedFiles(tool *kb.ToolDef, changed map[string]bool, changedExts map[string]bool) bool {
	if matchesConfigFiles(tool, changed) {
		return true
	}
	if matchesDetectionPatterns(tool, changed, changedExts) {
		return true
	}
	return matchesContentTargets(tool, changed)
}

func matchesConfigFiles(tool *kb.ToolDef, changed map[string]bool) bool {
	for _, cf := range tool.Config.Files {
		if changed[cf] {
			return true
		}
	}
	return tool.Config.Lockfile != "" && changed[tool.Config.Lockfile]
}

func matchesDetectionPatterns(tool *kb.ToolDef, changed map[string]bool, changedExts map[string]bool) bool {
	for _, pattern := range tool.Detect.Files {
		if changed[pattern] {
			return true
		}
		if idx := strings.LastIndex(pattern, "*."); idx >= 0 {
			ext := pattern[idx+1:]
			if changedExts[ext] {
				return true
			}
		}
		if strings.HasSuffix(pattern, "/") {
			for f := range changed {
				if strings.HasPrefix(f, pattern) {
					return true
				}
			}
		}
		if strings.ContainsAny(pattern, "*?[") {
			for f := range changed {
				if matched, _ := filepath.Match(pattern, f); matched {
					return true
				}
			}
		}
	}
	return false
}

func matchesContentTargets(tool *kb.ToolDef, changed map[string]bool) bool {
	for file := range tool.Detect.FileContains {
		if changed[file] {
			return true
		}
	}
	for file := range tool.Detect.KeyExists {
		if changed[file] {
			return true
		}
	}
	return false
}
