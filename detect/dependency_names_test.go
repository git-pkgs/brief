package detect

import (
	"testing"
	"testing/fstest"
)

func TestPyPIDependencyNamesThroughScan(t *testing.T) {
	for _, tt := range []struct{ name, tool, category string }{
		{"FLask", "Flask", "build"},
		{"MYST_PARSER", "MyST-Parser", "docs"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewFS(loadKB(t), fstest.MapFS{"requirements.txt": {Data: []byte(tt.name + "\n")}})
			report, err := engine.Run()
			if err != nil {
				t.Fatal(err)
			}
			assertToolDetected(t, report, tt.category, tt.tool)
			if len(report.Dependencies) != 1 || report.Dependencies[0].Name != tt.name {
				t.Fatalf("dependency spelling changed: %+v", report.Dependencies)
			}
		})
	}
}

func TestPyPIDependencyNamesRespectDevelopmentScope(t *testing.T) {
	for _, tt := range []struct {
		name, content string
		want          bool
	}{
		{"runtime", "[project]\nname = 'example'\ndependencies = ['PyTest']\n", false},
		{"development", "[tool.poetry.group.test.dependencies]\nPyTest = '*'\n", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base := loadKB(t)
			base.ByName["pytest"].Detect.Dependencies = nil
			engine := NewFS(base, fstest.MapFS{"pyproject.toml": {Data: []byte(tt.content)}})
			report, err := engine.Run()
			if err != nil {
				t.Fatal(err)
			}
			if tt.want {
				assertToolDetected(t, report, "test", "pytest")
			} else {
				assertToolNotDetected(t, report, "test", "pytest")
			}
		})
	}
}

func TestNonPyPIDependencyNamesRemainCaseSensitive(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"testthat", true},
		{"TestThat", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			content := "Package: example\nVersion: 1.0.0\nSuggests: " + tt.name + "\n"
			engine := NewFS(loadKB(t), fstest.MapFS{"DESCRIPTION": {Data: []byte(content)}})
			report, err := engine.Run()
			if err != nil {
				t.Fatal(err)
			}
			if tt.want {
				assertToolDetected(t, report, "test", "testthat")
			} else {
				assertToolNotDetected(t, report, "test", "testthat")
			}
		})
	}
}

func TestPyPIDependencyNamesInChangedFiles(t *testing.T) {
	base := loadKB(t)
	engine := NewFS(base, fstest.MapFS{
		"requirements.txt": {Data: []byte("MYST_PARSER\n")},
		"app.py":           {Data: []byte("print('example')\n")},
	})
	report, err := engine.Run()
	if err != nil {
		t.Fatal(err)
	}
	assertToolDetected(t, FilterByChangedFiles(report, base, []string{"requirements.txt"}), "docs", "MyST-Parser")
	assertToolNotDetected(t, FilterByChangedFiles(report, base, []string{"app.py"}), "docs", "MyST-Parser")
}

func TestPyPIDevelopmentDeclarationMatchesLockfileSpelling(t *testing.T) {
	base := loadKB(t)
	base.ByName["pytest"].Detect.Dependencies = nil
	engine := NewFS(base, fstest.MapFS{
		"pyproject.toml": {Data: []byte("[tool.poetry.group.test.dependencies]\nPyTest = '*'\n")},
		"poetry.lock":    {Data: []byte("[[package]]\nname = 'pytest'\nversion = '8.0.0'\ncategory = 'main'\n")},
	})
	report, err := engine.Run()
	if err != nil {
		t.Fatal(err)
	}
	assertToolDetected(t, report, "test", "pytest")
	assertToolDetected(t, FilterByChangedFiles(report, base, []string{"poetry.lock"}), "test", "pytest")
}
