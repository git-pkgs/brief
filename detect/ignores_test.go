package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/git-pkgs/brief"
)

func TestGitignoreKeepsTrackedAndNewProjectFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	files := map[string]string{
		".gitignore":         "/scratch/\n/package.json\n/Makefile\n",
		"scratch/pinned.go":  "package project\n",
		"scratch/foreign.rb": "puts 'scratch'\n",
		"new.py":             "print('new source')\n",
		"package.json":       `{"scripts":{"test":"echo scratch"},"devDependencies":{"eslint":"9.0.0"}}`,
		"Makefile":           "all:\n\techo scratch\n",
	}
	for name, data := range files {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	gitFixtureCommand(t, root, "init", "-q")
	gitFixtureCommand(t, root, "add", "-f", "scratch/pinned.go")

	for _, trackedOnly := range []bool{false, true} {
		engine := New(loadKB(t), root)
		engine.TrackedOnly = trackedOnly
		report, err := engine.Run()
		if err != nil {
			t.Fatal(err)
		}
		for _, lang := range []struct {
			name string
			want bool
		}{
			{"Go", true},
			{"Ruby", false},
			{"JavaScript", false},
			{"Python", !trackedOnly},
		} {
			found := slices.ContainsFunc(report.Languages, func(item brief.Detection) bool { return item.Name == lang.name })
			if found != lang.want {
				t.Errorf("tracked=%t: %s present = %t, want %t", trackedOnly, lang.name, found, lang.want)
			}
		}
		if !trackedOnly && (len(report.Scripts) > 0 || len(report.Dependencies) > 0 || len(report.Manifests) > 0) {
			t.Errorf("ignored exact paths leaked into the report: %v %v %v", report.Scripts, report.Dependencies, report.Manifests)
		}
		if !trackedOnly && (engine.contains("Makefile", []string{"echo scratch"}) || engine.hasKey("package.json", []string{"scripts.test"})) {
			t.Error("ignored exact paths provided configuration evidence")
		}
	}
}

func TestGitignoreLoadsAncestorRulesAndLocalExcludes(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := initGitProject(t, map[string]string{
		".gitignore":              "/section/tree/discarded/\n/section/tree/restored.py\n",
		"section/.gitignore":      "!tree/restored.py\n",
		"section/tree/.gitignore": "!unhidden.jsx\n",
		"section/tree/main.go":    "package project\n",
	})
	writeProjectFile(t, root, ".git/info/exclude", "/section/tree/excluded.jsx\n/section/tree/unhidden.jsx\n")
	writeProjectFile(t, root, "section/tree/discarded/sample.rb", "puts 'ignored'\n")
	writeProjectFile(t, root, "section/tree/restored.py", "print('included')\n")
	writeProjectFile(t, root, "section/tree/excluded.jsx", "export const tool = 'ignored';\n")
	writeProjectFile(t, root, "section/tree/unhidden.jsx", "export const tool = 'included';\n")
	globalExclude := filepath.Join(t.TempDir(), "ignores")
	if err := os.WriteFile(globalExclude, []byte("global-only.py\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitFixtureCommand(t, root, "config", "core.excludesFile", globalExclude)
	writeProjectFile(t, root, "section/tree/global-only.py", "print('included')\n")

	engine := New(loadKB(t), filepath.Join(root, "section", "tree"))
	engine.loadProjectFiles()
	want := []string{".gitignore", "global-only.py", "main.go", "restored.py", "unhidden.jsx"}
	if !slices.Equal(engine.projectFiles, want) {
		t.Errorf("subdirectory project files = %v, want %v", engine.projectFiles, want)
	}
}

func TestGitignoreRespectsEachIncludedSubmodule(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	parent := initGitProject(t, map[string]string{".gitignore": "*.rb\n"})
	child := initGitProject(t, map[string]string{".gitignore": "/discarded/\n", "main.go": "package child\n"})
	addGitSubmodule(t, parent, child, "modules/child")
	writeProjectFile(t, parent, "excluded.rb", "puts 'ignored'\n")
	writeProjectFile(t, parent, "modules/child/kept.rb", "puts 'included'\n")
	writeProjectFile(t, parent, "modules/child/discarded/sample.py", "print('ignored')\n")
	writeProjectFile(t, parent, "modules/child/excluded.py", "print('ignored locally')\n")
	writeProjectFile(t, parent, ".git/modules/modules/child/info/exclude", "/excluded.py\n")

	engine := New(loadKB(t), parent)
	engine.IncludeSubmodules = true
	engine.loadProjectFiles()
	want := []string{
		".gitignore", ".gitmodules",
		filepath.Join("modules", "child", ".git"),
		filepath.Join("modules", "child", ".gitignore"),
		filepath.Join("modules", "child", "kept.rb"),
		filepath.Join("modules", "child", "main.go"),
	}
	if !slices.Equal(engine.projectFiles, want) {
		t.Errorf("submodule project files = %v, want %v", engine.projectFiles, want)
	}
}

func TestGitignoreWorksOutsideGitOnDisk(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, ".gitignore", "/discarded/\n")
	writeProjectFile(t, root, "discarded/sample.rb", "puts 'ignored'\n")
	writeProjectFile(t, root, "main.go", "package project\n")

	engine := New(loadKB(t), root)
	engine.loadProjectFiles()
	want := []string{".gitignore", "main.go"}
	if !slices.Equal(engine.projectFiles, want) {
		t.Errorf("non-Git project files = %v, want %v", engine.projectFiles, want)
	}
}

func TestGitignoreNestedRulesAndDirectoryPruning(t *testing.T) {
	tree := fstest.MapFS{
		".gitignore":         {Data: []byte("/scratch/\n*.rb\n")},
		"main.go":            {Data: []byte("package project\n")},
		"scratch/one.java":   {},
		"scratch/two.java":   {},
		"scratch/three.java": {},
		"src/.gitignore":     {Data: []byte("!allow.rb\n")},
		"src/allow.rb":       {},
		"src/other.rb":       {},
		"tools/allow.rb":     {},
	}
	engine := NewFS(loadKB(t), tree)
	engine.ScanLimit = 10
	engine.loadProjectFiles()
	if !slices.Equal(engine.projectFiles, []string{".gitignore", "main.go", filepath.Join("src", ".gitignore"), filepath.Join("src", "allow.rb")}) {
		t.Errorf("unexpected project files: %v", engine.projectFiles)
	}
	if engine.scanTruncated || engine.scanEntries != 9 {
		t.Errorf("ignored directory consumed its children's scan budget: entries=%d truncated=%t", engine.scanEntries, engine.scanTruncated)
	}
}
