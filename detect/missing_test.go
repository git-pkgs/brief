package detect

import (
	"testing"
)

func TestMissingPythonProject(t *testing.T) {
	engine := New(loadKB(t), "../testdata/python-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mr := engine.Missing(r)

	if len(mr.Ecosystems) == 0 {
		t.Fatal("expected at least one ecosystem")
	}

	foundPython := false
	for _, eco := range mr.Ecosystems {
		if eco == "python" {
			foundPython = true
		}
	}
	if !foundPython {
		t.Error("expected python ecosystem")
	}

	// Python project has pytest, ruff, mypy detected.
	// Should be missing: format, docs.
	// Should NOT be missing: test, lint, typecheck.
	missingCats := make(map[string]bool)
	for _, m := range mr.Missing {
		missingCats[m.Category] = true
	}

	if missingCats["test"] {
		t.Error("test should not be missing (pytest detected)")
	}
	if missingCats["lint"] {
		t.Error("lint should not be missing (ruff detected)")
	}
	if missingCats["typecheck"] {
		t.Error("typecheck should not be missing (mypy detected)")
	}
	if !missingCats["docs"] {
		t.Error("docs should be missing")
	}
}

func TestMissingNodeProject(t *testing.T) {
	engine := New(loadKB(t), "../testdata/node-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mr := engine.Missing(r)

	missingCats := make(map[string]bool)
	for _, m := range mr.Missing {
		missingCats[m.Category] = true
	}

	// Node project has Jest and ESLint detected.
	if missingCats["test"] {
		t.Error("test should not be missing (jest detected)")
	}
	if missingCats["lint"] {
		t.Error("lint should not be missing (eslint detected)")
	}
	if !missingCats["format"] {
		t.Error("format should be missing")
	}
	if !missingCats["docs"] {
		t.Error("docs should be missing")
	}
}

func TestMissingEmptyProject(t *testing.T) {
	engine := New(loadKB(t), "../testdata/empty-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mr := engine.Missing(r)

	// No ecosystems detected, so nothing should be missing.
	if len(mr.Missing) != 0 {
		t.Errorf("expected no missing categories for empty project, got %d", len(mr.Missing))
	}
}

func TestMissingSuggestsDefaultTool(t *testing.T) {
	engine := New(loadKB(t), "../testdata/node-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mr := engine.Missing(r)

	for _, m := range mr.Missing {
		if m.Category == "format" {
			if m.Suggested != "Prettier" {
				t.Errorf("expected Prettier as default format suggestion, got %s", m.Suggested)
			}
			if m.SuggestedCmd == "" {
				t.Error("expected suggested command for format")
			}
			return
		}
	}
	t.Error("expected format to be in missing categories")
}

func TestMissingGoProject(t *testing.T) {
	engine := New(loadKB(t), "../testdata/go-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mr := engine.Missing(r)

	missingCats := make(map[string]bool)
	for _, m := range mr.Missing {
		missingCats[m.Category] = true
	}

	// Go project has go test and golangci-lint detected.
	if missingCats["test"] {
		t.Error("test should not be missing")
	}
	if missingCats["lint"] {
		t.Error("lint should not be missing")
	}
	// Go has no typecheck tools in KB, so it shouldn't appear.
	if missingCats["typecheck"] {
		t.Error("typecheck should not be missing (no Go typecheck tools in KB)")
	}
	// Go docs (pkgsite) detects on go.mod, so it's always present.
	if missingCats["docs"] {
		t.Error("docs should not be missing (pkgsite is built-in)")
	}
}
