package detect

import (
	"slices"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestRubyGemsPackageManager(t *testing.T) {
	t.Setenv("PATH", "")
	for name, files := range map[string]map[string]string{
		"gemspec only":       {"parser.gemspec": "Gem::Specification.new do |spec|\n  spec.name = 'parser'\nend\n"},
		"nested gemspec":     {"packages/parser/parser.gemspec": "Gem::Specification.new do |spec|\n  spec.name = 'parser'\nend\n"},
		"Hoe":                {"Rakefile": "require 'hoe'\nHoe.spec 'parser' do\nend\n"},
		"lowercase Rakefile": {"rakefile": "require 'hoe'\nHoe.spec 'parser' do\nend\n"},
		"legacy Hoe":         {"Rakefile": "require 'hoe'\nHoe.new 'parser', '0.1.0'\n"},
		"package task":       {"Rakefile": "require 'rubygems/package_task'\nGem::PackageTask.new(spec)\n"},
		"plain Ruby":         {"main.rb": "puts 'hello'\n"},
		"ordinary Rakefile":  {"Rakefile": "task :test do\n  ruby 'test.rb'\nend\n"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for path, content := range files {
				writeProjectFile(t, dir, path, content)
			}
			r := runOn(t, dir)
			if !slices.Contains(languageNames(r), "Ruby") {
				t.Fatalf("languages = %v, want Ruby", languageNames(r))
			}
			want := name != "plain Ruby" && name != "ordinary Rakefile"
			if got := slices.Contains(packageManagerNames(r), "RubyGems"); got != want {
				t.Errorf("package managers = %v, want RubyGems presence %v", packageManagerNames(r), want)
			}
			if slices.Contains(packageManagerNames(r), "Bundler") {
				t.Error("detected Bundler without a Bundler manifest")
			}
		})
	}
}

func TestBundlerManifestLayouts(t *testing.T) {
	t.Setenv("PATH", "")
	const gemfile = "source 'https://rubygems.org'\ngem 'rubocop'\n"
	const gemsRB = "source 'https://rubygems.org'\ngem 'rake'\n"
	const oldLock = "GEM\n  remote: https://rubygems.org/\n  specs:\n    rubocop (1.0.0)\n\nDEPENDENCIES\n  rubocop\n"
	const newLock = "GEM\n  remote: https://rubygems.org/\n  specs:\n    rake (13.0.0)\n\nDEPENDENCIES\n  rake\n"
	for _, tt := range []struct {
		name       string
		files      map[string]string
		manifest   string
		lockfile   string
		dependency string
	}{
		{"Gemfile", map[string]string{"Gemfile": gemfile, "Gemfile.lock": oldLock}, "Gemfile", "Gemfile.lock", "rubocop"},
		{"gems.rb", map[string]string{"gems.rb": gemsRB, "gems.locked": newLock}, "gems.rb", "gems.locked", "rake"},
		{"both layouts", map[string]string{"Gemfile": gemfile, "Gemfile.lock": oldLock, "gems.rb": gemsRB, "gems.locked": newLock}, "gems.rb", "gems.locked", "rake"},
		{"missing alternate lockfile", map[string]string{"Gemfile": gemfile, "Gemfile.lock": oldLock, "gems.rb": gemsRB}, "gems.rb", "", "rake"},
		{"orphan alternate lockfile", map[string]string{"Gemfile": gemfile, "gems.locked": newLock}, "Gemfile", "", "rubocop"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for path, content := range tt.files {
				writeProjectFile(t, dir, path, content)
			}
			r := runOn(t, dir)
			if len(r.PackageManagers) != 1 || r.PackageManagers[0].Name != "Bundler" {
				t.Fatalf("package managers = %v, want Bundler", packageManagerNames(r))
			}
			bundler := r.PackageManagers[0]
			if bundler.Lockfile != tt.lockfile {
				t.Errorf("lockfile = %q, want %q", bundler.Lockfile, tt.lockfile)
			}
			for _, path := range []string{tt.manifest, tt.lockfile} {
				if path != "" && !slices.Contains(bundler.ConfigFiles, path) {
					t.Errorf("config files = %v, want %s", bundler.ConfigFiles, path)
				}
			}
			checkBundlerManifests(t, r, tt.manifest, tt.lockfile, tt.dependency)
		})
	}
}

func checkBundlerManifests(t *testing.T, r *brief.Report, manifest, lockfile, dependency string) {
	t.Helper()
	want := []string{manifest}
	if lockfile != "" {
		want = append(want, lockfile)
	}
	var got []string
	for _, m := range r.Manifests {
		got = append(got, m.Path)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("manifests = %v, want %v", got, want)
	}
	if len(r.Dependencies) == 0 {
		t.Fatal("expected parsed dependencies")
	}
	for _, dep := range r.Dependencies {
		if dep.Name != dependency {
			t.Errorf("dependency = %s, want only %s", dep.Name, dependency)
		}
	}
}

func TestRubyGemsAndBundler(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	writeProjectFile(t, dir, "Gemfile", "source 'https://rubygems.org'\ngemspec\n")
	writeProjectFile(t, dir, "parser.gemspec", "Gem::Specification.new do |spec|\n  spec.name = 'parser'\nend\n")
	got := packageManagerNames(runOn(t, dir))
	if len(got) != 2 || !slices.Contains(got, "Bundler") || !slices.Contains(got, "RubyGems") {
		t.Errorf("package managers = %v, want Bundler and RubyGems", got)
	}
}
