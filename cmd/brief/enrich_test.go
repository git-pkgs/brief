package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGoModulePURL(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module github.com/example/foo\n\ngo 1.22.0\n")

	got := goModulePURL(dir)
	want := "pkg:golang/github.com/example/foo"
	if got != want {
		t.Errorf("goModulePURL() = %q, want %q", got, want)
	}
}

func TestGoModulePURL_Missing(t *testing.T) {
	dir := t.TempDir()
	if got := goModulePURL(dir); got != "" {
		t.Errorf("expected empty string for missing go.mod, got %q", got)
	}
}

func TestNpmPackagePURL(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "my-package", "version": "1.0.0"}`)

	got := npmPackagePURL(dir)
	want := "pkg:npm/my-package"
	if got != want {
		t.Errorf("npmPackagePURL() = %q, want %q", got, want)
	}
}

func TestNpmPackagePURL_Scoped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "@scope/my-package", "version": "1.0.0"}`)

	got := npmPackagePURL(dir)
	want := "pkg:npm/%40scope/my-package"
	if got != want {
		t.Errorf("npmPackagePURL() = %q, want %q", got, want)
	}
}

func TestNpmPackagePURL_Private(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "my-package", "private": true}`)

	if got := npmPackagePURL(dir); got != "" {
		t.Errorf("expected empty string for private package, got %q", got)
	}
}

func TestPythonPackagePURL(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "[project]\nname = \"my-lib\"\n")

	got := pythonPackagePURL(dir)
	want := "pkg:pypi/my-lib"
	if got != want {
		t.Errorf("pythonPackagePURL() = %q, want %q", got, want)
	}
}

func TestPythonPackagePURL_SetupCfg(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "setup.cfg", "[metadata]\nname = my-lib\nversion = 1.0\n")

	got := pythonPackagePURL(dir)
	want := "pkg:pypi/my-lib"
	if got != want {
		t.Errorf("pythonPackagePURL() = %q, want %q", got, want)
	}
}

func TestGemPURL(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "my_gem.gemspec", `Gem::Specification.new do |spec|
  spec.name = "my_gem"
  spec.version = "1.0.0"
end
`)

	got := gemPURL(dir)
	want := "pkg:gem/my_gem"
	if got != want {
		t.Errorf("gemPURL() = %q, want %q", got, want)
	}
}

func TestGemPURL_SingleQuotes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo.gemspec", `Gem::Specification.new do |s|
  s.name = 'foo'
end
`)

	got := gemPURL(dir)
	want := "pkg:gem/foo"
	if got != want {
		t.Errorf("gemPURL() = %q, want %q", got, want)
	}
}

func TestGemPURL_Missing(t *testing.T) {
	dir := t.TempDir()
	if got := gemPURL(dir); got != "" {
		t.Errorf("expected empty string for missing gemspec, got %q", got)
	}
}

func TestCratePURL(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", "[package]\nname = \"my-crate\"\nversion = \"0.1.0\"\n")

	got := cratePURL(dir)
	want := "pkg:cargo/my-crate"
	if got != want {
		t.Errorf("cratePURL() = %q, want %q", got, want)
	}
}

func TestCratePURL_Unpublished(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", "[package]\nname = \"my-crate\"\npublish = false\n")

	if got := cratePURL(dir); got != "" {
		t.Errorf("expected empty string for unpublished crate, got %q", got)
	}
}

func TestMajorMinor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"3.12.1", "3.12"},
		{"20.10", "20.10"},
		{"3", "3"},
		{"  3.12.1  ", "3.12"},
	}
	for _, tt := range tests {
		if got := majorMinor(tt.input); got != tt.want {
			t.Errorf("majorMinor(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCapitalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ruby", "Ruby"},
		{"", ""},
		{"Go", "Go"},
	}
	for _, tt := range tests {
		if got := capitalize(tt.input); got != tt.want {
			t.Errorf("capitalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestProductFromFile(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{".ruby-version", "ruby"},
		{".node-version", "nodejs"},
		{".nvmrc", "nodejs"},
		{".python-version", "python"},
		{".go-version", "go"},
		{"rust-toolchain.toml", "rust"},
		{"unknown-file", ""},
	}
	for _, tt := range tests {
		if got := productFromFile(tt.file); got != tt.want {
			t.Errorf("productFromFile(%q) = %q, want %q", tt.file, got, tt.want)
		}
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}
