package detect

import (
	"slices"
	"testing"
)

func TestNotebookToolConfigurations(t *testing.T) {
	cases := []struct {
		path, content, category, name string
	}{
		{"jupytext.toml", `formats = "ipynb,py:percent"`, categoryFormat, "Jupytext"},
		{"pyproject.toml", "[tool.jupytext]\nformats = \"ipynb,py:percent\"\n", categoryFormat, "Jupytext"},
		{".gitattributes", "*.ipynb filter=nbstripout\n", categoryFormat, "nbstripout"},
		{".pre-commit-config.yaml", "repos:\n  - repo: https://github.com/kynan/nbstripout\n    hooks:\n      - id: nbstripout\n", categoryFormat, "nbstripout"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectFile(t, dir, tc.path, tc.content)
			assertToolDetected(t, runOn(t, dir), tc.category, tc.name)
		})
	}
}

func TestJupytextConfigurationFilenames(t *testing.T) {
	cases := []struct{ path, content string }{
		{"jupytext", `formats = "ipynb,py:percent"`},
		{"jupytext.toml", `formats = "ipynb,py:percent"`},
		{"jupytext.yml", "formats: ipynb,py:percent\n"},
		{"jupytext.yaml", "formats: ipynb,py:percent\n"},
		{"jupytext.json", `{"formats":"ipynb,py:percent"}`},
		{".jupytext", `formats = "ipynb,py:percent"`},
		{".jupytext.toml", `formats = "ipynb,py:percent"`},
		{".jupytext.yml", "formats: ipynb,py:percent\n"},
		{".jupytext.yaml", "formats: ipynb,py:percent\n"},
		{".jupytext.json", `{"formats":"ipynb,py:percent"}`},
		{".jupytext.py", "c = get_config()\nc.JupytextConfiguration.formats = 'ipynb,py:percent'\n"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectFile(t, dir, tc.path, tc.content)
			report := runOn(t, dir)
			assertToolDetected(t, report, categoryFormat, "Jupytext")
			for _, tool := range report.Tools[categoryFormat] {
				if tool.Name == "Jupytext" && !slices.Equal(tool.ConfigFiles, []string{tc.path}) {
					t.Fatalf("configuration path missing or duplicated: %+v", tool.ConfigFiles)
				}
			}
		})
	}
}

func TestNotebookToolDependenciesAndReadmeMentions(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "requirements.txt", "jupytext\nnbstripout\npapermill\n")
	report := runOn(t, dir)
	assertToolDetected(t, report, categoryFormat, "Jupytext")
	assertToolDetected(t, report, categoryFormat, "nbstripout")
	assertToolDetected(t, report, categoryBuild, "Papermill")
	other := t.TempDir()
	writeProjectFile(t, other, "README.md", "Jupytext nbstripout papermill\n")
	report = runOn(t, other)
	for _, category := range []string{categoryFormat, categoryBuild} {
		for _, tool := range report.Tools[category] {
			if tool.Name == "Jupytext" || tool.Name == "nbstripout" || tool.Name == "Papermill" {
				t.Fatalf("README mention triggered notebook tooling: %+v", tool)
			}
		}
	}
}
