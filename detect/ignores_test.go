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
		hasLanguage := func(name string) bool {
			return slices.ContainsFunc(report.Languages, func(item brief.Detection) bool { return item.Name == name })
		}
		if !hasLanguage("Go") || hasLanguage("Ruby") || hasLanguage("JavaScript") || hasLanguage("Python") == trackedOnly {
			t.Errorf("tracked=%t: incorrect language evidence: %v", trackedOnly, report.Languages)
		}
		if !trackedOnly && (len(report.Scripts) > 0 || len(report.Dependencies) > 0 || len(report.Manifests) > 0) {
			t.Errorf("ignored exact paths leaked into the report: %v %v %v", report.Scripts, report.Dependencies, report.Manifests)
		}
		if !trackedOnly && (engine.contains("Makefile", []string{"echo scratch"}) || engine.hasKey("package.json", []string{"scripts.test"})) {
			t.Error("ignored exact paths provided configuration evidence")
		}
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
