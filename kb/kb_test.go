package kb_test

import (
	"bufio"
	"os"
	"slices"
	"sort"
	"testing"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/kb"
)

// loadTaxonomyTerms reads the vendored oss-taxonomy term list.
// Source: https://taxonomy.ecosyste.ms/terms.txt
// Refresh with: curl -s https://taxonomy.ecosyste.ms/terms.txt -o kb/testdata/oss-taxonomy-terms.txt
func loadTaxonomyTerms(t *testing.T) map[string]bool {
	t.Helper()
	f, err := os.Open("testdata/oss-taxonomy-terms.txt")
	if err != nil {
		t.Fatalf("opening vendored terms: %v", err)
	}
	defer func() { _ = f.Close() }()

	terms := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			terms[line] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading terms: %v", err)
	}
	if len(terms) == 0 {
		t.Fatal("vendored terms file is empty")
	}
	return terms
}

func loadKB(t *testing.T) *kb.KnowledgeBase {
	t.Helper()
	base, err := kb.Load(brief.KnowledgeFS)
	if err != nil {
		t.Fatalf("loading knowledge base: %v", err)
	}
	return base
}

func TestThreatsRegistryLoads(t *testing.T) {
	base := loadKB(t)
	if len(base.Threats) == 0 {
		t.Fatal("expected threats registry to be populated from _threats.toml")
	}

	want := []string{"sql_injection", "xss", "command_injection", "deserialization", "ssrf"}
	for _, id := range want {
		def, ok := base.Threats[id]
		if !ok {
			t.Errorf("expected threat %q in registry", id)
			continue
		}
		if def.CWE == "" {
			t.Errorf("threat %q has no CWE", id)
		}
		if def.Title == "" {
			t.Errorf("threat %q has no title", id)
		}
	}
}

func TestThreatMappingsLoad(t *testing.T) {
	base := loadKB(t)
	if len(base.ThreatMappings) == 0 {
		t.Fatal("expected threat mappings to be populated")
	}

	// Find the templating mapping and check its shape.
	var found bool
	for _, m := range base.ThreatMappings {
		if len(m.Match) == 1 && m.Match[0] == "function:templating" {
			found = true
			if !slices.Contains(m.Threats, "xss") {
				t.Errorf("templating mapping should include xss, got %v", m.Threats)
			}
			if !slices.Contains(m.Threats, "ssti") {
				t.Errorf("templating mapping should include ssti, got %v", m.Threats)
			}
		}
	}
	if !found {
		t.Error("expected function:templating mapping")
	}
}

func TestValidate(t *testing.T) {
	base := loadKB(t)
	if err := base.Validate(); err != nil {
		t.Fatalf("knowledge base failed validation: %v", err)
	}
}

func TestValidateRejectsUnknownMappingThreat(t *testing.T) {
	base := &kb.KnowledgeBase{
		Threats: map[string]kb.ThreatDef{
			"xss": {ID: "xss", CWE: "CWE-79", Title: "XSS"},
		},
		ThreatMappings: []kb.ThreatMapping{
			{Match: []string{"function:templating"}, Threats: []string{"xss", "nonexistent"}},
		},
	}
	err := base.Validate()
	if err == nil {
		t.Fatal("expected validation error for unknown threat in mapping")
	}
}

func TestValidateRejectsUnknownSinkThreat(t *testing.T) {
	base := &kb.KnowledgeBase{
		Threats: map[string]kb.ThreatDef{
			"xss": {ID: "xss", CWE: "CWE-79", Title: "XSS"},
		},
		Tools: []*kb.ToolDef{
			{
				Tool:   kb.ToolInfo{Name: "Fake"},
				Source: "fake.toml",
				Security: kb.SecurityInfo{
					Sinks: []kb.Sink{
						{Symbol: "eval", Threat: "nonexistent"},
					},
				},
			},
		},
	}
	err := base.Validate()
	if err == nil {
		t.Fatal("expected validation error for unknown threat in sink")
	}
}

