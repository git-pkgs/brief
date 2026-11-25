package detect

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/git-pkgs/gitignore"
)

const ignoreFileSizeLimit = 1 << 20

type projectIgnoreRules struct {
	matcher *gitignore.Matcher
	prefix  string
}

func (r *projectIgnoreRules) scopedPath(rel string) string {
	if scoped := path.Join(r.prefix, filepath.ToSlash(rel)); scoped != "." {
		return scoped
	}
	return ""
}

func (r *projectIgnoreRules) addRuleFile(file, scope string) {
	info, err := os.Lstat(file)
	if err == nil && info.Mode().IsRegular() {
		r.matcher.AddFromFile(file, scope)
	}
}

func (e *Engine) loadRepositoryIgnoreRules(rules *projectIgnoreRules, scanRoot string) {
	out, err := e.git(scanRoot, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-path", "info/exclude")
	if err != nil {
		return
	}
	top, exclude, ok := strings.Cut(strings.TrimRight(string(out), "\r\n"), "\n")
	if !ok {
		return
	}
	top = filepath.FromSlash(strings.TrimSuffix(top, "\r"))
	exclude = filepath.FromSlash(exclude)
	if !filepath.IsAbs(top) || !filepath.IsAbs(exclude) {
		return
	}
	absRoot, err := filepath.Abs(scanRoot)
	if err != nil {
		return
	}
	absRoot, err = filepath.EvalSymlinks(absRoot)
	if err != nil {
		return
	}
	prefix, err := filepath.Rel(top, absRoot)
	if err != nil || prefix == ".." || strings.HasPrefix(prefix, ".."+string(filepath.Separator)) {
		return
	}

	// Repository-local excludes have lower priority than working-tree rules.
	rules.addRuleFile(exclude, "")
	if prefix == "." {
		return
	}
	rules.prefix = filepath.ToSlash(prefix)
	scope := ""
	for segment := range strings.SplitSeq(rules.prefix, "/") {
		rules.addRuleFile(filepath.Join(top, filepath.FromSlash(scope), ".gitignore"), scope)
		scope = path.Join(scope, segment)
	}
}

// loadIgnoreRules reads rules when the bounded scan reaches their directory.
// Each included submodule uses its own matcher, like a separate checkout.
func (e *Engine) loadIgnoreRules(dir string) {
	if e.TrackedOnly {
		return
	}
	root := e.analysisRootFor(dir)
	if e.ignoreMatchers == nil {
		e.ignoreMatchers = make(map[string]*projectIgnoreRules)
	}
	rules := e.ignoreMatchers[root]
	if rules == nil {
		rules = &projectIgnoreRules{matcher: gitignore.New("", gitignore.MaxIgnoreFileSize(ignoreFileSizeLimit))}
		if !e.fsOnly {
			e.loadRepositoryIgnoreRules(rules, filepath.Join(e.Root, root))
		}
		e.ignoreMatchers[root] = rules
	}

	file := filepath.Join(dir, ".gitignore")
	var info fs.FileInfo
	var err error
	if e.fsOnly {
		info, err = fs.Lstat(e.fsys, filepath.ToSlash(file))
	} else {
		info, err = os.Lstat(filepath.Join(e.Root, file))
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > ignoreFileSizeLimit {
		return
	}
	data, err := e.safeReadFileLimit(file, ignoreFileSizeLimit+1)
	if err != nil || len(data) == 0 || len(data) > ignoreFileSizeLimit {
		return
	}
	rules.matcher.AddPatterns(data, rules.scopedPath(e.pathAtAnalysisRoot(dir)))
}

func (e *Engine) ignoredProjectPath(rel string, isDir bool) bool {
	if e.TrackedOnly {
		return false
	}
	e.loadProjectFiles()
	rel = filepath.Clean(rel)
	rules := e.ignoreMatchers[e.analysisRootFor(rel)]
	if rules == nil || !rules.matcher.MatchPath(rules.scopedPath(e.pathAtAnalysisRoot(rel)), isDir) {
		return false
	}
	if e.fsOnly {
		return true
	}

	// A tracked file remains visible even if its name matches an ignore rule.
	// Only consult the index once a matching rule makes it necessary.
	if !e.ignoreTrackedLoaded {
		e.ignoreTrackedLoaded = true
		_ = e.loadTracked(e.Root)
	}
	if isDir {
		return !e.trackedDirs[rel] && !e.tracked[rel]
	}
	return !e.tracked[rel]
}

func (e *Engine) readEvidenceFile(file string) ([]byte, error) {
	if e.ignoredProjectPath(file, false) {
		return nil, fs.ErrNotExist
	}
	return e.safeReadFile(file)
}
