package detect

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/kb"
)

// filterContext holds precomputed data used by filter helpers.
type filterContext struct {
	changed                map[string]bool
	changedExts            map[string]bool
	changedDependencies    map[string]bool
	changedDevDependencies map[string]bool
	manifestChanged        bool
	kb                     *kb.KnowledgeBase
}

func newFilterContext(knowledgeBase *kb.KnowledgeBase, changedFiles []string) *filterContext {
	changed := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		changed[filepath.ToSlash(filepath.Clean(f))] = true
	}

	changedExts := make(map[string]bool)
	for _, f := range changedFiles {
		if ext := filepath.Ext(f); ext != "" {
			changedExts[strings.ToLower(ext)] = true
		}
	}

	manifestNames := make(map[string]bool, len(knowledgeBase.ManifestFiles))
	for _, mf := range knowledgeBase.ManifestFiles {
		manifestNames[path.Base(filepath.ToSlash(mf))] = true
	}
	manifestChanged := false
	for file := range changed {
		if manifestNames[path.Base(filepath.ToSlash(file))] {
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
	fc.setChangedDependencies(r.Dependencies, r.Manifests)

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
	filtered.Skills = fc.filterSkills(r.Skills, changedFiles)

	if fc.manifestChanged {
		filtered.Manifests = r.Manifests
		filtered.Dependencies = r.Dependencies
	}

	return filtered
}

func (fc *filterContext) filterDetections(dets []brief.Detection) []brief.Detection {
	var result []brief.Detection
	for _, det := range dets {
		tool := fc.kb.ByName[det.Name]
		if tool != nil && fc.toolAffected(tool) {
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
			if fc.toolAffected(tool) {
				out[cat] = append(out[cat], det)
			}
		}
	}
}

func (fc *filterContext) toolAffected(tool *kb.ToolDef) bool {
	return toolMatchesChangedFiles(tool, fc.changed, fc.changedExts) || fc.dependencyAffected(tool)
}

func (fc *filterContext) setChangedDependencies(dependencies []brief.DepInfo, manifests []brief.ManifestInfo) {
	fc.changedDependencies = make(map[string]bool)
	fc.changedDevDependencies = make(map[string]bool)
	manifestByPath := make(map[string]brief.ManifestInfo, len(manifests))
	for _, manifest := range manifests {
		manifest.Path = filepath.ToSlash(filepath.Clean(manifest.Path))
		manifestByPath[manifest.Path] = manifest
	}

	devDependencyManifests := make(map[string][]brief.ManifestInfo)
	for _, dependency := range dependencies {
		if !isDevelopmentScope(dependency.Scope) {
			continue
		}
		manifest := manifestByPath[filepath.ToSlash(filepath.Clean(dependency.Manifest))]
		if manifest.Kind == "manifest" {
			devDependencyManifests[dependency.Name] = append(devDependencyManifests[dependency.Name], manifest)
		}
	}

	for _, dependency := range dependencies {
		manifest := filepath.ToSlash(filepath.Clean(dependency.Manifest))
		if !fc.changed[manifest] {
			continue
		}
		fc.changedDependencies[dependency.Name] = true
		if isDevelopmentScope(dependency.Scope) || lockfileHasDevelopmentDeclaration(
			manifestByPath[manifest], devDependencyManifests[dependency.Name],
		) {
			fc.changedDevDependencies[dependency.Name] = true
		}
	}
}

func isDevelopmentScope(scope string) bool {
	switch brief.Scope(scope) {
	case brief.ScopeDevelopment, brief.ScopeTest, brief.ScopeBuild:
		return true
	default:
		return false
	}
}

func lockfileHasDevelopmentDeclaration(lockfile brief.ManifestInfo, manifests []brief.ManifestInfo) bool {
	if lockfile.Kind != "lockfile" {
		return false
	}
	lockfileDir := path.Dir(lockfile.Path)
	for _, manifest := range manifests {
		if manifest.Ecosystem != lockfile.Ecosystem {
			continue
		}
		manifestDir := path.Dir(manifest.Path)
		if lockfileDir == "." || manifestDir == lockfileDir || strings.HasPrefix(manifestDir, lockfileDir+"/") {
			return true
		}
	}
	return false
}

func (fc *filterContext) dependencyAffected(tool *kb.ToolDef) bool {
	for _, dependency := range tool.Detect.Dependencies {
		if fc.changedDependencies[dependency] {
			return true
		}
	}
	for _, dependency := range tool.Detect.DevDependencies {
		if fc.changedDevDependencies[dependency] {
			return true
		}
	}
	return false
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

func (fc *filterContext) filterSkills(skills []brief.Skill, changedFiles []string) []brief.Skill {
	var out []brief.Skill
	for _, s := range skills {
		dir := path.Dir(s.Path) + "/"
		for _, f := range changedFiles {
			if strings.HasPrefix(filepath.ToSlash(f), dir) {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

func (fc *filterContext) filterResources(res *brief.ResourceInfo, changedFiles []string) *brief.ResourceInfo {
	if res == nil {
		return nil
	}
	changed := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		changed[strings.ToLower(filepath.ToSlash(f))] = true
	}
	hit := func(p string) bool {
		return p != "" && changed[strings.ToLower(p)]
	}

	out := &brief.ResourceInfo{}
	if hit(res.Readme) {
		out.Readme = res.Readme
	}
	if hit(res.Changelog) {
		out.Changelog = res.Changelog
	}
	if hit(res.Roadmap) {
		out.Roadmap = res.Roadmap
	}
	if hit(res.License) {
		out.License = res.License
		out.LicenseType = res.LicenseType
	}
	filterGroup := func(name string, in map[string]string) {
		for k, v := range in {
			if hit(v) {
				out.Group(name)[k] = v
			}
		}
	}
	filterGroup("legal", res.Legal)
	filterGroup("community", res.Community)
	filterGroup("security", res.Security)
	filterGroup("metadata", res.Metadata)
	filterGroup("agents", res.Agents)
	if res.Citation != nil && hit(res.Citation.Path) {
		out.Citation = res.Citation
	}
	if res.Codemeta != nil && hit(res.Codemeta.Path) {
		out.Codemeta = res.Codemeta
	}
	if res.BibTeX != nil && hit(res.BibTeX.Path) {
		out.BibTeX = res.BibTeX
	}
	if res.ReadmeBibTeX != nil && hit(res.ReadmeBibTeX.Path) {
		out.ReadmeBibTeX = res.ReadmeBibTeX
	}

	out.Templates = filterTemplates(res.Templates, hit)

	if out.Empty() {
		return nil
	}
	return out
}

func filterTemplates(t *brief.TemplateInfo, hit func(string) bool) *brief.TemplateInfo {
	if t == nil {
		return nil
	}
	ft := &brief.TemplateInfo{}
	for _, p := range t.Issue {
		if hit(p) {
			ft.Issue = append(ft.Issue, p)
		}
	}
	for _, p := range t.PullRequest {
		if hit(p) {
			ft.PullRequest = append(ft.PullRequest, p)
		}
	}
	if hit(t.Config) {
		ft.Config = t.Config
	}
	if ft.Empty() {
		return nil
	}
	return ft
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
// detection signals: config files, lockfile, detection or exclusion file
// patterns, or file_contains targets.
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
	return matchesPathPatterns(tool.Detect.Files, changed, changedExts) ||
		matchesPathPatterns(tool.Detect.ExcludeFiles, changed, changedExts) ||
		(len(tool.Detect.YAMLResources) > 0 && (changedExts[".yaml"] || changedExts[".yml"]))
}

func matchesPathPatterns(patterns []string, changed map[string]bool, changedExts map[string]bool) bool {
	for _, pattern := range patterns {
		if changed[pattern] {
			return true
		}
		if idx := strings.LastIndex(pattern, "*."); idx >= 0 {
			ext := strings.ToLower(pattern[idx+1:])
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
				if matchPathPattern(pattern, f) {
					return true
				}
			}
		}
	}
	return false
}

func matchesContentTargets(tool *kb.ToolDef, changed map[string]bool) bool {
	if matchesContentPatterns(tool.Detect.FileContains, changed) ||
		matchesContentPatterns(tool.Detect.ExcludeFileContains, changed) {
		return true
	}
	for file := range tool.Detect.KeyExists {
		if changed[file] {
			return true
		}
	}
	return false
}

func matchesContentPatterns(patterns map[string][]string, changed map[string]bool) bool {
	for pattern := range patterns {
		if changed[pattern] {
			return true
		}
		if kb.HasGlobPattern(pattern) {
			for file := range changed {
				if matchPathPattern(pattern, file) {
					return true
				}
			}
		}
	}
	return false
}
