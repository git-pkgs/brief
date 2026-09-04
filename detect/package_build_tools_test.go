package detect

import (
	"slices"
	"testing"
)

func TestPackageBuildToolSignals(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		tool    string
		config  bool
	}{
		{
			name:    "Conda legacy YAML recipe",
			path:    "meta.yaml",
			content: "package:\n  name: example\n  version: 1.0.0\n",
			tool:    "Conda Build",
			config:  true,
		},
		{
			name:    "Conda legacy YML recipe",
			path:    "meta.yml",
			content: "package:\n  name: example\n  version: 1.0.0\n",
			tool:    "Conda Build",
			config:  true,
		},
		{
			name:    "Conda feedstock YAML recipe",
			path:    "recipe/meta.yaml",
			content: "package:\n  name: example\n  version: 1.0.0\n",
			tool:    "Conda Build",
			config:  true,
		},
		{
			name:    "Conda feedstock YML recipe",
			path:    "recipe/meta.yml",
			content: "package:\n  name: example\n  version: 1.0.0\n",
			tool:    "Conda Build",
			config:  true,
		},
		{
			name:    "Conda v1 YAML recipe",
			path:    "recipe/recipe.yaml",
			content: "schema_version: 1\npackage:\n  name: example\n  version: 1.0.0\n",
			tool:    "Conda Build",
			config:  true,
		},
		{
			name:    "Conda v1 YML recipe",
			path:    "recipe/recipe.yml",
			content: "schema_version: 1\npackage:\n  name: example\n  version: 1.0.0\n",
			tool:    "Conda Build",
			config:  true,
		},
		{
			name:    "Arch SRCINFO metadata",
			path:    ".SRCINFO",
			content: "pkgbase = example\npkgver = 1.0.0\npkgrel = 1\npkgname = example\n",
			tool:    "Arch Linux Packaging",
			config:  true,
		},
		{
			name:    "Arch legacy AURINFO metadata",
			path:    ".AURINFO",
			content: "pkgname = example\npkgver = 1.0.0\npkgrel = 1\n",
			tool:    "Arch Linux Packaging",
			config:  true,
		},
		{
			name:    "Arch package metadata",
			path:    ".PKGINFO",
			content: "pkgname = example\npkgver = 1.0.0-1\narch = any\n",
			tool:    "Arch Linux Packaging",
		},
		{
			name:    "Debian control file",
			path:    "debian/control",
			content: "Source: example\nMaintainer: Example <example@example.com>\n\nPackage: example\nArchitecture: any\n",
			tool:    "Debian Packaging",
			config:  true,
		},
		{
			name:    "Debian source control file",
			path:    "example_1.0-1.dsc",
			content: "Format: 3.0 (quilt)\nSource: example\nVersion: 1.0-1\n",
			tool:    "Debian Packaging",
		},
		{
			name:    "FreeBSD compact manifest",
			path:    "+COMPACT_MANIFEST",
			content: "{\"name\":\"example\",\"version\":\"1.0.0\"}\n",
			tool:    "FreeBSD pkg",
		},
	}

	knowledgeBase := loadKB(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectFile(t, dir, test.path, test.content)

			report, err := New(knowledgeBase, dir).Run()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertToolDetected(t, report, "build", test.tool)
			for _, tool := range report.Tools["build"] {
				if tool.Name == test.tool {
					if got := slices.Contains(tool.ConfigFiles, test.path); got != test.config {
						t.Errorf("config files = %v, contains %q = %v, want %v", tool.ConfigFiles, test.path, got, test.config)
					}
					break
				}
			}
		})
	}
}

func TestPackageBuildToolFixtures(t *testing.T) {
	tests := []struct {
		fixture string
		tool    string
	}{
		{fixture: "conda-build-project", tool: "Conda Build"},
		{fixture: "arch-linux-package-project", tool: "Arch Linux Packaging"},
		{fixture: "debian-package-project", tool: "Debian Packaging"},
		{fixture: "freebsd-package-project", tool: "FreeBSD pkg"},
	}

	for _, test := range tests {
		t.Run(test.tool, func(t *testing.T) {
			report := runOn(t, "../testdata/"+test.fixture)
			assertToolDetected(t, report, "build", test.tool)
		})
	}
}

func TestCondaBuildOffersRattlerBuildForV1Recipes(t *testing.T) {
	report := runOn(t, "../testdata/conda-build-project")
	for _, tool := range report.Tools["build"] {
		if tool.Name != "Conda Build" {
			continue
		}
		if tool.Command == nil || !slices.Contains(tool.Command.Alternatives, "rattler-build build") {
			t.Fatalf("Conda Build command = %+v, want rattler-build alternative", tool.Command)
		}
		return
	}
	t.Fatal("expected Conda Build in build category")
}
