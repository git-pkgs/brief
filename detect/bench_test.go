package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/kb"
)

func BenchmarkCitationProject(b *testing.B) {
	b.Setenv("PATH", "")
	knowledgeBase := loadKBForBench(b)
	data, err := os.ReadFile("../testdata/citation-project/CITATION.cff")
	if err != nil {
		b.Fatal(err)
	}
	for _, name := range []string{"absent", "present"} {
		b.Run(name, func(b *testing.B) {
			root := b.TempDir()
			if name == "present" {
				if err := os.WriteFile(filepath.Join(root, "CITATION.cff"), data, 0o600); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				r, err := New(knowledgeBase, root).Run()
				if err != nil {
					b.Fatal(err)
				}
				if name == "present" && (r.Resources == nil || r.Resources.Citation == nil || r.Resources.Citation.ValidationStatus != "valid") {
					b.Fatal("missing citation")
				}
			}
		})
	}
}

func loadKBForBench(b *testing.B) *kb.KnowledgeBase {
	b.Helper()
	knowledgeBase, err := kb.Load(brief.KnowledgeFS)
	if err != nil {
		b.Fatalf("loading knowledge base: %v", err)
	}
	return knowledgeBase
}

func BenchmarkKBLoad(b *testing.B) {
	for b.Loop() {
		_, err := kb.Load(brief.KnowledgeFS)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEmptyProject(b *testing.B) {
	knowledgeBase := loadKBForBench(b)
	b.ResetTimer()
	for b.Loop() {
		engine := New(knowledgeBase, "../testdata/empty-project")
		_, err := engine.Run()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRubyProject(b *testing.B) {
	knowledgeBase := loadKBForBench(b)
	b.ResetTimer()
	for b.Loop() {
		engine := New(knowledgeBase, "../testdata/ruby-project")
		_, err := engine.Run()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGoProject(b *testing.B) {
	knowledgeBase := loadKBForBench(b)
	b.ResetTimer()
	for b.Loop() {
		engine := New(knowledgeBase, "../testdata/go-project")
		_, err := engine.Run()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNodeProject(b *testing.B) {
	knowledgeBase := loadKBForBench(b)
	b.ResetTimer()
	for b.Loop() {
		engine := New(knowledgeBase, "../testdata/node-project")
		_, err := engine.Run()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPythonProject(b *testing.B) {
	knowledgeBase := loadKBForBench(b)
	b.ResetTimer()
	for b.Loop() {
		engine := New(knowledgeBase, "../testdata/python-project")
		_, err := engine.Run()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSelfDetect(b *testing.B) {
	knowledgeBase := loadKBForBench(b)
	b.ResetTimer()
	for b.Loop() {
		engine := New(knowledgeBase, "..")
		_, err := engine.Run()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNestedCargoProject(b *testing.B) {
	knowledgeBase := loadKBForBench(b)
	b.ResetTimer()
	for b.Loop() {
		engine := New(knowledgeBase, "../testdata/react-compiler-project")
		_, err := engine.Run()
		if err != nil {
			b.Fatal(err)
		}
	}
}
