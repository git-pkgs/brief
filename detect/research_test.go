package detect

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/git-pkgs/brief"
)

func TestResearchToolDetectors(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"analysis/notebook.ipynb":  "{}\n",
		"report.qmd":               "---\ntitle: Analysis\n---\n",
		"workflow/Snakefile":       "rule all:\n    input: []\n",
		"nextflow.config":          "manifest { name = 'research-pipeline' }\n",
		"main.nf":                  "workflow { }\n",
		"pyproject.toml":           "[project]\nname = \"research-tools\"\nversion = \"0.1.0\"\n",
		"tox.ini":                  "[tox]\nenv_list = py312\n",
		"benchmarks/asv.conf.json": `{"repo": "."}`,
		"DESCRIPTION": `Package: researchtools
Version: 0.1.0
Suggests:
    covr,
    knitr,
    pkgdown,
    rmarkdown,
    testthat
Config/roxygen2/version: 7.3.3
Config/testthat/edition: 3
VignetteBuilder: knitr
`,
		"README.Rmd":                   "---\ntitle: Research tools\n---\n",
		"_pkgdown.yml":                 "url: https://example.com\n",
		"renv.lock":                    `{"R":{"Version":"4.5.0"}}`,
		"Project.toml":                 "name = \"ResearchTools\"\nuuid = \"11111111-1111-1111-1111-111111111111\"\n",
		"src/ResearchTools.jl":         "module ResearchTools\nend\n",
		"docs/Project.toml":            "[deps]\nDocumenter = \"e30172f5-a6a5-5a46-863b-614d45cd2de4\"\n",
		"docs/make.jl":                 "using Documenter\n",
		".JuliaFormatter.toml":         "margin = 92\n",
		".github/workflows/format.yml": "name: format\non: push\njobs:\n  format:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: fredrikekre/runic-action@v1\n",
		"model.f90":                    "program model\nend program model\n",
		"fortitude.toml":               "line-length = 100\n",
	}
	for path, content := range files {
		writeProjectFile(t, dir, path, content)
	}

	report := runOn(t, dir)
	want := []struct {
		category string
		name     string
	}{
		{category: "environment", name: "Jupyter"},
		{category: "docs", name: "Quarto"},
		{category: "build", name: "Snakemake"},
		{category: "build", name: "Nextflow"},
		{category: "test", name: "tox"},
		{category: "test", name: "ASV"},
		{category: "test", name: "testthat"},
		{category: "coverage", name: "covr"},
		{category: "docs", name: "pkgdown"},
		{category: "docs", name: "roxygen2"},
		{category: "docs", name: "knitr"},
		{category: "docs", name: "R Markdown"},
		{category: "docs", name: "Documenter.jl"},
		{category: "format", name: "JuliaFormatter"},
		{category: "format", name: "Runic"},
		{category: "lint", name: "Fortitude"},
	}
	for _, item := range want {
		assertToolDetected(t, report, item.category, item.name)
	}
	if !slices.ContainsFunc(report.PackageManagers, func(d brief.Detection) bool {
		return d.Name == "renv"
	}) {
		t.Error("expected renv package manager")
	}
}

func TestSnakemakeSmkConfigFile(t *testing.T) {
	dir := t.TempDir()
	const path = "workflow/rules/common.smk"
	const configPattern = "**/*.smk"
	writeProjectFile(t, dir, path, "rule all:\n    input: []\n")

	report := runOn(t, dir)
	for _, tool := range report.Tools["build"] {
		if tool.Name != "Snakemake" {
			continue
		}
		if !slices.Contains(tool.ConfigFiles, configPattern) {
			t.Errorf("Snakemake config files = %v, want %q", tool.ConfigFiles, configPattern)
		}
		return
	}
	t.Fatal("expected Snakemake in build category")
}

func TestSnakemakeWorkflowFilenames(t *testing.T) {
	for _, tt := range []struct{ path, pattern string }{
		{"Snakefile", "Snakefile"},
		{"snakefile", "snakefile"},
		{"workflow/Snakefile", "**/Snakefile"},
		{"workflow/snakefile", "**/snakefile"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			engine := NewFS(loadKB(t), fstest.MapFS{tt.path: {Data: []byte("rule all:\n    input: []\n")}})
			report, err := engine.Run()
			if err != nil {
				t.Fatal(err)
			}
			assertToolDetected(t, report, "build", "Snakemake")
			for _, tool := range report.Tools["build"] {
				if tool.Name == "Snakemake" && !slices.Contains(tool.ConfigFiles, tt.pattern) {
					t.Errorf("config files = %v, want %q", tool.ConfigFiles, tt.pattern)
				}
			}
		})
	}
}

func TestLowercaseSnakemakeProject(t *testing.T) {
	assertToolDetected(t, runOn(t, "../testdata/snakemake-project"), "build", "Snakemake")
}