func TestValidateRejectsUnknownExplicitThreat(t *testing.T) {
	base := &kb.KnowledgeBase{
		Threats: map[string]kb.ThreatDef{},
		Tools: []*kb.ToolDef{
			{
				Tool:     kb.ToolInfo{Name: "Fake"},
				Source:   "fake.toml",
				Security: kb.SecurityInfo{Threats: []string{"nonexistent"}},
			},
		},
	}
	err := base.Validate()
	if err == nil {
		t.Fatal("expected validation error for unknown explicit threat")
	}
}

func TestTaxonomyEmpty(t *testing.T) {
	var empty kb.Taxonomy
	if !empty.Empty() {
		t.Error("zero-value Taxonomy should be Empty()")
	}

	populated := kb.Taxonomy{Role: []string{"framework"}}
	if populated.Empty() {
		t.Error("Taxonomy with role should not be Empty()")
	}
}

func TestTaxonomyTags(t *testing.T) {
	tax := kb.Taxonomy{
		Role:     []string{"framework"},
		Function: []string{"templating", "authentication"},
		Layer:    []string{"backend"},
	}
	tags := tax.Tags()

	want := []string{"role:framework", "function:templating", "function:authentication", "layer:backend"}
	if len(tags) != len(want) {
		t.Fatalf("expected %d tags, got %d: %v", len(want), len(tags), tags)
	}
	for _, w := range want {
		if !slices.Contains(tags, w) {
			t.Errorf("expected tag %q in %v", w, tags)
		}
	}
}

func TestRailsHasTaxonomy(t *testing.T) {
	base := loadKB(t)
	rails := base.ByName["Rails"]
	if rails == nil {
		t.Fatal("Rails not found in KB")
	}
	if rails.Taxonomy.Empty() {
		t.Fatal("Rails should have taxonomy populated")
	}
	if !slices.Contains(rails.Taxonomy.Role, "framework") {
		t.Errorf("Rails role should include framework, got %v", rails.Taxonomy.Role)
	}
	if !slices.Contains(rails.Taxonomy.Layer, "backend") {
		t.Errorf("Rails layer should include backend, got %v", rails.Taxonomy.Layer)
	}
}

func TestTaxonomyTermsResolve(t *testing.T) {
	base := loadKB(t)
	valid := loadTaxonomyTerms(t)

	var unknown []string
	check := func(source, tag string) {
		if !valid[tag] {
			unknown = append(unknown, source+": "+tag)
		}
	}

	for _, tool := range base.Tools {
		if tool.Taxonomy.Empty() {
			continue
		}
		for _, tag := range tool.Taxonomy.Tags() {
			check(tool.Source, tag)
		}
	}

	for _, m := range base.ThreatMappings {
		for _, tag := range m.Match {
			check("knowledge/_shared/_threats.toml", tag)
		}
	}

	if len(unknown) > 0 {
		sort.Strings(unknown)
		t.Errorf("%d taxonomy tag(s) not in vendored oss-taxonomy term list:\n", len(unknown))
		for _, u := range unknown {
			t.Errorf("  %s", u)
		}
		t.Errorf("If these are new oss-taxonomy terms, refresh the vendored list:\n  curl -s https://taxonomy.ecosyste.ms/terms.txt -o kb/testdata/oss-taxonomy-terms.txt")
	}
}

func TestRubyHasSinks(t *testing.T) {
	base := loadKB(t)
	ruby := base.ByName["Ruby"]
	if ruby == nil {
		t.Fatal("Ruby not found in KB")
	}
	if len(ruby.Security.Sinks) == 0 {
		t.Fatal("Ruby should have sinks populated")
	}

	var foundEval bool
	for _, s := range ruby.Security.Sinks {
		if s.Symbol == "eval" {
			foundEval = true
			if s.Threat != "code_injection" {
				t.Errorf("eval sink threat = %q, want code_injection", s.Threat)
			}
		}
	}
	if !foundEval {
		t.Error("expected eval sink on Ruby")
	}
}
