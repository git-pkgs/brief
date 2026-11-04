package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/git-pkgs/brief"
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

func TestRunToolsLinksChangedProjectScripts(t *testing.T) {
	knowledge := toolScanKnowledgeBase()
	project := fstest.MapFS{
		"builder.yml":  {Data: []byte("build: true\n")},
		"package.json": {Data: []byte(`{"scripts":{"build":"builder --release"}}`)},
	}

	scan, err := NewFS(knowledge, project).RunTools(ToolScanOptions{ChangedFiles: []string{"package.json"}})
	if err != nil {
		t.Fatal(err)
	}
	if !scanCheckedTool(scan, "build", "Builder") {
		t.Fatalf("checked tools: %+v", scan.Checked)
	}
	detections := scan.Tools["build"]
	if len(detections) != 1 {
		t.Fatalf("build tools: %+v", detections)
	}
	command := detections[0].Command
	if command == nil || command.Run != "builder --release" || command.Source != brief.SourceProjectScript {
		t.Fatalf("build command: %+v", command)
	}
}

func TestRunToolsLinksScriptsAgainstFullCategory(t *testing.T) {
	knowledge := toolScanKnowledgeBase()
	project := fstest.MapFS{
		"builder.yml":       {Data: []byte("build: true\n")},
		"other-builder.yml": {Data: []byte("build: true\n")},
		"package.json":      {Data: []byte(`{"scripts":{"build":"builder --release"}}`)},
	}

	scan, err := NewFS(knowledge, project).RunTools(ToolScanOptions{ChangedFiles: []string{"other-builder.yml"}})
	if err != nil {
		t.Fatal(err)
	}
	if !scanCheckedTool(scan, "build", "Builder") || !scanCheckedTool(scan, "build", "Other Builder") {
		t.Fatalf("checked tools: %+v", scan.Checked)
	}
	detections := scan.Tools["build"]
	if len(detections) != 2 {
		t.Fatalf("build tools: %+v", detections)
	}
	if command := detections[0].Command; command == nil || command.Source != brief.SourceProjectScript {
		t.Fatalf("first build command: %+v", command)
	}
	if command := detections[1].Command; command == nil || command.Source != brief.SourceKnowledgeBase {
		t.Fatalf("second build command: %+v", command)
	}

	delete(project, "builder.yml")
	scan, err = NewFS(knowledge, project).RunTools(ToolScanOptions{ChangedFiles: []string{"builder.yml"}})
	if err != nil {
		t.Fatal(err)
	}
	detections = scan.Tools["build"]
	if len(detections) != 1 || detections[0].Name != "Other Builder" {
		t.Fatalf("build tools after removal: %+v", detections)
	}
	if command := detections[0].Command; command == nil || command.Source != brief.SourceProjectScript {
		t.Fatalf("remaining build command: %+v", command)
	}
}

func TestRunToolsTrackedOnlyIgnoresUntrackedScripts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	writeToolScanFile(t, dir, "builder.yml", "build: true\n")
	runToolScanGit(t, dir, "init", "-q")
	runToolScanGit(t, dir, "add", "builder.yml")
	writeToolScanFile(t, dir, "package.json", `{"scripts":{"build":"builder --release"}}`)

	engine := New(toolScanKnowledgeBase(), dir)
	engine.TrackedOnly = true
	scan, err := engine.RunTools(ToolScanOptions{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	detections := scan.Tools["build"]
	if len(detections) != 1 {
		t.Fatalf("build tools: %+v", detections)
	}
	if command := detections[0].Command; command == nil || command.Run != "builder" || command.Source != brief.SourceKnowledgeBase {
		t.Fatalf("build command: %+v", command)
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
	builder := &kb.ToolDef{
		Tool:     kb.ToolInfo{Name: "Builder", Category: "build"},
		Detect:   kb.DetectInfo{Files: []string{"builder.yml"}},
		Commands: kb.CommandInfo{Run: "builder"},
	}
	otherBuilder := &kb.ToolDef{
		Tool:     kb.ToolInfo{Name: "Other Builder", Category: "build"},
		Detect:   kb.DetectInfo{Files: []string{"other-builder.yml"}},
		Commands: kb.CommandInfo{Run: "other-builder"},
	}
	return &kb.KnowledgeBase{
		Tools:         []*kb.ToolDef{language, ci, security, packageManager, builder, otherBuilder},
		ByName:        map[string]*kb.ToolDef{"Go": language, "GitHub Actions": ci, "gosec": security, "npm": packageManager, "Builder": builder, "Other Builder": otherBuilder},
		ByCategory:    map[string][]*kb.ToolDef{"language": {language}, "ci": {ci}, "security": {security}, "package_manager": {packageManager}, "build": {builder, otherBuilder}},
		Ecosystems:    map[string][]*kb.ToolDef{"go": {language, security}},
		ManifestFiles: []string{"go.mod"},
		ScriptSources: []kb.ScriptSourceDef{{Source: kb.SourceInfo{Name: "package.json", File: "package.json", Format: "json_scripts"}}},
	}
}

func writeToolScanFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runToolScanGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
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

func scanCheckedTool(scan *ToolScan, category, name string) bool {
	for _, tool := range scan.Checked {
		if tool.Category == category && tool.Name == name {
			return true
		}
	}
	return false
}
