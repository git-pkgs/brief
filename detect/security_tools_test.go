package detect

import (
	"testing"

	"github.com/git-pkgs/brief"
)

func TestSecurityToolEvidence(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"Cargo.toml":                     "[package]\nname = \"security-tools\"\nversion = \"0.1.0\"\n",
		"deny.toml":                      "[advisories]\n",
		".github/zizmor.yml":             "rules: {}\n",
		".syft.yaml":                     "output: [syft-json]\n",
		".gitleaks.toml":                 "title = \"example\"\n",
		".secrets.baseline":              "{\"version\": \"1.5.0\", \"results\": {}}\n",
		".secretlintrc.json":             "{\"rules\": []}\n",
		".checkov.yaml":                  "quiet: true\n",
		".gitguardian.yaml":              "version: 2\n",
		"requirements.txt":               "detect-secrets==1.5.0\ncheckov==3.2.0\nggshield==1.43.0\n",
		"package.json":                   "{\"devDependencies\":{\"secretlint\":\"^10.0.0\"}}\n",
		".github/workflows/security.yml": securityActionsWorkflow,
	}
	for path, content := range files {
		writeProjectFile(t, dir, path, content)
	}

	report := runOn(t, dir)
	for _, name := range []string{
		"zizmor",
		"Syft",
		"Gitleaks",
		"detect-secrets",
		"Secretlint",
		"Checkov",
		"ggshield",
		"Dependency Review Action",
		"Harden-Runner",
		"OpenSSF Scorecard",
		"CodeQL",
		"Grype",
		"cargo-deny",
		"Trivy",
		"TruffleHog",
		"Snyk",
	} {
		assertHighConfidenceToolDetected(t, report, "security", name)
	}

	for name, path := range map[string]string{
		"zizmor":         ".github/zizmor.yml",
		"Syft":           ".syft.yaml",
		"Gitleaks":       ".gitleaks.toml",
		"detect-secrets": ".secrets.baseline",
		"Secretlint":     ".secretlintrc.json",
		"Checkov":        ".checkov.yaml",
		"ggshield":       ".gitguardian.yaml",
		"cargo-deny":     "deny.toml",
	} {
		assertToolConfigFile(t, report, name, path)
	}
}

func TestSecurityToolConfigIsMediumConfidence(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, ".gitleaks.toml", "title = \"example\"\n")

	report := runOn(t, dir)
	for _, tool := range report.Tools["security"] {
		if tool.Name != "Gitleaks" {
			continue
		}
		if tool.Confidence != brief.ConfidenceMedium {
			t.Fatalf("Gitleaks confidence = %q, want %q", tool.Confidence, brief.ConfidenceMedium)
		}
		return
	}
	t.Fatal("expected Gitleaks in security category")
}

func assertToolConfigFile(t *testing.T, report *brief.Report, name, path string) {
	t.Helper()
	for _, tool := range report.Tools["security"] {
		if tool.Name != name {
			continue
		}
		for _, config := range tool.ConfigFiles {
			if config == path {
				return
			}
		}
		t.Fatalf("%s config files = %v, want %q", name, tool.ConfigFiles, path)
	}
	t.Fatalf("expected %s in security category", name)
}

const securityActionsWorkflow = `name: security
on: [push]
jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: zizmorcore/zizmor-action@v0
      - uses: anchore/sbom-action/download-syft@v0
      - uses: EmbarkStudios/cargo-deny-action@v2
      - uses: gitleaks/gitleaks-action@v2
      - uses: bridgecrewio/checkov-action@v12
      - uses: GitGuardian/ggshield-action@v1
      - uses: actions/dependency-review-action@v4
      - uses: step-security/harden-runner@v2
      - uses: ossf/scorecard-action@v2
      - uses: github/codeql-action/init@v3
      - uses: anchore/scan-action/download-grype@v6
      - uses: aquasecurity/trivy-action@v0
      - uses: trufflesecurity/trufflehog@main
      - uses: snyk/actions/node@master
`
