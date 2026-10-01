package detect

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestLanguageEvidenceByRole(t *testing.T) {
	t.Setenv("PATH", "")
	for _, name := range []string{
		"evals/fixtures/sqli/app.py",
		"skills/semgrep/scripts/scan.py",
		"docs/conf.py",
		"examples/client/app.py",
		"samples/client/app.py",
		"spec/app.py",
		"features/app.py",
		"src/test_app.py",
		"src/service_pb2.py",
		"src/generated/client.py",
		"src/vendor/app.py",
		"bower_components/app.py",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectFile(t, dir, "main.go", "package main\n")
			writeProjectFile(t, dir, name, "print('fixture')\n")
			r := runOn(t, dir)
			if got := languageNames(r); !slices.Equal(got, []string{"Go"}) {
				t.Errorf("languages = %v, want only Go", got)
			}
		})
	}
}

func TestLanguageRankingExcludesTestAndGeneratedFiles(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	for _, name := range []string{"main.go", "internal/a_test.go", "internal/b_test.go", "service.pb.go", "zz_generated_types.go"} {
		writeProjectFile(t, dir, name, "package main\n")
	}
	writeProjectFile(t, dir, "app.py", "print('app')\n")
	writeProjectFile(t, dir, "worker.py", "print('worker')\n")
	if got := languageNames(runOn(t, dir)); !slices.Equal(got, []string{"Python", "Go"}) {
		t.Errorf("languages = %v, want Python then Go", got)
	}
}

func TestLanguageRankingDoesNotCountManifestsAsSource(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	writeProjectFile(t, dir, "main.go", "package main\n")
	for _, name := range []string{"parser.gemspec", "lexer.gemspec", "src/parser.gemspec", "src/lexer.gemspec"} {
		writeProjectFile(t, dir, name, "Gem::Specification.new do |spec|\n  spec.name = 'parser'\nend\n")
	}
	if got := languageNames(runOn(t, dir)); !slices.Equal(got, []string{"Go", "Ruby"}) {
		t.Errorf("languages = %v, want Go source before Ruby manifests", got)
	}
}

func TestRoleFilteringPreservesManifestLanguageAndTestTools(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	writeProjectFile(t, dir, "Gemfile", "source 'https://rubygems.org'\n")
	writeProjectFile(t, dir, "spec/app_spec.rb", "describe 'app' do\nend\n")
	r := runOn(t, dir)
	if got := languageNames(r); !slices.Equal(got, []string{"Ruby"}) {
		t.Errorf("languages = %v, want Ruby from Gemfile", got)
	}
	assertToolDetected(t, r, "test", "RSpec")
}

func TestRoleFilteringPreservesRubyManifestEvidence(t *testing.T) {
	t.Setenv("PATH", "")
	for name, content := range map[string]string{
		"parser.gemspec": "Gem::Specification.new do |spec|\n  spec.name = 'parser'\n  spec.version = '0.1.0'\nend\n",
		"gems.rb":        "source 'https://rubygems.org'\n",
		"Rakefile":       "require 'hoe'\nHoe.spec 'parser'\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeProjectFile(t, root, "Gemfile", "source 'https://rubygems.org'\ngem 'rubocop'\n")
			writeProjectFile(t, root, "packages/parser/"+name, content)
			r := runOn(t, filepath.Join(root, "packages", "parser"))
			if got := languageNames(r); !slices.Equal(got, []string{"Ruby"}) {
				t.Errorf("languages = %v, want Ruby from %s", got, name)
			}
			assertToolNotDetected(t, r, "lint", "RuboCop")
		})
	}
}

func TestRoleFilteringToolAndDependencyEvidence(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	writeProjectFile(t, dir, "Cargo.toml", "[package]\nname = \"app\"\nversion = \"0.1.0\"\n[workspace]\nmembers = [\"fixtures/*\", \"packages/*\"]\n")
	for _, name := range []string{"fixtures/demo/Cargo.toml", "bower_components/demo/Cargo.toml"} {
		writeProjectFile(t, dir, name, "[package]\nname = \"demo\"\nversion = \"0.1.0\"\n[dependencies]\naxum = \"0.8\"\n")
	}
	writeProjectFile(t, dir, "packages/core/Cargo.toml", "[package]\nname = \"core\"\nversion = \"0.1.0\"\n[dependencies]\nserde = \"1\"\n")
	writeProjectFile(t, dir, "Cargo.lock", "version = 3\n[[package]]\nname = \"serde\"\nversion = \"1.0.0\"\nsource = \"registry+https://github.com/rust-lang/crates.io-index\"\n")
	writeProjectFile(t, dir, "fixtures/deploy.yaml", "apiVersion: argoproj.io/v1alpha1\nkind: Application\n")
	r := runOn(t, dir)
	for _, detections := range r.Tools {
		for _, tool := range detections {
			if tool.Name == "Axum" || tool.Name == "Argo CD" {
				t.Errorf("fixture triggered %s", tool.Name)
			}
		}
	}
	var foundMember, foundLock bool
	for _, manifest := range r.Manifests {
		switch manifest.Path {
		case "fixtures/demo/Cargo.toml", "bower_components/demo/Cargo.toml":
			t.Errorf("unexpected manifest %s", manifest.Path)
		case "packages/core/Cargo.toml":
			foundMember = true
		case "Cargo.lock":
			foundLock = true
		}
	}
	if !foundMember || !foundLock {
		t.Errorf("workspace member or lockfile missing: %+v", r.Manifests)
	}
	for _, dep := range r.Dependencies {
		if dep.Name == "axum" {
			t.Error("fixture dependency included in report")
		}
	}
}

func TestRoleFilteringStyleAndFlatLayout(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	writeProjectFile(t, dir, "service/main.go", "package main\nfunc main() {\n\tprintln(1)\n}\n")
	for _, name := range []string{"aaa/types.pb.go", "aaa/main_test.go", "fixtures/main.go", "examples/main.go", "scripts/main.go"} {
		writeProjectFile(t, dir, name, "package main\r\nfunc main() {\r\n    println(1)\r\n    println(2)\r\n}\r\n")
	}
	r := runOn(t, dir)
	if r.Style == nil || r.Style.Indentation != "tabs" || r.Style.LineEnding != "LF" {
		t.Errorf("style = %+v, want tabs and LF from source", r.Style)
	}
	if r.Layout == nil || !slices.Equal(r.Layout.SourceDirs, []string{"service"}) {
		t.Errorf("layout = %+v, want service as the only source directory", r.Layout)
	}
}
