package detect

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/git-pkgs/brief"
)

const (
	apmManifest = "name: agent-workspace\nversion: 1.0.0\ndependencies:\n  apm:\n    - microsoft/apm-sample-package#v1.0.0\n"
	apmLockfile = "lockfile_version: \"2\"\ndependencies:\n  - repo_url: microsoft/apm-sample-package\n    resolved_commit: a1b2c3d4e5f6789012345678901234567890abcd\n    resolved_ref: v1.0.0\n    depth: 1\n"
)

func TestAPMDetection(t *testing.T) {
	for _, tc := range []struct{ file, body string }{
		{"apm.yml", apmManifest},
		{"apm.lock.yaml", apmLockfile},
		{"apm.lock", apmLockfile},
	} {
		t.Run(tc.file, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectFile(t, dir, tc.file, tc.body)
			r := runOn(t, dir)
			if !slices.Contains(packageManagerNames(r), "APM") {
				t.Errorf("package managers = %v, want APM", packageManagerNames(r))
			}
			if !slices.ContainsFunc(r.Manifests, func(m brief.ManifestInfo) bool {
				return m.Path == tc.file && m.Ecosystem == "apm"
			}) {
				t.Errorf("manifests = %+v, want %s", r.Manifests, tc.file)
			}
			if !slices.ContainsFunc(r.Dependencies, func(d brief.DepInfo) bool {
				return d.Manifest == tc.file && d.Name == "github.com/microsoft/apm-sample-package"
			}) {
				t.Errorf("dependencies = %+v", r.Dependencies)
			}
		})
	}
}

func TestAPMAlongsideOtherPackageManager(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "package.json", `{"name":"x","dependencies":{"lodash":"^4.17.21"}}`)
	writeProjectFile(t, dir, "package-lock.json", `{"name":"x","lockfileVersion":3,"packages":{}}`)
	writeProjectFile(t, dir, "index.js", "")
	writeProjectFile(t, dir, "apm.yml", apmManifest)
	names := packageManagerNames(runOn(t, dir))
	if !slices.Contains(names, "APM") || !slices.Contains(names, "npm") {
		t.Errorf("package managers = %v, want APM and npm", names)
	}
}

func TestAPMIgnoresInstalledModules(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "apm.yml", apmManifest)
	writeProjectFile(t, dir, "apm_modules/microsoft/apm-sample-package/apm.yml", "name: sample\nversion: 1.0.0\ndependencies:\n  apm:\n    - other/transitive\n")
	writeProjectFile(t, dir, "apm_modules/microsoft/apm-sample-package/scripts/check.py", "print('hi')\n")
	engine := New(loadKB(t), dir)
	r, err := engine.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(engine.projectFiles, []string{"apm.yml"}) {
		t.Errorf("project files = %v, want only apm.yml", engine.projectFiles)
	}
	for _, m := range r.Manifests {
		if m.Path != "apm.yml" {
			t.Errorf("unexpected manifest %+v", m)
		}
	}
	for _, d := range r.Dependencies {
		if strings.HasSuffix(d.Name, "other/transitive") {
			t.Errorf("dependency from installed module: %+v", d)
		}
	}
}

func TestAPMChangedFiles(t *testing.T) {
	knowledge := loadKB(t)
	project := fstest.MapFS{"apm.yml": {Data: []byte(apmManifest)}}
	for _, changed := range []string{"apm.yml", "apm.lock.yaml", "apm.lock"} {
		scan, err := NewFS(knowledge, project).RunTools(ToolScanOptions{ChangedFiles: []string{changed}})
		if err != nil {
			t.Fatal(err)
		}
		if !scanCheckedTool(scan, "package_manager", "APM") {
			t.Errorf("%s: APM not rechecked: %+v", changed, scan.Checked)
		}
		if changed == "apm.yml" && !scanHasTool(scan, "package_manager", "APM") {
			t.Errorf("%s: APM not detected: %+v", changed, scan.Tools)
		}
	}

	dir := t.TempDir()
	writeProjectFile(t, dir, "apm.yml", apmManifest)
	filtered := FilterByChangedFiles(runOn(t, dir), knowledge, []string{"apm.yml"})
	if !slices.Contains(packageManagerNames(filtered), "APM") || len(filtered.Manifests) == 0 {
		t.Errorf("filtered = %v, %+v", packageManagerNames(filtered), filtered.Manifests)
	}
}
