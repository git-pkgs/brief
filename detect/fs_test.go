package detect_test

import (
	"cmp"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/detect"
	"github.com/git-pkgs/brief/kb"
)

func snapshotKB(t *testing.T) *kb.KnowledgeBase {
	t.Helper()
	knowledge, err := kb.Load(brief.KnowledgeFS)
	if err != nil {
		t.Fatal(err)
	}
	return knowledge
}

func snapshotReport(t *testing.T, fsys fs.FS) *brief.Report {
	t.Helper()
	r, err := detect.NewFS(snapshotKB(t), fsys).Run()
	if err != nil {
		t.Fatal(err)
	}
	if r.Path != "." || r.Git != nil || r.Lines != nil {
		t.Fatalf("snapshot report has worktree fields: path=%q, git=%+v, lines=%+v", r.Path, r.Git, r.Lines)
	}
	return r
}

func snapshotTool(t *testing.T, r *brief.Report, category, name string) brief.Detection {
	t.Helper()
	for _, tool := range r.Tools[category] {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("missing %s tool %q: %+v", category, name, r.Tools[category])
	return brief.Detection{}
}

func TestNewFSPathAndContentEvidence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		content    string
		confidence brief.Confidence
	}{
		{"path and content", "[tool.ruff]\nline-length = 88\n", brief.ConfidenceHigh},
		{"path only", "[project]\nname = 'sample'\n", brief.ConfidenceMedium},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := snapshotReport(t, fstest.MapFS{
				".ruff.toml":     {Data: []byte("line-length = 88\n")},
				"pyproject.toml": {Data: []byte(tc.content)},
				"src/main.py":    {Data: []byte("print('hello')\n")},
			})
			if len(r.Languages) != 1 || r.Languages[0].Name != "Python" {
				t.Fatalf("languages = %+v, want Python", r.Languages)
			}
			ruff := snapshotTool(t, r, "lint", "Ruff")
			if ruff.Confidence != tc.confidence {
				t.Errorf("Ruff confidence = %s, want %s", ruff.Confidence, tc.confidence)
			}
			if !slices.Equal(ruff.ConfigFiles, []string{".ruff.toml", "pyproject.toml"}) {
				t.Errorf("Ruff config files = %v", ruff.ConfigFiles)
			}
		})
	}
}

func TestNewFSDetectionPrimitives(t *testing.T) {
	for _, tc := range []struct {
		name       string
		files      fstest.MapFS
		category   string
		tool       string
		confidence brief.Confidence
	}{
		{"directory", fstest.MapFS{"locale": {Mode: fs.ModeDir}}, "i18n", "gettext", brief.ConfidenceLow},
		{"glob", fstest.MapFS{"nested/main.tf": {Data: []byte("terraform {}\n")}}, "infrastructure", "Terraform", brief.ConfidenceMedium},
		{"JSON key", fstest.MapFS{"package.json": {Data: []byte(`{"name":"sample","jest":{}}`)}}, "test", "Jest", brief.ConfidenceMedium},
		{"TOML key", fstest.MapFS{"pyproject.toml": {Data: []byte("[tool.tox]\n")}}, "test", "tox", brief.ConfidenceMedium},
		{"YAML resource", fstest.MapFS{"deploy/app.yaml": {Data: []byte("apiVersion: argoproj.io/v1alpha1\nkind: Application\nmetadata:\n  name: sample\n")}}, "infrastructure", "Argo CD", brief.ConfidenceHigh},
		{"dependency", fstest.MapFS{"package.json": {Data: []byte(`{"name":"sample","devDependencies":{"eslint":"^9.0.0"}}`)}}, "lint", "ESLint", brief.ConfidenceHigh},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool := snapshotTool(t, snapshotReport(t, tc.files), tc.category, tc.tool)
			if tool.Confidence != tc.confidence {
				t.Errorf("confidence = %s, want %s", tool.Confidence, tc.confidence)
			}
		})
	}
}

