package detect

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
)

const renvDescription = `Package: RenvInput
Version: 0.0.0
Title: Renv input
Description: Renv input.
Imports: BiocGenerics, survival
`

const renvLock = `{
  "R": {
    "Version": "4.5.0",
    "Repositories": [{"Name": "CRAN", "URL": "https://cloud.r-project.org"}]
  },
  "Packages": {
    "BiocGenerics": {
      "Package": "BiocGenerics",
      "Version": "0.58.1",
      "Source": "Bioconductor",
      "Repository": "Bioconductor"
    },
    "survival": {
      "Package": "survival",
      "Version": "2.34",
      "Source": "Repository",
      "Repository": "CRAN",
      "Hash": "0123456789abcdef0123456789abcdef"
    },
    "ghpkg": {
      "Package": "ghpkg",
      "Version": "0.5.0",
      "Source": "GitHub",
      "RemoteUsername": "acme",
      "RemoteRepo": "ghpkg",
      "RemoteSha": "0123abc"
    }
  }
}
`

func assertRenvLockDependencies(t *testing.T, r *brief.Report, lockfile string) {
	t.Helper()
	wantManifest := brief.ManifestInfo{Ecosystem: "cran", Path: lockfile, Kind: "lockfile"}
	if !slices.Contains(r.Manifests, wantManifest) {
		t.Errorf("manifests should contain %+v, got %+v", wantManifest, r.Manifests)
	}

	for _, want := range []brief.DepInfo{
		{
			Manifest: lockfile,
			Name:     "BiocGenerics",
			Version:  "0.58.1",
			PURL:     "pkg:bioconductor/BiocGenerics@0.58.1",
			Scope:    brief.ScopeRuntime,
			Source:   &brief.DepSource{Kind: "registry", Value: "Bioconductor"},
		},
		{
			Manifest: lockfile,
			Name:     "survival",
			Version:  "2.34",
			PURL:     "pkg:cran/survival@2.34",
			Scope:    brief.ScopeRuntime,
			Source:   &brief.DepSource{Kind: "registry", Value: "CRAN"},
		},
	} {
		if !slices.ContainsFunc(r.Dependencies, func(dep brief.DepInfo) bool {
			return dep.Manifest == want.Manifest && dep.Name == want.Name &&
				dep.Version == want.Version && dep.PURL == want.PURL &&
				dep.Scope == want.Scope && !dep.Direct &&
				dep.Source != nil && *dep.Source == *want.Source
		}) {
			t.Errorf("dependencies should contain locked %s %+v, got %+v", want.Name, want, r.Dependencies)
		}
	}

	// A GitHub package has no registry identity, so it must not be reported
	// as a CRAN package.
	for _, dep := range r.Dependencies {
		if dep.Name == "ghpkg" && strings.HasPrefix(dep.PURL, "pkg:cran/") {
			t.Errorf("GitHub package reported with CRAN identity: %+v", dep)
		}
	}
}

func TestRenvLockfileAlone(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "renv.lock", renvLock)

	r := runOn(t, dir)
	assertRenvLockDependencies(t, r, "renv.lock")
	for _, dep := range r.Dependencies {
		if dep.Manifest != "renv.lock" {
			t.Errorf("unexpected dependency from %s: %+v", dep.Manifest, dep)
		}
	}
}

func TestRenvLockfileWithDescription(t *testing.T) {
	// https://github.com/git-pkgs/brief/issues/214
	dir := t.TempDir()
	writeProjectFile(t, dir, "DESCRIPTION", renvDescription)
	writeProjectFile(t, dir, "renv.lock", renvLock)

	r := runOn(t, dir)
	if !slices.Contains(packageManagerNames(r), "renv") {
		t.Errorf("package managers = %v, want renv", packageManagerNames(r))
	}
	wantManifest := brief.ManifestInfo{Ecosystem: "cran", Path: "DESCRIPTION", Kind: "manifest"}
	if !slices.Contains(r.Manifests, wantManifest) {
		t.Errorf("manifests should contain %+v, got %+v", wantManifest, r.Manifests)
	}
	assertRenvLockDependencies(t, r, "renv.lock")

	// DESCRIPTION declarations stay separate, direct and unversioned.
	for _, name := range []string{"BiocGenerics", "survival"} {
		if !slices.ContainsFunc(r.Dependencies, func(dep brief.DepInfo) bool {
			return dep.Manifest == "DESCRIPTION" && dep.Name == name && dep.Direct && dep.Version == ""
		}) {
			t.Errorf("dependencies should contain the DESCRIPTION declaration of %s, got %+v", name, r.Dependencies)
		}
	}
}

func TestRenvLockfileInIncludedSubmodule(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	child := initGitProject(t, map[string]string{
		"DESCRIPTION": renvDescription,
		"renv.lock":   renvLock,
	})
	parent := initGitProject(t, map[string]string{
		"main.py": "print('example')\n",
	})
	addGitSubmodule(t, parent, child, "vendor/analysis")
	gitFixtureCommand(t, parent, "-c", "protocol.file.allow=always", "submodule", "update", "--init", "--recursive")

	without := runOn(t, parent)
	for _, manifest := range without.Manifests {
		if manifest.Path == "vendor/analysis/renv.lock" {
			t.Errorf("default scan collected a submodule lockfile: %+v", manifest)
		}
	}

	engine := New(loadKB(t), parent)
	engine.IncludeSubmodules = true
	engine.LineCountTimeout = 0
	with, err := engine.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertRenvLockDependencies(t, with, "vendor/analysis/renv.lock")
}
