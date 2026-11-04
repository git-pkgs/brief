package detect

import (
	"testing"
	"testing/fstest"

	"github.com/git-pkgs/brief/kb"
)

func TestRunToolsUsesKnownLanguagesAndChangedFiles(t *testing.T) {
	knowledge := toolScanKnowledgeBase()
	project := fstest.MapFS{
		".github/workflows/ci.yml": {Data: []byte("jobs: {}\n")},
		".gosec.json":              {Data: []byte("{}\n")},
	}

	full, err := NewFS(knowledge, project).RunTools(ToolScanOptions{Full: true, Languages: []string{"go"}})
	if err != nil {
		t.Fatal(err)
	}
	if !scanHasTool(full, "ci", "GitHub Actions") || !scanHasTool(full, "security", "gosec") {
		t.Fatalf("full scan tools: %+v", full.Tools)
	}

	partial, err := NewFS(knowledge, project).RunTools(ToolScanOptions{
		ChangedFiles: []string{".github/workflows/ci.yml"},
		Languages:    []string{"go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(partial.Checked) != 1 || partial.Checked[0].Name != "GitHub Actions" {
		t.Fatalf("checked tools: %+v", partial.Checked)
	}
	if !scanHasTool(partial, "ci", "GitHub Actions") {
		t.Fatalf("partial scan tools: %+v", partial.Tools)
	}

	removed, err := NewFS(knowledge, project).RunTools(ToolScanOptions{
		ChangedFiles:     []string{"main.go"},
		LanguagesChanged: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Checked) != 1 || removed.Checked[0].Name != "gosec" {
		t.Fatalf("language-affected tools: %+v", removed.Checked)
	}
	if scanHasTool(removed, "security", "gosec") {
		t.Fatalf("tool matched without its ecosystem: %+v", removed.Tools)
	}

	manifest, err := NewFS(knowledge, project).RunTools(ToolScanOptions{ChangedFiles: []string{"go.mod"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Checked) != 1 || manifest.Checked[0].Name != "npm" {
		t.Fatalf("manifest-affected tools: %+v", manifest.Checked)
	}
}

func TestEcosystemsForLanguageAliases(t *testing.T) {
	c := &kb.ToolDef{Tool: kb.ToolInfo{Name: "C", Category: "language"}, Detect: kb.DetectInfo{Ecosystems: []string{"c"}}}
	cpp := &kb.ToolDef{Tool: kb.ToolInfo{Name: "C++", Category: "language"}, Detect: kb.DetectInfo{Ecosystems: []string{"cpp"}}}
	objectiveC := &kb.ToolDef{Tool: kb.ToolInfo{Name: "Objective-C", Category: "language"}, Detect: kb.DetectInfo{Ecosystems: []string{"objective-c"}}}
	knowledge := &kb.KnowledgeBase{ByCategory: map[string][]*kb.ToolDef{"language": {c, cpp, objectiveC}}}

	got := ecosystemsForLanguages(knowledge, []string{"c", "c++", "objectivec"})
	for _, ecosystem := range []string{"c", "cpp", "objective-c"} {
		if !got[ecosystem] {
			t.Errorf("missing ecosystem %q: %v", ecosystem, got)
		}
	}
}

func toolScanKnowledgeBase() *kb.KnowledgeBase {
	language := &kb.ToolDef{
		Tool:   kb.ToolInfo{Name: "Go", Category: "language"},
		Detect: kb.DetectInfo{Files: []string{"*.go"}, Ecosystems: []string{"go"}},
	}
	ci := &kb.ToolDef{
		Tool:   kb.ToolInfo{Name: "GitHub Actions", Category: "ci"},
		Detect: kb.DetectInfo{Files: []string{".github/workflows/"}},
		Config: kb.ConfigInfo{Files: []string{".github/workflows/"}},
	}
	security := &kb.ToolDef{
		Tool:   kb.ToolInfo{Name: "gosec", Category: "security"},
		Detect: kb.DetectInfo{Files: []string{".gosec.json"}, Ecosystems: []string{"go"}},
		Config: kb.ConfigInfo{Files: []string{".gosec.json"}},
	}
	packageManager := &kb.ToolDef{
		Tool:   kb.ToolInfo{Name: "npm", Category: "package_manager"},
		Detect: kb.DetectInfo{Files: []string{"package.json"}},
	}
	return &kb.KnowledgeBase{
		Tools:         []*kb.ToolDef{language, ci, security, packageManager},
		ByName:        map[string]*kb.ToolDef{"Go": language, "GitHub Actions": ci, "gosec": security, "npm": packageManager},
		ByCategory:    map[string][]*kb.ToolDef{"language": {language}, "ci": {ci}, "security": {security}, "package_manager": {packageManager}},
		Ecosystems:    map[string][]*kb.ToolDef{"go": {language, security}},
		ManifestFiles: []string{"go.mod"},
	}
}

func scanHasTool(scan *ToolScan, category, name string) bool {
	for _, detection := range scan.Tools[category] {
		if detection.Name == name {
			return true
		}
	}
	return false
}
