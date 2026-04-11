package detect

import (
	"slices"
	"testing"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/kb"
)

// syntheticEngine builds an Engine wrapping a hand-constructed KB,
// for tests that don't want to load the full embedded knowledge base.
func syntheticEngine(base *kb.KnowledgeBase) *Engine {
	return &Engine{
		KB:                 base,
		detectedEcosystems: make(map[string]bool),
	}
}

func TestThreatModelRubyProject(t *testing.T) {
	engine := New(loadKB(t), "../testdata/ruby-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := engine.ThreatModel(r)

	if tr.Path != r.Path {
		t.Errorf("Path = %q, want %q", tr.Path, r.Path)
	}
	if !slices.Contains(tr.Ecosystems, "ruby") {
		t.Errorf("expected ruby in ecosystems, got %v", tr.Ecosystems)
	}

	// Rails has role:framework + layer:backend, which fires the backend-framework mapping.
	// That mapping includes xss, csrf, open_redirect, ssrf, path_traversal, auth_bypass.
	// Rails also has function:templating which fires xss + ssti.
	// Rails also has function:authentication which fires auth_bypass + session_fixation.
	threatIDs := make(map[string]bool)
	for _, th := range tr.Threats {
		threatIDs[th.ID] = true
	}

	wantThreats := []string{"xss", "csrf", "ssti", "auth_bypass", "ssrf"}
	for _, w := range wantThreats {
		if !threatIDs[w] {
			t.Errorf("expected threat %q, got %v", w, tr.Threats)
		}
	}

	// xss should be introduced by Rails (via both backend-framework and templating mappings).
	for _, th := range tr.Threats {
		if th.ID == "xss" {
			if !slices.Contains(th.IntroducedBy, "Rails") {
				t.Errorf("xss introduced_by = %v, want to include Rails", th.IntroducedBy)
			}
			if th.CWE != "CWE-79" {
				t.Errorf("xss CWE = %q, want CWE-79", th.CWE)
			}
			if th.Title == "" {
				t.Error("xss should have a title from the registry")
			}
		}
	}

	// Stack should include Rails (has taxonomy) and Ruby (has sinks)
	// but not RuboCop (no taxonomy, no security data).
	stackNames := make(map[string]bool)
	for _, s := range tr.Stack {
		stackNames[s.Name] = true
	}
	if !stackNames["Rails"] {
		t.Error("expected Rails in stack")
	}
	if !stackNames["Ruby"] {
		t.Error("expected Ruby in stack (has sinks)")
	}
	if stackNames["RuboCop"] {
		t.Error("RuboCop has no taxonomy/security data, should not be in stack")
	}
}

func TestThreatModelDeterministic(t *testing.T) {
	engine := New(loadKB(t), "../testdata/ruby-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr1 := engine.ThreatModel(r)
	tr2 := engine.ThreatModel(r)

	if len(tr1.Threats) != len(tr2.Threats) {
		t.Fatalf("threat count differs between runs: %d vs %d", len(tr1.Threats), len(tr2.Threats))
	}
	for i := range tr1.Threats {
		if tr1.Threats[i].ID != tr2.Threats[i].ID {
			t.Errorf("threat order differs at %d: %q vs %q", i, tr1.Threats[i].ID, tr2.Threats[i].ID)
		}
		if !slices.Equal(tr1.Threats[i].IntroducedBy, tr2.Threats[i].IntroducedBy) {
			t.Errorf("introduced_by order differs for %q", tr1.Threats[i].ID)
		}
	}
}

func TestThreatModelConjunctiveMatch(t *testing.T) {
	// A tool with role:framework but NOT layer:backend should not trigger
	// the [role:framework, layer:backend] mapping.
	base := &kb.KnowledgeBase{
		ByName: map[string]*kb.ToolDef{
			"FrontendOnly": {
				Tool:     kb.ToolInfo{Name: "FrontendOnly"},
				Taxonomy: kb.Taxonomy{Role: []string{"framework"}, Layer: []string{"frontend"}},
			},
		},
		Threats: map[string]kb.ThreatDef{
			"xss":           {ID: "xss", CWE: "CWE-79", Title: "XSS"},
			"csrf":          {ID: "csrf", CWE: "CWE-352", Title: "CSRF"},
			"open_redirect": {ID: "open_redirect", CWE: "CWE-601", Title: "Open Redirect"},
		},
		ThreatMappings: []kb.ThreatMapping{
			{Match: []string{"role:framework", "layer:backend"}, Threats: []string{"csrf"}},
			{Match: []string{"role:framework", "layer:frontend"}, Threats: []string{"xss", "open_redirect"}},
		},
	}

	r := &brief.Report{
		Tools: map[string][]brief.Detection{
			"build": {{Name: "FrontendOnly"}},
		},
	}

	tr := syntheticEngine(base).ThreatModel(r)

	threatIDs := make(map[string]bool)
	for _, th := range tr.Threats {
		threatIDs[th.ID] = true
	}

	if threatIDs["csrf"] {
		t.Error("csrf should not fire: tool has layer:frontend, mapping requires layer:backend")
	}
	if !threatIDs["xss"] {
		t.Error("xss should fire: tool matches role:framework + layer:frontend")
	}
	if !threatIDs["open_redirect"] {
		t.Error("open_redirect should fire")
	}
}

func TestThreatModelDeduplicatesIntroducers(t *testing.T) {
	// Two tools both introducing xss should produce one threat entry
	// with both names in introduced_by.
	base := &kb.KnowledgeBase{
		ByName: map[string]*kb.ToolDef{
			"ToolA": {
				Tool:     kb.ToolInfo{Name: "ToolA"},
				Taxonomy: kb.Taxonomy{Function: []string{"templating"}},
			},
			"ToolB": {
				Tool:     kb.ToolInfo{Name: "ToolB"},
				Taxonomy: kb.Taxonomy{Function: []string{"templating"}},
			},
		},
		Threats: map[string]kb.ThreatDef{
			"xss": {ID: "xss", CWE: "CWE-79", Title: "XSS"},
		},
		ThreatMappings: []kb.ThreatMapping{
			{Match: []string{"function:templating"}, Threats: []string{"xss"}},
		},
	}

	r := &brief.Report{
		Tools: map[string][]brief.Detection{
			"build": {{Name: "ToolA"}, {Name: "ToolB"}},
		},
	}

	tr := syntheticEngine(base).ThreatModel(r)

	if len(tr.Threats) != 1 {
		t.Fatalf("expected 1 threat, got %d", len(tr.Threats))
	}
	got := tr.Threats[0].IntroducedBy
	want := []string{"ToolA", "ToolB"}
	if !slices.Equal(got, want) {
		t.Errorf("introduced_by = %v, want %v", got, want)
	}
}

func TestThreatModelExplicitThreats(t *testing.T) {
	// A tool with no taxonomy but explicit [security].threats should still contribute.
	base := &kb.KnowledgeBase{
		ByName: map[string]*kb.ToolDef{
			"Bare": {
				Tool:     kb.ToolInfo{Name: "Bare"},
				Security: kb.SecurityInfo{Threats: []string{"ssrf"}},
			},
		},
		Threats: map[string]kb.ThreatDef{
			"ssrf": {ID: "ssrf", CWE: "CWE-918", Title: "SSRF"},
		},
	}

	r := &brief.Report{Languages: []brief.Detection{{Name: "Bare"}}}
	tr := syntheticEngine(base).ThreatModel(r)

	if len(tr.Threats) != 1 || tr.Threats[0].ID != "ssrf" {
		t.Fatalf("expected ssrf threat, got %v", tr.Threats)
	}
}

func TestThreatModelEmptyMatch(t *testing.T) {
	// A mapping with empty match should never fire.
	base := &kb.KnowledgeBase{
		ByName: map[string]*kb.ToolDef{
			"Any": {
				Tool:     kb.ToolInfo{Name: "Any"},
				Taxonomy: kb.Taxonomy{Role: []string{"library"}},
			},
		},
		Threats: map[string]kb.ThreatDef{
			"xss": {ID: "xss", Title: "XSS"},
		},
		ThreatMappings: []kb.ThreatMapping{
			{Match: []string{}, Threats: []string{"xss"}},
		},
	}

	r := &brief.Report{Tools: map[string][]brief.Detection{"build": {{Name: "Any"}}}}
	tr := syntheticEngine(base).ThreatModel(r)

	if len(tr.Threats) != 0 {
		t.Errorf("empty match should fire on nothing, got %v", tr.Threats)
	}
}

func TestThreatModelGoProjectEmpty(t *testing.T) {
	// Go fixture has no tools with taxonomy/security data yet.
	engine := New(loadKB(t), "../testdata/go-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := engine.ThreatModel(r)
	if len(tr.Threats) != 0 {
		t.Errorf("go-project has no security data, expected 0 threats, got %d", len(tr.Threats))
	}
	if len(tr.Stack) != 0 {
		t.Errorf("go-project has no taxonomy data, expected empty stack, got %v", tr.Stack)
	}
}

func TestSinksRubyProject(t *testing.T) {
	engine := New(loadKB(t), "../testdata/ruby-project")
	r, err := engine.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sr := engine.Sinks(r)

	if len(sr.Sinks) == 0 {
		t.Fatal("expected sinks from Ruby language def")
	}

	// All sinks in this fixture come from Ruby (only ruby/language.toml has sinks).
	bySymbol := make(map[string]brief.SinkEntry)
	for _, s := range sr.Sinks {
		if s.Tool != "Ruby" {
			t.Errorf("unexpected sink from %q: %v", s.Tool, s)
		}
		bySymbol[s.Symbol] = s
	}

	if e, ok := bySymbol["eval"]; !ok {
		t.Error("expected eval sink")
	} else if e.Threat != "code_injection" || e.CWE != "CWE-95" {
		t.Errorf("eval sink = %+v", e)
	}

	if _, ok := bySymbol["Marshal.load"]; !ok {
		t.Error("expected Marshal.load sink")
	}
}

func TestSinksFillsCWEFromRegistry(t *testing.T) {
	base := &kb.KnowledgeBase{
		ByName: map[string]*kb.ToolDef{
			"Lib": {
				Tool: kb.ToolInfo{Name: "Lib"},
				Security: kb.SecurityInfo{
					Sinks: []kb.Sink{
						{Symbol: "danger", Threat: "xss"}, // no CWE on the sink
					},
				},
			},
		},
		Threats: map[string]kb.ThreatDef{
			"xss": {ID: "xss", CWE: "CWE-79", Title: "XSS"},
		},
	}

	r := &brief.Report{Languages: []brief.Detection{{Name: "Lib"}}}
	sr := syntheticEngine(base).Sinks(r)

	if len(sr.Sinks) != 1 {
		t.Fatalf("expected 1 sink, got %d", len(sr.Sinks))
	}
	if sr.Sinks[0].CWE != "CWE-79" {
		t.Errorf("CWE = %q, want CWE-79 from registry", sr.Sinks[0].CWE)
	}
}

func TestSinksSorted(t *testing.T) {
	base := &kb.KnowledgeBase{
		ByName: map[string]*kb.ToolDef{
			"Zebra": {
				Tool: kb.ToolInfo{Name: "Zebra"},
				Security: kb.SecurityInfo{
					Sinks: []kb.Sink{{Symbol: "exec", Threat: "x"}},
				},
			},
			"Alpha": {
				Tool: kb.ToolInfo{Name: "Alpha"},
				Security: kb.SecurityInfo{
					Sinks: []kb.Sink{
						{Symbol: "zebra_method", Threat: "x"},
						{Symbol: "alpha_method", Threat: "x"},
					},
				},
			},
		},
		Threats: map[string]kb.ThreatDef{"x": {ID: "x"}},
	}

	r := &brief.Report{
		Languages: []brief.Detection{{Name: "Zebra"}, {Name: "Alpha"}},
	}
	sr := syntheticEngine(base).Sinks(r)

	if len(sr.Sinks) != 3 {
		t.Fatalf("expected 3 sinks, got %d", len(sr.Sinks))
	}
	// Sorted by tool, then symbol.
	want := []struct{ tool, symbol string }{
		{"Alpha", "alpha_method"},
		{"Alpha", "zebra_method"},
		{"Zebra", "exec"},
	}
	for i, w := range want {
		if sr.Sinks[i].Tool != w.tool || sr.Sinks[i].Symbol != w.symbol {
			t.Errorf("sink[%d] = %s/%s, want %s/%s", i, sr.Sinks[i].Tool, sr.Sinks[i].Symbol, w.tool, w.symbol)
		}
	}
}

func TestMatchesAll(t *testing.T) {
	have := map[string]bool{"a": true, "b": true, "c": true}

	cases := []struct {
		required []string
		want     bool
	}{
		{[]string{"a"}, true},
		{[]string{"a", "b"}, true},
		{[]string{"a", "b", "c"}, true},
		{[]string{"a", "x"}, false},
		{[]string{"x"}, false},
		{[]string{}, false}, // vacuous match disallowed
		{nil, false},
	}
	for _, tc := range cases {
		if got := matchesAll(have, tc.required); got != tc.want {
			t.Errorf("matchesAll(%v, %v) = %v, want %v", have, tc.required, got, tc.want)
		}
	}
}
