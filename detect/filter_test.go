package detect

import (
	"testing"

	"github.com/git-pkgs/brief"
)

func TestFilterByChangedFiles_Languages(t *testing.T) {
	knowledgeBase := loadKB(t)

	engine := New(knowledgeBase, "../testdata/go-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Filter with only .go files changed - should keep Go language.
	filtered := FilterByChangedFiles(r, knowledgeBase, []string{"main.go"})
	if len(filtered.Languages) == 0 {
		t.Fatal("expected Go language to be kept when .go file changed")
	}
	foundGo := false
	for _, l := range filtered.Languages {
		if l.Name == "Go" {
			foundGo = true
		}
	}
	if !foundGo {
		t.Error("expected Go in filtered languages")
	}

	// Filter with only .txt file changed - should not keep Go.
	filtered = FilterByChangedFiles(r, knowledgeBase, []string{"notes.txt"})
	for _, l := range filtered.Languages {
		if l.Name == "Go" {
			t.Error("did not expect Go language when only .txt changed")
		}
	}
}

func TestFilterByChangedFiles_Tools(t *testing.T) {
	knowledgeBase := loadKB(t)

	engine := New(knowledgeBase, "../testdata/go-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Filter with golangci-lint config changed.
	filtered := FilterByChangedFiles(r, knowledgeBase, []string{".golangci.yml"})
	found := false
	if tools, ok := filtered.Tools["lint"]; ok {
		for _, tool := range tools {
			if tool.Name == "golangci-lint" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected golangci-lint when its config file changed")
	}
}

func TestFilterByChangedFiles_PackageManagers(t *testing.T) {
	knowledgeBase := loadKB(t)

	engine := New(knowledgeBase, "../testdata/go-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// go.mod is both a config file and a manifest.
	filtered := FilterByChangedFiles(r, knowledgeBase, []string{"go.mod"})
	foundGoMod := false
	for _, pm := range filtered.PackageManagers {
		if pm.Name == "Go Modules" {
			foundGoMod = true
		}
	}
	if !foundGoMod {
		t.Error("expected Go Modules when go.mod changed")
	}
}

func TestFilterByChangedFiles_NoRelevantChanges(t *testing.T) {
	knowledgeBase := loadKB(t)

	engine := New(knowledgeBase, "../testdata/go-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Filter with a completely unrelated file.
	filtered := FilterByChangedFiles(r, knowledgeBase, []string{"random.xyz"})
	if len(filtered.Languages) != 0 {
		t.Errorf("expected no languages, got %d", len(filtered.Languages))
	}
	if len(filtered.PackageManagers) != 0 {
		t.Errorf("expected no package managers, got %d", len(filtered.PackageManagers))
	}
	total := 0
	for _, tools := range filtered.Tools {
		total += len(tools)
	}
	if total != 0 {
		t.Errorf("expected no tools, got %d", total)
	}
}

func TestFilterByChangedFiles_Scripts(t *testing.T) {
	knowledgeBase := loadKB(t)

	engine := New(knowledgeBase, "../testdata/ruby-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Makefile changed - should keep Makefile scripts.
	filtered := FilterByChangedFiles(r, knowledgeBase, []string{"Makefile"})
	if len(filtered.Scripts) == 0 {
		t.Error("expected scripts when Makefile changed")
	}

	// Unrelated file - should not keep scripts.
	filtered = FilterByChangedFiles(r, knowledgeBase, []string{"random.xyz"})
	if len(filtered.Scripts) != 0 {
		t.Errorf("expected no scripts, got %d", len(filtered.Scripts))
	}
}

func TestFilterByChangedFiles_PreservesMetadata(t *testing.T) {
	knowledgeBase := loadKB(t)

	r := &brief.Report{
		Version:      "test",
		Path:         "/test",
		DiffRef:      "main..HEAD",
		ChangedFiles: []string{"main.go"},
		Tools:        make(map[string][]brief.Detection),
		Git: &brief.GitInfo{
			Branch: "feature",
		},
	}

	filtered := FilterByChangedFiles(r, knowledgeBase, []string{"main.go"})
	if filtered.Version != "test" {
		t.Error("expected version to be preserved")
	}
	if filtered.DiffRef != "main..HEAD" {
		t.Error("expected diff_ref to be preserved")
	}
	if filtered.Git == nil || filtered.Git.Branch != "feature" {
		t.Error("expected git info to be preserved")
	}
}
