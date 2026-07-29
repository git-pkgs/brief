package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

func TestRubyTaxonomyPassesThrough(t *testing.T) {
	r := rubyReport(t)

	// Rails has [taxonomy] populated, should appear on the Detection.
	var rails *brief.Detection
	for i, d := range r.Tools["build"] {
		if d.Name == "Rails" {
			rails = &r.Tools["build"][i]
		}
	}
	if rails == nil {
		t.Fatal("expected Rails in build tools")
	}
	if rails.Taxonomy == nil {
		t.Fatal("expected Rails detection to carry taxonomy")
	}
	if len(rails.Taxonomy.Role) == 0 || rails.Taxonomy.Role[0] != "framework" {
		t.Errorf("Rails taxonomy.role = %v, want [framework]", rails.Taxonomy.Role)
	}
	if len(rails.Taxonomy.Layer) == 0 {
		t.Error("Rails taxonomy.layer should be populated")
	}
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

func TestResourceGroups(t *testing.T) {
	dir := t.TempDir()
	touch := func(p string) {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	touch("README.md")
	touch("CHANGELOG.md")
	touch("AGENTS.md")
	touch("NOTICE")
	touch("CONTRIBUTING.md")
	touch(".github/CODE_OF_CONDUCT.md")
	touch(".github/CODEOWNERS")
	touch(".github/FUNDING.yml")
	touch("docs/SECURITY.md")
	touch("DISCLOSURE")
	touch("CITATION.cff")

	engine := New(loadKB(t), dir)
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	res := r.Resources
	if res == nil {
		t.Fatal("expected resources")
	}

	if res.Readme != "README.md" {
		t.Errorf("readme = %q", res.Readme)
	}
	if res.Agents["agents"] != "AGENTS.md" {
		t.Errorf("agents.agents = %q", res.Agents["agents"])
	}
	if res.Legal["notice"] != "NOTICE" {
		t.Errorf("legal.notice = %q", res.Legal["notice"])
	}
	if res.Community["contributing"] != "CONTRIBUTING.md" {
		t.Errorf("community.contributing = %q", res.Community["contributing"])
	}
	if res.Community["code_of_conduct"] != ".github/CODE_OF_CONDUCT.md" {
		t.Errorf("community.code_of_conduct = %q", res.Community["code_of_conduct"])
	}
	if res.Community["codeowners"] != ".github/CODEOWNERS" {
		t.Errorf("community.codeowners = %q", res.Community["codeowners"])
	}
	if res.Security["policy"] != "docs/SECURITY.md" {
		t.Errorf("security.policy = %q", res.Security["policy"])
	}
	if res.Security["disclosure"] != "DISCLOSURE" {
		t.Errorf("security.disclosure = %q", res.Security["disclosure"])
	}
	if res.Metadata["funding"] != ".github/FUNDING.yml" {
		t.Errorf("metadata.funding = %q", res.Metadata["funding"])
	}
	if res.Metadata["citation"] != "CITATION.cff" {
		t.Errorf("metadata.citation = %q", res.Metadata["citation"])
	}
}

func TestResourceCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"ReadMe.rst", "Security.MD", ".github/Code_Of_Conduct.md"} {
		full := filepath.Join(dir, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	engine := New(loadKB(t), dir)
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Resources == nil {
		t.Fatal("expected resources")
	}
	if r.Resources.Readme != "ReadMe.rst" {
		t.Errorf("readme = %q", r.Resources.Readme)
	}
	if r.Resources.Security["policy"] != "Security.MD" {
		t.Errorf("security.policy = %q", r.Resources.Security["policy"])
	}
	if r.Resources.Community["code_of_conduct"] != ".github/Code_Of_Conduct.md" {
		t.Errorf("code_of_conduct = %q", r.Resources.Community["code_of_conduct"])
	}
}

func TestDetectTemplates(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{
		".github/ISSUE_TEMPLATE/bug_report.md",
		".github/ISSUE_TEMPLATE/feature.yml",
		".github/ISSUE_TEMPLATE/config.yml",
		".github/ISSUE_TEMPLATE/ignore.png",
		".github/PULL_REQUEST_TEMPLATE.md",
		".gitlab/merge_request_templates/Default.md",
		".gitlab/issue_templates/Bug.md",
		".forgejo/pull_request_template.yaml",
		"docs/issue_template.md",
	} {
		writeFile(t, dir, p, "x")
	}

	engine := New(loadKB(t), dir)
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	tpl := r.Resources.Templates
	if tpl == nil {
		t.Fatal("expected templates")
	}

	wantIssue := []string{
		".github/ISSUE_TEMPLATE/bug_report.md",
		".github/ISSUE_TEMPLATE/feature.yml",
		".gitlab/issue_templates/Bug.md",
		"docs/issue_template.md",
	}
	if !slices.Equal(tpl.Issue, wantIssue) {
		t.Errorf("issue = %v, want %v", tpl.Issue, wantIssue)
	}

	wantPR := []string{
		".forgejo/pull_request_template.yaml",
		".github/PULL_REQUEST_TEMPLATE.md",
		".gitlab/merge_request_templates/Default.md",
	}
	if !slices.Equal(tpl.PullRequest, wantPR) {
		t.Errorf("pull_request = %v, want %v", tpl.PullRequest, wantPR)
	}

	if tpl.Config != ".github/ISSUE_TEMPLATE/config.yml" {
		t.Errorf("config = %q", tpl.Config)
	}
}

func TestDetectTemplatesNone(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "x")
	engine := New(loadKB(t), dir)
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Resources.Templates != nil {
		t.Errorf("expected nil templates, got %+v", r.Resources.Templates)
	}
}

func TestResourceRootBeatsSubdir(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"CONTRIBUTING.md", ".github/CONTRIBUTING.md"} {
		full := filepath.Join(dir, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	engine := New(loadKB(t), dir)
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := r.Resources.Community["contributing"]; got != "CONTRIBUTING.md" {
		t.Errorf("expected root CONTRIBUTING.md to win, got %q", got)
	}
}

func writeFile(t *testing.T, dir, p, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResourceAgents(t *testing.T) {
	dir := t.TempDir()
	touch := func(p string) {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	touch("AGENTS.md")
	touch("CLAUDE.md")
	touch("GEMINI.md")
	touch(".cursorrules")
	touch(".windsurfrules")
	touch(".github/copilot-instructions.md")
	touch(".aider.conf.yml")
	touch(".clinerules")
	touch(".rules")
	touch(".junie/guidelines.md")
	touch("replit.md")
	touch(".continuerules")
	touch(".augment-guidelines")
	touch(".roorules")

	engine := New(loadKB(t), dir)
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Resources == nil {
		t.Fatal("expected resources")
	}
	want := map[string]string{
		"agents":   "AGENTS.md",
		"claude":   "CLAUDE.md",
		"gemini":   "GEMINI.md",
		"cursor":   ".cursorrules",
		"windsurf": ".windsurfrules",
		"copilot":  ".github/copilot-instructions.md",
		"aider":    ".aider.conf.yml",
		"cline":    ".clinerules",
		"zed":      ".rules",
		"junie":    ".junie/guidelines.md",
		"replit":   "replit.md",
		"continue": ".continuerules",
		"augment":  ".augment-guidelines",
		"roo":      ".roorules",
	}
	for field, path := range want {
		if got := r.Resources.Agents[field]; got != path {
			t.Errorf("agents.%s = %q, want %q", field, got, path)
		}
	}
	if len(r.Resources.Agents) != len(want) {
		t.Errorf("agents has %d entries, want %d: %v", len(r.Resources.Agents), len(want), r.Resources.Agents)
	}
}

func TestResourceAgentsCursorRulesDir(t *testing.T) {
	dir := t.TempDir()
	full := filepath.Join(dir, ".cursor", "rules", "general.mdc")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := New(loadKB(t), dir)
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Resources == nil {
		t.Fatal("expected resources")
	}
	if got := r.Resources.Agents["cursor"]; got != ".cursor/rules/general.mdc" {
		t.Errorf("agents.cursor = %q", got)
	}
}

func TestDetectSkills(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "skills/pdf/SKILL.md", `---
name: pdf
description: Read and fill PDF forms
---

Body here.
`)
	writeFile(t, dir, ".claude/skills/excel/SKILL.md", `---
name: excel-tools
description: Generate spreadsheets
---
`)
	writeFile(t, dir, "skills/empty/SKILL.md", "no frontmatter\n")

	engine := New(loadKB(t), dir)
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(r.Skills) != 3 {
		t.Fatalf("expected 3 skills, got %d: %+v", len(r.Skills), r.Skills)
	}

	byPath := map[string]brief.Skill{}
	for _, s := range r.Skills {
		byPath[s.Path] = s
	}

	pdf := byPath["skills/pdf/SKILL.md"]
	if pdf.Name != "pdf" || pdf.Description != "Read and fill PDF forms" || pdf.Format != "claude" {
		t.Errorf("pdf skill = %+v", pdf)
	}
	excel := byPath[".claude/skills/excel/SKILL.md"]
	if excel.Name != "excel-tools" || excel.Description != "Generate spreadsheets" {
		t.Errorf("excel skill = %+v", excel)
	}
	empty := byPath["skills/empty/SKILL.md"]
	if empty.Name != "empty" || empty.Description != "" {
		t.Errorf("empty skill should fall back to dir name, got %+v", empty)
	}
}

func TestDetectSkillsMalformedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "skills/broken/SKILL.md", `---
name: [unclosed
---
`)
	engine := New(loadKB(t), dir)
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(r.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(r.Skills))
	}
	if r.Skills[0].Name != "broken" {
		t.Errorf("expected fallback to dir name, got %q", r.Skills[0].Name)
	}
}

