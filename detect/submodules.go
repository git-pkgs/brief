package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/git-pkgs/brief/kb"
)

type submoduleInfo struct {
	Path        string
	Parent      string
	Initialized bool
}

func (e *Engine) loadSubmodules() {
	if e.submodulesLoaded {
		return
	}
	e.submodulesLoaded = true
	if e.fsOnly {
		return
	}
	e.loadSubmodulesFrom(e.Root, "", make(map[string]bool))
	sort.Slice(e.submodules, func(i, j int) bool {
		return e.submodules[i].Path < e.submodules[j].Path
	})
	e.submoduleByPath = make(map[string]submoduleInfo, len(e.submodules))
	e.submoduleRoutes = make(map[string]bool)
	for _, submodule := range e.submodules {
		e.submoduleByPath[submodule.Path] = submodule
		if !submodule.Initialized {
			continue
		}
		for route := submodule.Path; route != "."; route = filepath.Dir(route) {
			e.submoduleRoutes[route] = true
		}
	}
}

func (e *Engine) loadSubmodulesFrom(root, parent string, seen map[string]bool) {
	modulesFile := filepath.Join(root, ".gitmodules")
	info, err := os.Lstat(modulesFile)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	absRoot, err := filepath.Abs(root)
	if err != nil || seen[absRoot] {
		return
	}
	seen[absRoot] = true

	cmd := exec.Command("git", "config", "--null", "--file", ".gitmodules", "--get-regexp", `^submodule\..*\.path$`)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return
	}
	for record := range strings.SplitSeq(string(out), "\x00") {
		if record == "" {
			continue
		}
		if e.IncludeSubmodules && e.ScanLimit > 0 && e.submoduleEntries >= e.ScanLimit {
			e.scanTruncated = true
			return
		}
		e.submoduleEntries++
		_, configuredPath, ok := strings.Cut(record, "\n")
		if !ok {
			continue
		}
		child, ok := cleanSubmodulePath(configuredPath)
		if !ok {
			continue
		}
		fullPath := filepath.Clean(filepath.Join(parent, child))
		childRoot := filepath.Join(root, child)
		initialized := e.IncludeSubmodules && initializedSubmoduleDir(childRoot, root)
		e.submodules = append(e.submodules, submoduleInfo{
			Path:        fullPath,
			Parent:      parent,
			Initialized: initialized,
		})
		if initialized && e.IncludeSubmodules &&
			(e.ScanDepth == 0 || pathDepth(fullPath) <= e.ScanDepth) {
			e.loadSubmodulesFrom(childRoot, fullPath, seen)
		}
	}
}

func cleanSubmodulePath(value string) (string, bool) {
	value = filepath.FromSlash(strings.TrimSpace(value))
	if value == "" || filepath.IsAbs(value) {
		return "", false
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", false
	}
	return cleaned, true
}

func initializedSubmoduleDir(root, parent string) bool {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	out, err := exec.Command("git", "-C", root, "rev-parse", "--show-superproject-working-tree").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return false
	}
	superproject, err := os.Stat(filepath.FromSlash(strings.TrimSpace(string(out))))
	if err != nil {
		return false
	}
	parentInfo, err := os.Stat(parent)
	return err == nil && os.SameFile(superproject, parentInfo)
}

func (e *Engine) submoduleForPath(rel string) (submoduleInfo, bool) {
	e.loadSubmodules()
	rel = filepath.Clean(rel)
	submodule, ok := e.submoduleByPath[rel]
	return submodule, ok
}

func (e *Engine) initializedSubmoduleRoute(rel string) bool {
	e.loadSubmodules()
	return e.submoduleRoutes[filepath.Clean(rel)]
}

func (e *Engine) addIncludedSubmodule(rel string) {
	if e.includedSubmoduleSet == nil {
		e.includedSubmoduleSet = make(map[string]bool)
	}
	if e.includedSubmoduleSet[rel] {
		return
	}
	e.includedSubmoduleSet[rel] = true
	e.includedSubmodules = append(e.includedSubmodules, rel)
}

func (e *Engine) analysisRoots() []string {
	e.loadProjectFiles()
	roots := make([]string, 1, len(e.includedSubmodules)+1)
	copyRoots := append([]string(nil), e.includedSubmodules...)
	sort.Strings(copyRoots)
	return append(roots, copyRoots...)
}

func (e *Engine) analysisRootFor(rel string) string {
	for candidate := filepath.Clean(rel); candidate != "."; candidate = filepath.Dir(candidate) {
		if e.includedSubmoduleSet[candidate] {
			return candidate
		}
	}
	return ""
}

func (e *Engine) pathAtAnalysisRoot(rel string) string {
	root := e.analysisRootFor(rel)
	if root == "" {
		return filepath.Clean(rel)
	}
	local, err := filepath.Rel(root, rel)
	if err != nil {
		return filepath.Clean(rel)
	}
	return local
}

func (e *Engine) matchesProjectPattern(pattern, rel string) bool {
	slashRel := filepath.ToSlash(rel)
	if kb.MatchPathPattern(pattern, slashRel) {
		return true
	}
	local := e.pathAtAnalysisRoot(rel)
	return local != rel && kb.MatchPathPattern(pattern, filepath.ToSlash(local))
}

func (e *Engine) isAnalysisRootPath(root string) bool {
	if filepath.Clean(root) == filepath.Clean(e.Root) {
		return true
	}
	rel, err := filepath.Rel(e.Root, root)
	if err != nil {
		return false
	}
	submodule, ok := e.submoduleForPath(rel)
	return ok && submodule.Initialized
}

func (e *Engine) directSubmodulePaths(parent string) []string {
	e.loadSubmodules()
	var paths []string
	for _, submodule := range e.submodules {
		if submodule.Parent != parent {
			continue
		}
		rel, err := filepath.Rel(baseOrDot(parent), submodule.Path)
		if err == nil {
			paths = append(paths, filepath.ToSlash(rel))
		}
	}
	sort.Strings(paths)
	return paths
}

// ExpandSubmoduleChanges adds scanned files beneath changed submodule roots.
func (e *Engine) ExpandSubmoduleChanges(changedFiles []string) []string {
	e.loadProjectFiles()
	expanded := append([]string(nil), changedFiles...)
	seen := make(map[string]bool, len(changedFiles))
	for _, file := range changedFiles {
		seen[filepath.ToSlash(filepath.Clean(filepath.FromSlash(file)))] = true
	}
	for _, changed := range changedFiles {
		root := filepath.Clean(filepath.FromSlash(changed))
		if !e.includedSubmoduleSet[root] {
			continue
		}
		prefix := root + string(filepath.Separator)
		for _, file := range e.projectFiles {
			if !strings.HasPrefix(file, prefix) {
				continue
			}
			slashFile := filepath.ToSlash(file)
			if !seen[slashFile] {
				seen[slashFile] = true
				expanded = append(expanded, slashFile)
			}
		}
	}
	return expanded
}