func TestNewFSMatchesDirectoryFixtures(t *testing.T) {
	knowledge := snapshotKB(t)
	for _, fixture := range []string{"ruby-project", "node-project", "python-project", "go-project", "argocd-project", "citation-project", "bibtex-project", "codemeta-project", "rustler-project"} {
		t.Run(fixture, func(t *testing.T) {
			root := filepath.Join("..", "testdata", fixture)
			files := make(fstest.MapFS)
			err := fs.WalkDir(os.DirFS(root), ".", func(name string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				data, err := fs.ReadFile(os.DirFS(root), name)
				if err == nil {
					files[name] = &fstest.MapFile{Data: data}
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			want, err := detect.New(knowledge, root).Run()
			if err != nil {
				t.Fatal(err)
			}
			got := snapshotReport(t, files)
			want.Path = got.Path
			want.Git, want.Lines = nil, nil
			want.Stats, got.Stats = brief.Stats{}, brief.Stats{}
			for _, r := range []*brief.Report{got, want} {
				slices.SortFunc(r.Scripts, func(a, b brief.Script) int { return cmp.Compare(a.Name, b.Name) })
				slices.SortFunc(r.Dependencies, func(a, b brief.DepInfo) int {
					return cmp.Or(cmp.Compare(a.Manifest, b.Manifest), cmp.Compare(a.Name, b.Name))
				})
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("snapshot differs from directory:\ngot  %+v\nwant %+v", got, want)
			}
		})
	}
}

type readDirOnlyFS struct{ fs.FS }

func (r readDirOnlyFS) Open(name string) (fs.File, error) {
	f, err := r.FS.Open(name)
	if err != nil {
		return nil, err
	}
	return struct{ fs.File }{f}, nil
}

func (r readDirOnlyFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(r.FS, name)
}

func TestNewFSReadDirFS(t *testing.T) {
	r := snapshotReport(t, readDirOnlyFS{fstest.MapFS{
		"nested/main.py": {Data: []byte("print('hello')\n")},
	}})
	if len(r.Languages) != 1 || r.Languages[0].Name != "Python" || r.Stats.ScanTruncated {
		t.Fatalf("ReadDirFS scan: languages=%+v, stats=%+v", r.Languages, r.Stats)
	}
}

func TestNewFSManifestsAndWorkspaces(t *testing.T) {
	r := snapshotReport(t, fstest.MapFS{
		"package.json":              {Data: []byte(`{"name":"sample","workspaces":["packages/*"]}`)},
		"packages/app/package.json": {Data: []byte(`{"name":"app","devDependencies":{"eslint":"^9.0.0"}}`)},
	})
	snapshotTool(t, r, "lint", "ESLint")
	if !slices.ContainsFunc(r.Manifests, func(m brief.ManifestInfo) bool {
		return m.Path == "packages/app/package.json" && m.Ecosystem == "npm"
	}) {
		t.Errorf("missing workspace manifest: %+v", r.Manifests)
	}
	if !slices.ContainsFunc(r.Dependencies, func(d brief.DepInfo) bool {
		return d.Name == "eslint" && d.Manifest == "packages/app/package.json" && d.Scope == brief.ScopeDevelopment
	}) {
		t.Errorf("missing workspace dependency: %+v", r.Dependencies)
	}
}

func TestNewFSResourcesAndPlatforms(t *testing.T) {
	r := snapshotReport(t, fstest.MapFS{
		"README.md":                {Data: []byte("Sample project\n")},
		".python-version":          {Data: []byte("3.13\n")},
		"src/main.py":              {Data: []byte("print('hello')\n")},
		".github/workflows/ci.yml": {Data: []byte("jobs:\n  test:\n    strategy:\n      matrix:\n        os: [ubuntu-latest]\n        python-version: ['3.13']\n")},
	})
	snapshotTool(t, r, "ci", "GitHub Actions")
	if r.Resources == nil || r.Resources.Readme != "README.md" {
		t.Fatalf("resources = %+v", r.Resources)
	}
	if r.Platforms == nil || r.Platforms.RuntimeVersionFiles[".python-version"] != "3.13" ||
		!slices.Equal(r.Platforms.CIMatrixOS, []string{"ubuntu-latest"}) ||
		!slices.Equal(r.Platforms.CIMatrixVersions["python"], []string{"3.13"}) {
		t.Errorf("platforms = %+v", r.Platforms)
	}
}

func TestNewFSSubdirectoryAndSkipDirs(t *testing.T) {
	files := fstest.MapFS{
		"repo/src/main.go":       {Data: []byte("package main\n")},
		"repo/ignored/main.py":   {Data: []byte("print('hello')\n")},
		"repo/vendor/native.c":   {Data: []byte("int main(void) {}\n")},
		"outside/pyproject.toml": {Data: []byte("[tool.ruff]\n")},
	}
	fsys, err := fs.Sub(files, "repo")
	if err != nil {
		t.Fatal(err)
	}
	e := detect.NewFS(snapshotKB(t), fsys)
	e.SkipDirs = []string{"ignored"}
	e.IncludeSubmodules = true
	r, err := e.Run()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Languages) != 1 || r.Languages[0].Name != "Go" {
		t.Errorf("languages = %+v, want Go", r.Languages)
	}
}

func TestNewFSScanBounds(t *testing.T) {
	files := fstest.MapFS{
		"nested/deep/main.py": {Data: []byte("print('hello')\n")},
	}
	for _, tc := range []struct {
		name  string
		setup func(*detect.Engine)
	}{
		{"depth", func(e *detect.Engine) { e.ScanDepth = 1 }},
		{"entries", func(e *detect.Engine) { e.ScanLimit = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := detect.NewFS(snapshotKB(t), files)
			tc.setup(e)
			r, err := e.Run()
			if err != nil {
				t.Fatal(err)
			}
			if !r.Stats.ScanTruncated || len(r.Languages) != 0 {
				t.Fatalf("bounded scan: stats=%+v, languages=%+v", r.Stats, r.Languages)
			}
		})
	}
}

func TestNewFSRejectsUnavailableOptions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fsys  fs.FS
		setup func(*detect.Engine)
		want  string
	}{
		{"nil filesystem", nil, func(*detect.Engine) {}, "filesystem must not be nil"},
		{"tracked only", fstest.MapFS{}, func(e *detect.Engine) { e.TrackedOnly = true }, "TrackedOnly requires a project directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := detect.NewFS(snapshotKB(t), tc.fsys)
			tc.setup(e)
			if _, err := e.Run(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Run error = %v, want %q", err, tc.want)
			}
		})
	}
}