func TestDetectSkillsNone(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "x")
	engine := New(loadKB(t), dir)
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Skills != nil {
		t.Errorf("expected nil skills, got %+v", r.Skills)
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

func TestCargoWorkspaceMemberDependencies(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", `[workspace]
members = ["crates/*"]
`)
	writeFile(t, dir, "crates/app/Cargo.toml", `[package]
name = "app"
version = "0.1.0"
edition = "2021"

[dependencies]
reqwest = "0.12"
`)
	writeFile(t, dir, "crates/app/src/lib.rs", "pub fn app() {}\n")

	r, err := New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertToolDetected(t, r, "monorepo", "Cargo workspaces")
	assertToolDetected(t, r, "library", "reqwest")
}

func TestGoWorkspaceMemberDependencies(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.work", `go 1.22

use (
	./services/api
)
`)
	writeFile(t, dir, "services/api/go.mod", `module example.com/api

go 1.22

require github.com/go-resty/resty/v2 v2.16.5
`)
	writeFile(t, dir, "services/api/main.go", "package main\nfunc main() {}\n")

	r, err := New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertToolDetected(t, r, "monorepo", "Go workspace")
	assertToolDetected(t, r, "library", "Resty")
}

func TestPackageWorkspaceMemberDependencies(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{
  "private": true,
  "workspaces": ["packages/*"]
}`)
	writeFile(t, dir, "packages/web/package.json", `{
  "dependencies": {
    "axios": "^1.7.0"
  }
}`)
	writeFile(t, dir, "packages/web/index.js", "import axios from 'axios'\n")

	r, err := New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertToolDetected(t, r, "monorepo", "Yarn workspaces")
	assertToolDetected(t, r, "library", "axios")
}

