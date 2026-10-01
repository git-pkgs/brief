package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestScanCompactNPMLockfile(t *testing.T) {
	const helperEnv = "BRIEF_COMPACT_NPM_ROOT"
	if root := os.Getenv(helperEnv); root != "" {
		cmdScan([]string{"-json", root})
		return
	}

	root := t.TempDir()
	writeScanFixture(t, root, "index.js", "console.log('hello');\n")
	writeScanFixture(t, root, "package-lock.json", `{"name":"example","lockfileVersion":3,"packages":{"":{"devDependencies":{"eslint":"^9.0.0"}},"node_modules/eslint":{"version":"9.0.0","dev":true},"node_modules/@eslint/js":{"version":"9.0.0","dev":true}}}`)
	cmd := exec.Command(os.Args[0], "-test.run=^TestScanCompactNPMLockfile$")
	cmd.Env = append(os.Environ(), helperEnv+"="+root, "PATH=")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	var report brief.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parsing scan output: %v\n%s", err, out)
	}
	want := map[string]brief.DepInfo{
		"eslint": {
			Manifest: "package-lock.json", Name: "eslint", Version: "9.0.0",
			PURL: "pkg:npm/eslint@9.0.0", Scope: brief.ScopeDevelopment, Direct: true,
		},
		"@eslint/js": {
			Manifest: "package-lock.json", Name: "@eslint/js", Version: "9.0.0",
			PURL: "pkg:npm/%40eslint/js@9.0.0", Scope: brief.ScopeDevelopment,
		},
	}
	if len(report.Dependencies) != len(want) {
		t.Fatalf("dependencies = %+v, want %+v", report.Dependencies, want)
	}
	for _, dep := range report.Dependencies {
		if !reflect.DeepEqual(dep, want[dep.Name]) {
			t.Errorf("dependency = %+v, want %+v", dep, want[dep.Name])
		}
	}
	if !reportHasTool(&report, "lint", "ESLint") {
		t.Error("compact lockfile did not trigger ESLint detection")
	}
}
