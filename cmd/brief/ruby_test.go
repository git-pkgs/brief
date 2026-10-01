package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestScanRubyPackageManagers(t *testing.T) {
	const helperEnv = "BRIEF_RUBY_PACKAGE_MANAGER_ROOT"
	if root := os.Getenv(helperEnv); root != "" {
		cmdScan([]string{"-json", root})
		return
	}
	for _, tt := range []struct {
		name     string
		path     string
		files    map[string]string
		manager  string
		lockfile string
	}{
		{"gem subdirectory", "packages/parser", map[string]string{
			"Gemfile":                        "source 'https://rubygems.org'\ngem 'rubocop'\n",
			"packages/parser/parser.gemspec": "Gem::Specification.new do |spec|\n  spec.name = 'parser'\nend\n",
		}, "RubyGems", ""},
		{"alternate Bundler layout", ".", map[string]string{
			"gems.rb":     "source 'https://rubygems.org'\ngem 'rake'\n",
			"gems.locked": "GEM\n  specs:\n    rake (13.0.0)\n\nDEPENDENCIES\n  rake\n",
		}, "Bundler", "gems.locked"},
		{"Hoe project", ".", map[string]string{
			"Rakefile": "require 'hoe'\nHoe.spec 'parser' do\nend\n",
		}, "RubyGems", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for path, content := range tt.files {
				writeScanFixture(t, root, path, content)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestScanRubyPackageManagers$")
			cmd.Env = append(os.Environ(), helperEnv+"="+filepath.Join(root, tt.path), "PATH=")
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("scan failed: %v", err)
			}
			var report brief.Report
			if err := json.Unmarshal(out, &report); err != nil {
				t.Fatalf("parsing scan output: %v\n%s", err, out)
			}
			if len(report.PackageManagers) != 1 || report.PackageManagers[0].Name != tt.manager {
				t.Fatalf("package managers = %+v, want %s", report.PackageManagers, tt.manager)
			}
			if got := report.PackageManagers[0].Lockfile; got != tt.lockfile {
				t.Errorf("lockfile = %q, want %q", got, tt.lockfile)
			}
		})
	}
}