func TestPnpmWorkspaceMemberDependencies(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"private": true}`)
	writeFile(t, dir, "pnpm-workspace.yaml", "packages:\n  - packages/*\n")
	writeFile(t, dir, "packages/web/package.json", `{
  "dependencies": {
    "axios": "^1.7.0"
  }
}`)
	writeFile(t, dir, "packages/web/index.js", "import axios from 'axios'\n")

	r, err := New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertToolDetected(t, r, "monorepo", "pnpm workspaces")
	assertToolDetected(t, r, "library", "axios")
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

	// Detection-only library defs via dependencies primitive
	assertToolDetected(t, r, "library", "requests")
	assertToolDetected(t, r, "library", "Jinja2")

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

func TestPerlProject(t *testing.T) {
	r := runOn(t, "../testdata/perl-project")

	if !slices.ContainsFunc(r.Languages, func(d brief.Detection) bool { return d.Name == "Perl" }) {
		t.Fatalf("expected Perl language, got %v", languageNames(r))
	}
	// Perl repos should get a package_managers entry so consumers that key
	// on package_managers[].name (rather than languages) see the ecosystem.
	if !slices.ContainsFunc(r.PackageManagers, func(d brief.Detection) bool { return d.Name == "cpanm" }) {
		t.Errorf("expected cpanm package manager, got %v", r.PackageManagers)
	}
}

func TestNativeExtensionProject(t *testing.T) {
	r := runOn(t, "../testdata/native-ext-project")

	// One fixture with markers for each ecosystem's native-extension build
	// tool: extconf.rb under ext/, setup.py declaring Extension(), config.m4
	// with PHP_ARG_, and binding.gyp.
	assertToolDetected(t, r, "native_extension", "mkmf")
	assertToolDetected(t, r, "native_extension", "setuptools Extension")
	assertToolDetected(t, r, "native_extension", "phpize")
	assertToolDetected(t, r, "native_extension", "node-gyp")

	// The plain ruby-project fixture has no ext/ so must NOT report a
	// native extension: guards against an over-broad detect.files pattern.
	rr := runOn(t, "../testdata/ruby-project")
	if got := rr.Tools["native_extension"]; len(got) != 0 {
		t.Errorf("ruby-project should not detect native_extension, got %v", got)
	}
}

func TestGradleJavaKotlinDSL(t *testing.T) {
	// Regression for #84: a Java project that uses the Kotlin DSL for its
	// Gradle build scripts must be reported as Java, not Kotlin.
	r := runOn(t, "../testdata/gradle-java-kotlin-dsl")

	if len(r.Languages) != 1 || r.Languages[0].Name != "Java" {
		t.Fatalf("expected only Java language, got %v", languageNames(r))
	}
	if !slices.Contains(packageManagerNames(r), "Gradle") {
		t.Errorf("expected Gradle package manager, got %v", packageManagerNames(r))
	}
}

func TestGradleJavaGroovyDSL(t *testing.T) {
	// Regression for #84: with build.gradle in app/ and source under
	// app/src/main/java/<pkg>/, both Java and Gradle must be detected.
	r := runOn(t, "../testdata/gradle-java-groovy-dsl")

	if len(r.Languages) == 0 || r.Languages[0].Name != "Java" {
		t.Fatalf("expected Java language, got %v", languageNames(r))
	}
	for _, l := range r.Languages {
		if l.Name == "Groovy" {
			t.Errorf("did not expect Groovy language for build script only, got %v", languageNames(r))
		}
	}
	if !slices.Contains(packageManagerNames(r), "Gradle") {
		t.Errorf("expected Gradle package manager, got %v", packageManagerNames(r))
	}
}

func TestScanDepthOverride(t *testing.T) {
	engine := New(loadKB(t), "../testdata/gradle-java-groovy-dsl")
	engine.ScanDepth = 2
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, l := range r.Languages {
		if l.Name == "Java" {
			t.Errorf("expected ScanDepth=2 to miss app/src/main/java/..., got %v", languageNames(r))
		}
	}
}

func languageNames(r *brief.Report) []string {
	names := make([]string, 0, len(r.Languages))
	for _, l := range r.Languages {
		names = append(names, l.Name)
	}
	return names
}

func writeProjectFile(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runOn(t *testing.T, dir string) *brief.Report {
	t.Helper()
	r, err := New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return r
}

func packageManagerNames(r *brief.Report) []string {
	names := make([]string, 0, len(r.PackageManagers))
	for _, pm := range r.PackageManagers {
		names = append(names, pm.Name)
	}
	return names
}

func TestPythonPackageManagerManifests(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		content string
		want    string
	}{
		{"setup.py", "setup.py", "from setuptools import setup\nsetup()\n", "setuptools"},
		{"setup.cfg", "setup.cfg", "[metadata]\nname = x\n", "setuptools"},
		{"pyproject setuptools", "pyproject.toml", "[build-system]\nrequires = [\"setuptools\"]\n", "setuptools"},
		{"pyproject hatchling", "pyproject.toml", "[build-system]\nbuild-backend = \"hatchling.build\"\n", "Hatch"},
		{"pyproject flit", "pyproject.toml", "[build-system]\nrequires = [\"flit_core\"]\n", "Flit"},
		{"Pipfile", "Pipfile", "[packages]\n", "Pipenv"},
		{"requirements.txt", "requirements.txt", "requests\n", "pip"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectFile(t, dir, "main.py", "")
			writeProjectFile(t, dir, tc.file, tc.content)
			r := runOn(t, dir)
			got := packageManagerNames(r)
			found := false
			for _, n := range got {
				if n == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("manifest %s: want %q in package managers, got %v", tc.file, tc.want, got)
			}
		})
	}
}

func TestPythonFlatLayout(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "setup.py", "")
	writeProjectFile(t, dir, "mypkg/__init__.py", "")
	writeProjectFile(t, dir, "mypkg/core.py", "")
	writeProjectFile(t, dir, "tests/test_core.py", "")
	writeProjectFile(t, dir, "docs/index.md", "")
	writeProjectFile(t, dir, "scripts/release.py", "")

	r := runOn(t, dir)
	if r.Layout == nil {
		t.Fatal("expected layout info")
	}
	if got := r.Layout.SourceDirs; len(got) != 1 || got[0] != "mypkg" {
		t.Errorf("source_dirs = %v, want [mypkg]", got)
	}
	for _, d := range r.Layout.SourceDirs {
		if d == "docs" || d == "scripts" || d == "tests" {
			t.Errorf("source_dirs should not include %q", d)
		}
	}
	foundTests := false
	for _, d := range r.Layout.TestDirs {
		if d == "tests" {
			foundTests = true
		}
	}
	if !foundTests {
		t.Error("expected tests/ in test_dirs")
	}
}

func TestFlatLayoutSkippedWhenSrcExists(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "src/mypkg/__init__.py", "")
	writeProjectFile(t, dir, "helper/tool.py", "")

	r := runOn(t, dir)
	if r.Layout == nil {
		t.Fatal("expected layout info")
	}
	if got := r.Layout.SourceDirs; len(got) != 1 || got[0] != "src" {
		t.Errorf("source_dirs = %v, want [src]", got)
	}
}

func TestFlatLayoutNonPython(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "Gemfile", "source 'https://rubygems.org'\n")
	writeProjectFile(t, dir, "mygem/version.rb", "VERSION = '1.0'\n")
	writeProjectFile(t, dir, "examples/demo.rb", "")
	writeProjectFile(t, dir, "spec/mygem_spec.rb", "")

	r := runOn(t, dir)
	if r.Layout == nil {
		t.Fatal("expected layout info")
	}
	if got := r.Layout.SourceDirs; len(got) != 1 || got[0] != "mygem" {
		t.Errorf("source_dirs = %v, want [mygem]", got)
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

func TestDetectSelf(t *testing.T) {
	dir := t.TempDir()
	gomod := "module github.com/git-pkgs/brief\n\ngo 1.22.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertToolDetected(t, r, "introspection", "brief")
}

func TestDetectSelfNotTriggeredOnOtherGoProjects(t *testing.T) {
	r, err := New(loadKB(t), "../testdata/go-project").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := r.Tools["introspection"]; ok {
		t.Error("introspection category should not appear for non-brief go projects")
	}
}

func TestTaskRunnerDetection(t *testing.T) {
	cases := []struct {
		name     string
		files    map[string]string
		category string
		command  string
	}{
		{
			name:     "Just",
			files:    map[string]string{"justfile": "default:\n\techo hi\n"},
			category: "build",
			command:  "just --list",
		},
		{
			name:     "Task",
			files:    map[string]string{"Taskfile.yml": "version: '3'\ntasks:\n  hello:\n    cmds: [echo hi]\n"},
			category: "build",
			command:  "task --list-all",
		},
		{
			name:     "Pixi",
			files:    map[string]string{"pixi.toml": "[project]\nname = \"x\"\n"},
			category: "environment",
			command:  "pixi install",
		},
		{
			name: "Spin",
			files: map[string]string{
				"x.py":           "x = 1\n",
				"pyproject.toml": "[project]\nname = \"x\"\n[tool.spin]\npackage = \"x\"\n",
			},
			category: "build",
			command:  "spin",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			r, err := New(loadKB(t), dir).Run()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertToolDetected(t, r, tc.category, tc.name)
			for _, d := range r.Tools[tc.category] {
				if d.Name == tc.name {
					if d.Command == nil || d.Command.Run != tc.command {
						t.Errorf("expected command %q, got %v", tc.command, d.Command)
					}
				}
			}
		})
	}
}

func TestInvokeDetection(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.py"), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tasks := "from invoke import task\n\n@task\ndef build(c):\n    c.run('true')\n"
	if err := os.WriteFile(filepath.Join(dir, "tasks.py"), []byte(tasks), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertToolDetected(t, r, "build", "Invoke")
	for _, d := range r.Tools["build"] {
		if d.Name == "Invoke" {
			if d.Command == nil || d.Command.Run != "invoke --list" {
				t.Errorf("expected 'invoke --list' command, got %v", d.Command)
			}
		}
	}

	// A tasks.py that doesn't import invoke should not trigger detection.
	celery := "from celery import shared_task\n\n@shared_task\ndef foo():\n    pass\n"
	if err := os.WriteFile(filepath.Join(dir, "tasks.py"), []byte(celery), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err = New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, d := range r.Tools["build"] {
		if d.Name == "Invoke" {
			t.Error("tasks.py without invoke import should not trigger Invoke detection")
		}
	}
}

func TestGlobIgnoresDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"NEWS.d", "scratch.js"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	r, err := New(loadKB(t), dir).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, lang := range r.Languages {
		if lang.Name == "D" || lang.Name == "JavaScript" {
			t.Errorf("directory with source extension should not trigger %s detection", lang.Name)
		}
	}
	if len(r.Languages) == 0 || r.Languages[0].Name != "Go" {
		t.Errorf("expected Go as primary language, got %v", r.Languages)
	}
}

func TestDirectoryGlobPattern(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Foo.xcodeproj"), 0o755); err != nil {
		t.Fatal(err)
	}

	engine := New(loadKB(t), dir)
	if !engine.exists("*.xcodeproj/") {
		t.Error("expected *.xcodeproj/ to match Foo.xcodeproj directory")
	}
	if engine.exists("*.xcodeproj") {
		t.Error("*.xcodeproj without trailing slash should not match a directory")
	}
}

func TestTrackedOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	dir := t.TempDir()
	gitCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	gitCmd("init", "-q")
	write("main.go", "package main\n")
	write("go.mod", "module example.com/m\n\ngo 1.22\n")
	gitCmd("add", "main.go", "go.mod")
	gitCmd("commit", "-q", "-m", "init")

	// Untracked noise that would normally trigger npm/JS detection.
	write("package.json", `{"name":"x"}`)
	write("package-lock.json", `{"name":"x","lockfileVersion":3}`)
	write("htmlcov/index.html", "<html></html>")
	write("htmlcov/app.js", "var x = 1;")

	engine := New(loadKB(t), dir)
	engine.TrackedOnly = true
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, lang := range r.Languages {
		if lang.Name == "JavaScript" {
			t.Errorf("untracked .js files should not trigger JavaScript detection, got languages: %v", langNames(r))
		}
	}
	for _, pm := range r.PackageManagers {
		if pm.Name == "npm" {
			t.Errorf("untracked package.json should not trigger npm detection, got package managers: %v", pmNames(r))
		}
	}
	if len(r.Languages) == 0 || r.Languages[0].Name != "Go" {
		t.Errorf("expected Go as primary language, got %v", langNames(r))
	}

	// Without TrackedOnly the untracked files should be picked up.
	engine = New(loadKB(t), dir)
	r, err = engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sawNpm := false
	for _, pm := range r.PackageManagers {
		if pm.Name == "npm" {
			sawNpm = true
		}
	}
	if !sawNpm {
		t.Errorf("expected npm to be detected without -tracked, got %v", pmNames(r))
	}
}

func TestTrackedOnlyNotARepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	engine := New(loadKB(t), dir)
	engine.TrackedOnly = true
	if _, err := engine.Run(); err == nil {
		t.Error("expected error when -tracked is used outside a git repository")
	}
}

func langNames(r *brief.Report) []string {
	names := make([]string, len(r.Languages))
	for i, l := range r.Languages {
		names[i] = l.Name
	}
	return names
}

func pmNames(r *brief.Report) []string {
	names := make([]string, len(r.PackageManagers))
	for i, p := range r.PackageManagers {
		names[i] = p.Name
	}
	return names
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
