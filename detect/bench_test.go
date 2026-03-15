package detect

import (
	"testing"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/kb"
)

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
