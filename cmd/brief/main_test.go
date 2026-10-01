package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
)

const scanHelperRootEnv = "BRIEF_SCAN_HELPER_ROOT"
const diffHelperEnv = "BRIEF_DIFF_HELPER"
const submoduleHelperRootEnv = "BRIEF_SUBMODULE_HELPER_ROOT"
const submoduleDiffHelperEnv = "BRIEF_SUBMODULE_DIFF_HELPER"
const yamlResourceDiffHelperEnv = "BRIEF_YAML_RESOURCE_DIFF_HELPER"

func TestScanUsesPathRoles(t *testing.T) {
	const helperEnv = "BRIEF_ROLES_HELPER_ROOT"
	if root := os.Getenv(helperEnv); root != "" {
		cmdScan([]string{"-json", root})
		return
	}
	root := t.TempDir()
	writeScanFixture(t, root, "main.go", "package main\nfunc main() {\n\tprintln(1)\n}\n")
	for _, name := range []string{"evals/fixtures/a/app.py", "evals/fixtures/b/app.py", "skills/semgrep/scripts/scan.py"} {
		writeScanFixture(t, root, name, "def run():\n    print('fixture')\n")
	}
	writeScanFixture(t, root, "fixtures/deploy.yaml", "apiVersion: argoproj.io/v1alpha1\nkind: Application\n")
	cmd := exec.Command(os.Args[0], "-test.run=^TestScanUsesPathRoles$")
	cmd.Env = append(os.Environ(), helperEnv+"="+root, "PATH=")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("scan command failed: %v", err)
	}
	var report brief.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parsing scan output: %v\n%s", err, out)
	}
	if len(report.Languages) != 1 || report.Languages[0].Name != "Go" {
		t.Errorf("languages = %+v, want only Go", report.Languages)
	}
	if report.Style == nil || report.Style.Indentation != "tabs" {
		t.Errorf("style = %+v, want tabs", report.Style)
	}
	if reportHasTool(&report, "infrastructure", "Argo CD") {
		t.Error("fixture manifest triggered Argo CD")
	}
}

func TestScanDefaultsBoundRecursiveDetection(t *testing.T) {
	if root := os.Getenv(scanHelperRootEnv); root != "" {
		cmdScan([]string{"-json", root})
		return
	}

	root := t.TempDir()
	writeScanFixture(t, root, "pyproject.toml", "[project]\nname = \"example\"\nversion = \"1.0.0\"\n")
	writeScanFixture(t, root, "pipelines/example/Snakefile", "rule all:\n")
	deep := filepath.Join(
		"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "asv.conf.json",
	)
	writeScanFixture(t, root, deep, "{}\n")

	cmd := exec.Command(os.Args[0], "-test.run=^TestScanDefaultsBoundRecursiveDetection$")
	cmd.Env = append(os.Environ(), scanHelperRootEnv+"="+root, "PATH=")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("scan command failed: %v", err)
	}

	var report brief.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parsing scan output: %v\n%s", err, out)
	}
	if !reportHasTool(&report, "build", "Snakemake") {
		t.Fatal("expected nested Snakemake project within the default depth")
	}
	if reportHasTool(&report, "test", "ASV") {
		t.Fatal("did not expect ASV beyond the default depth")
	}
	if !report.Stats.ScanTruncated {
		t.Fatal("expected scan output to report the depth truncation")
	}
}

func TestDiffAppliesScanOverrides(t *testing.T) {
	if os.Getenv(diffHelperEnv) != "" {
		cmdDiff([]string{"-scan-limit=-1", "HEAD"})
		return
	}

	dir := t.TempDir()
	writeScanFixture(t, dir, "go.mod", "module example.com/project\n\ngo 1.22\n")
	writeScanFixture(t, dir, "main.go", "package main\n")
	runGitFixture(t, dir, "init", "-q")
	runGitFixture(t, dir, "add", "go.mod", "main.go")
	runGitFixture(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "initial")
	writeScanFixture(t, dir, "main.go", "package main\n\nfunc main() {}\n")

	cmd := exec.Command(os.Args[0], "-test.run=^TestDiffAppliesScanOverrides$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), diffHelperEnv+"=1", "PATH="+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("diff command succeeded with a negative scan limit\n%s", out)
	}
	if !strings.Contains(string(out), "scan limit must not be negative") {
		t.Fatalf("diff command output = %q, want scan limit validation error", out)
	}
}

