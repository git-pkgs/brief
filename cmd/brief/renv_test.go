package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestScanRenvLockfile(t *testing.T) {
	// https://github.com/git-pkgs/brief/issues/214
	const helperEnv = "BRIEF_RENV_LOCKFILE_ROOT"
	if root := os.Getenv(helperEnv); root != "" {
		cmdScan([]string{"-json", root})
		return
	}

	root := t.TempDir()
	writeScanFixture(t, root, "DESCRIPTION", "Package: RenvInput\nVersion: 0.0.0\nTitle: Renv input\n"+
		"Description: Renv input.\nImports: BiocGenerics, survival\n")
	writeScanFixture(t, root, "renv.lock", `{"Packages": {
  "BiocGenerics": {"Package": "BiocGenerics", "Version": "0.58.1", "Source": "Bioconductor", "Repository": "Bioconductor"},
  "survival": {"Package": "survival", "Version": "2.34", "Source": "Repository", "Repository": "CRAN"}
}}`)

	cmd := exec.Command(os.Args[0], "-test.run=^TestScanRenvLockfile$")
	cmd.Env = append(os.Environ(), helperEnv+"="+root, "PATH=")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	var report brief.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parsing scan output: %v\n%s", err, out)
	}

	wantManifest := brief.ManifestInfo{Ecosystem: "cran", Path: "renv.lock", Kind: "lockfile"}
	if !slices.Contains(report.Manifests, wantManifest) {
		t.Errorf("manifests should contain %+v, got %+v", wantManifest, report.Manifests)
	}
	for name, purl := range map[string]string{
		"BiocGenerics": "pkg:bioconductor/BiocGenerics@0.58.1",
		"survival":     "pkg:cran/survival@2.34",
	} {
		if !slices.ContainsFunc(report.Dependencies, func(dep brief.DepInfo) bool {
			return dep.Manifest == "renv.lock" && dep.Name == name && dep.PURL == purl && !dep.Direct
		}) {
			t.Errorf("dependencies should contain locked %s as %s, got %+v", name, purl, report.Dependencies)
		}
	}
}
