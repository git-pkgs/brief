package detect

import (
	"testing"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/kb"
)

func loadKB(t *testing.T) *kb.KnowledgeBase {
	t.Helper()
	knowledgeBase, err := kb.Load(brief.KnowledgeFS)
	if err != nil {
		t.Fatalf("loading knowledge base: %v", err)
	}
	return knowledgeBase
}

func rubyReport(t *testing.T) *brief.Report {
	t.Helper()
	engine := New(loadKB(t), "../testdata/ruby-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return r
}

func TestRubyLanguage(t *testing.T) {
	r := rubyReport(t)
	if len(r.Languages) == 0 {
		t.Fatal("expected at least one language")
	}
	if r.Languages[0].Name != "Ruby" {
		t.Errorf("expected Ruby language, got %s", r.Languages[0].Name)
	}
}

func TestRubyPackageManager(t *testing.T) {
	r := rubyReport(t)
	if len(r.PackageManagers) == 0 {
		t.Fatal("expected at least one package manager")
	}
	found := false
	for _, pm := range r.PackageManagers {
		if pm.Name == "Bundler" {
			found = true
			if pm.Command == nil || pm.Command.Run != "bundle install" {
				t.Errorf("expected 'bundle install' command for Bundler")
			}
			if pm.Lockfile != "Gemfile.lock" {
				t.Errorf("expected lockfile Gemfile.lock, got %q", pm.Lockfile)
			}
		}
	}
	if !found {
		t.Error("expected Bundler package manager")
	}
}

func TestRubyTools(t *testing.T) {
	r := rubyReport(t)
	assertToolDetected(t, r, "test", "RSpec")
	assertToolDetected(t, r, "lint", "RuboCop")
}

func TestRubyScripts(t *testing.T) {
	r := rubyReport(t)
	if len(r.Scripts) == 0 {
		t.Error("expected scripts from Makefile")
	}
	foundTest := false
	for _, s := range r.Scripts {
		if s.Name == "test" {
			foundTest = true
		}
	}
	if !foundTest {
		t.Error("expected 'test' script from Makefile")
	}
}

func TestRubyResources(t *testing.T) {
	r := rubyReport(t)
	if r.Resources == nil || r.Resources.Readme == "" {
		t.Error("expected README to be detected")
	}
	if r.Resources == nil || r.Resources.LicenseType == "" {
		t.Error("expected license type to be detected")
	}
	if r.Resources != nil && r.Resources.LicenseType != "MIT" {
		t.Errorf("expected MIT license, got %s", r.Resources.LicenseType)
	}
}

func TestRubyPlatforms(t *testing.T) {
	r := rubyReport(t)
	if r.Platforms == nil {
		t.Fatal("expected platform info")
	}
	if v, ok := r.Platforms.RuntimeVersionFiles[".ruby-version"]; !ok || v != "3.4.2" {
		t.Errorf("expected .ruby-version 3.4.2, got %v", r.Platforms.RuntimeVersionFiles)
	}
	if versions, ok := r.Platforms.CIMatrixVersions["ruby"]; !ok || len(versions) == 0 {
		t.Error("expected ruby versions from CI matrix")
	} else if len(versions) != 3 {
		t.Errorf("expected 3 ruby versions from CI matrix, got %d", len(versions))
	}
	if len(r.Platforms.CIMatrixOS) == 0 {
		t.Error("expected OS targets from CI matrix")
	}
}

func TestRubyLayout(t *testing.T) {
	r := rubyReport(t)
	if r.Layout == nil {
		t.Fatal("expected layout info")
	}
}

func TestGoProject(t *testing.T) {
	engine := New(loadKB(t), "../testdata/go-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(r.Languages) == 0 || r.Languages[0].Name != "Go" {
		t.Error("expected Go language")
	}

	found := false
	for _, pm := range r.PackageManagers {
		if pm.Name == "Go Modules" {
			found = true
		}
	}
	if !found {
		t.Error("expected Go Modules package manager")
	}

	assertToolDetected(t, r, "test", "go test")
	assertToolDetected(t, r, "lint", "golangci-lint")
}

func TestSQLiteDetection(t *testing.T) {
	engine := New(loadKB(t), "../testdata/sqlite-go-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertToolDetected(t, r, "database", "SQLite")
}

func TestNodeProject(t *testing.T) {
	engine := New(loadKB(t), "../testdata/node-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(r.Languages) == 0 {
		t.Fatal("expected at least one language")
	}
	foundJS := false
	for _, l := range r.Languages {
		if l.Name == "JavaScript" {
			foundJS = true
		}
	}
	if !foundJS {
		t.Error("expected JavaScript language")
	}

	// npm detection via lockfile
	foundNPM := false
	for _, pm := range r.PackageManagers {
		if pm.Name == "npm" {
			foundNPM = true
		}
	}
	if !foundNPM {
		t.Error("expected npm package manager")
	}

	// Jest and ESLint from dependencies
	assertToolDetected(t, r, "test", "Jest")
	assertToolDetected(t, r, "lint", "ESLint")

	// Scripts from package.json
	if len(r.Scripts) == 0 {
		t.Error("expected scripts from package.json")
	}
}

func TestPythonProject(t *testing.T) {
	engine := New(loadKB(t), "../testdata/python-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(r.Languages) == 0 || r.Languages[0].Name != "Python" {
		t.Error("expected Python language")
	}

	// uv detection via file_contains and lockfile
	foundUV := false
	for _, pm := range r.PackageManagers {
		if pm.Name == "uv" {
			foundUV = true
		}
	}
	if !foundUV {
		t.Error("expected uv package manager")
	}

	// pytest, ruff, mypy from file_contains
	assertToolDetected(t, r, "test", "pytest")
	assertToolDetected(t, r, "lint", "Ruff")
	assertToolDetected(t, r, "typecheck", "mypy")

	// Layout
	if r.Layout == nil {
		t.Fatal("expected layout info")
	}
	foundTests := false
	for _, d := range r.Layout.TestDirs {
		if d == "tests" {
			foundTests = true
		}
	}
	if !foundTests {
		t.Error("expected tests/ in layout")
	}
}

func TestEmptyProject(t *testing.T) {
	engine := New(loadKB(t), "../testdata/empty-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(r.Languages) != 0 {
		t.Errorf("expected no languages, got %v", r.Languages)
	}
	if len(r.PackageManagers) != 0 {
		t.Errorf("expected no package managers, got %v", r.PackageManagers)
	}
}

func TestKnowledgeBaseLoads(t *testing.T) {
	knowledgeBase := loadKB(t)

	if len(knowledgeBase.Tools) == 0 {
		t.Fatal("expected tools to be loaded")
	}

	ecosystems := knowledgeBase.AllEcosystems()
	if len(ecosystems) == 0 {
		t.Fatal("expected ecosystems to be loaded")
	}

	categories := knowledgeBase.Categories()
	if len(categories) == 0 {
		t.Fatal("expected categories to be loaded")
	}

	// Check that manifest files are loaded from config
	if len(knowledgeBase.ManifestFiles) == 0 {
		t.Fatal("expected manifest files to be loaded from _manifests.toml")
	}

	// Check that script sources are loaded
	if len(knowledgeBase.ScriptSources) == 0 {
		t.Fatal("expected script sources to be loaded")
	}

	// Check that resources are loaded
	if len(knowledgeBase.Resources) == 0 {
		t.Fatal("expected resources to be loaded")
	}
}

func TestNoEmptyToolNames(t *testing.T) {
	knowledgeBase := loadKB(t)

	for _, tool := range knowledgeBase.Tools {
		if tool.Tool.Name == "" {
			t.Errorf("tool with empty name loaded into knowledge base (category=%q)", tool.Tool.Category)
		}
	}
}

func TestNoDuplicateToolNames(t *testing.T) {
	knowledgeBase := loadKB(t)

	seen := make(map[string]string) // name -> first source path
	for _, tool := range knowledgeBase.Tools {
		if tool.Tool.Name == "" {
			continue
		}
		if prev, ok := seen[tool.Tool.Name]; ok {
			t.Errorf("duplicate tool name %q: first in %s, also in %s", tool.Tool.Name, prev, tool.Source)
		} else {
			seen[tool.Tool.Name] = tool.Source
		}
	}
}

func TestScriptPriority(t *testing.T) {
	engine := New(loadKB(t), "../testdata/ruby-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Ruby project has Makefile with test: target and RSpec detected
	// The test tool's command should come from the project script
	tests, ok := r.Tools["test"]
	if !ok || len(tests) == 0 {
		t.Fatal("expected test tools")
	}

	// First test tool should have script-sourced command
	first := tests[0]
	if first.Command == nil {
		t.Fatal("expected command on first test tool")
	}
	if first.Command.Source != brief.SourceProjectScript {
		t.Errorf("expected source project_script, got %s", first.Command.Source)
	}
	if first.Command.Run != "make test" {
		t.Errorf("expected 'make test', got %s", first.Command.Run)
	}
	if first.Command.InferredTool == "" {
		t.Error("expected inferred_tool to be set")
	}
}

func TestKeyExists(t *testing.T) {
	knowledgeBase := loadKB(t)
	engine := New(knowledgeBase, "../testdata/node-project")

	// package.json has a top-level "jest" key check via key_exists
	// and also has devDependencies with jest, so it should be detected
	matched := engine.hasKey("package.json", []string{"scripts.test"})
	if !matched {
		t.Error("expected key_exists to match scripts.test in package.json")
	}

	matched = engine.hasKey("package.json", []string{"scripts.nonexistent"})
	if matched {
		t.Error("expected key_exists to not match scripts.nonexistent")
	}

	matched = engine.hasKey("nonexistent.json", []string{"anything"})
	if matched {
		t.Error("expected key_exists to not match nonexistent file")
	}
}

func TestShouldSkipDir(t *testing.T) {
	engine := New(loadKB(t), "../testdata/empty-project")

	tests := []struct {
		name string
		skip bool
	}{
		{"node_modules", true},
		{"vendor", true},
		{".git", true},
		{"coverage", true},
		{"src", false},
		{"lib", false},
		{"app", false},
	}

	for _, tt := range tests {
		if got := engine.shouldSkipDir(tt.name); got != tt.skip {
			t.Errorf("shouldSkipDir(%q) = %v, want %v", tt.name, got, tt.skip)
		}
	}
}

func assertToolDetected(t *testing.T, r *brief.Report, category, name string) {
	t.Helper()
	tools, ok := r.Tools[category]
	if !ok {
		t.Errorf("expected %s category in tools", category)
		return
	}
	for _, tool := range tools {
		if tool.Name == name {
			return
		}
	}
	t.Errorf("expected %s in %s category", name, category)
}