func TestDiffReportsYAMLResourceWithUppercaseExtension(t *testing.T) {
	if os.Getenv(yamlResourceDiffHelperEnv) != "" {
		cmdDiff([]string{"-json", "HEAD"})
		os.Exit(0)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	dir := t.TempDir()
	initGitScanFixture(t, dir)
	writeScanFixture(t, dir, "README.md", "# Example\n")
	runGitFixture(t, dir, "add", "README.md")
	runGitFixture(t, dir, "commit", "-q", "-m", "initial")
	writeScanFixture(t, dir, "manifest.YAML", "apiVersion: argoproj.io/v1alpha1\nkind: Application\n")

	cmd := exec.Command(os.Args[0], "-test.run=^TestDiffReportsYAMLResourceWithUppercaseExtension$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), yamlResourceDiffHelperEnv+"=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("diff command failed: %v", err)
	}

	var report brief.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parsing diff output: %v\n%s", err, out)
	}
	if !reportHasTool(&report, "infrastructure", "Argo CD") {
		t.Fatalf("tools = %+v, want Argo CD from changed manifest.YAML", report.Tools)
	}
}

func TestScanIncludeSubmodulesFlag(t *testing.T) {
	if root := os.Getenv(submoduleHelperRootEnv); root != "" {
		cmdScan([]string{"-json", "-include-submodules", root})
		return
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	native := t.TempDir()
	initGitScanFixture(t, native)
	writeScanFixture(t, native, "native.c", "int native(void) { return 0; }\n")
	runGitFixture(t, native, "add", "native.c")
	runGitFixture(t, native, "commit", "-q", "-m", "add native source")

	parent := t.TempDir()
	initGitScanFixture(t, parent)
	writeScanFixture(t, parent, "main.py", "print('example')\n")
	runGitFixture(t, parent, "add", "main.py")
	runGitFixture(t, parent, "commit", "-q", "-m", "add parent source")
	runGitFixture(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", "-q", native, "vendor/native")
	runGitFixture(t, parent, "commit", "-q", "-m", "add submodule")

	cmd := exec.Command(os.Args[0], "-test.run=^TestScanIncludeSubmodulesFlag$")
	cmd.Env = append(os.Environ(), submoduleHelperRootEnv+"="+parent)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("scan command failed: %v", err)
	}

	var report brief.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parsing scan output: %v\n%s", err, out)
	}
	if !slices.ContainsFunc(report.Languages, func(language brief.Detection) bool {
		return language.Name == "C"
	}) {
		t.Errorf("languages = %+v, want C from initialized submodule", report.Languages)
	}
}

func TestDiffIncludeSubmodulesFlag(t *testing.T) {
	if os.Getenv(submoduleDiffHelperEnv) != "" {
		cmdDiff([]string{"-json", "-include-submodules", "HEAD"})
		os.Exit(0)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	native := t.TempDir()
	initGitScanFixture(t, native)
	writeScanFixture(t, native, "go.mod", "module example.com/native\n\ngo 1.22\n")
	writeScanFixture(t, native, "native.c", "int native(void) { return 0; }\n")
	runGitFixture(t, native, "add", "go.mod", "native.c")
	runGitFixture(t, native, "commit", "-q", "-m", "add native source")

	parent := t.TempDir()
	initGitScanFixture(t, parent)
	writeScanFixture(t, parent, "main.py", "print('example')\n")
	runGitFixture(t, parent, "add", "main.py")
	runGitFixture(t, parent, "commit", "-q", "-m", "add parent source")
	runGitFixture(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", "-q", native, "modules/native")
	runGitFixture(t, parent, "commit", "-q", "-m", "add submodule")

	checkout := filepath.Join(parent, "modules/native")
	writeScanFixture(t, checkout, "version.txt", "2\n")
	runGitFixture(t, checkout, "add", "version.txt")
	runGitFixture(
		t, checkout, "-c", "user.name=Test", "-c", "user.email=test@example.com",
		"commit", "-q", "-m", "update native source",
	)
	runGitFixture(t, parent, "add", "modules/native")

	cmd := exec.Command(os.Args[0], "-test.run=^TestDiffIncludeSubmodulesFlag$")
	cmd.Dir = parent
	cmd.Env = append(os.Environ(), submoduleDiffHelperEnv+"=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("diff command failed: %v", err)
	}

	var report brief.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parsing diff output: %v\n%s", err, out)
	}
	if !slices.ContainsFunc(report.Languages, func(language brief.Detection) bool {
		return language.Name == "C"
	}) {
		t.Errorf("languages = %+v, want C from changed submodule", report.Languages)
	}
	if !slices.ContainsFunc(report.Manifests, func(manifest brief.ManifestInfo) bool {
		return manifest.Path == "modules/native/go.mod"
	}) {
		t.Errorf("manifests = %+v, want modules/native/go.mod", report.Manifests)
	}
}

func initGitScanFixture(t *testing.T, dir string) {
	t.Helper()
	runGitFixture(t, dir, "init", "-q")
	runGitFixture(t, dir, "config", "user.name", "Test")
	runGitFixture(t, dir, "config", "user.email", "test@example.com")
}

func runGitFixture(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeScanFixture(t *testing.T, root, name, content string) {
	t.Helper()
	full := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func reportHasTool(report *brief.Report, category, name string) bool {
	for _, tool := range report.Tools[category] {
		if tool.Name == name {
			return true
		}
	}
	return false
}
