package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestCitationCLIHelper(_ *testing.T) {
	root := os.Getenv("BRIEF_CITATION_ROOT")
	if root == "" {
		return
	}
	mode := os.Getenv("BRIEF_CITATION_MODE")
	switch mode {
	case "schema":
		cmdSchema()
	case "diff":
		cmdDiff([]string{"--json", "HEAD"})
	default:
		cmdScan(append(strings.Fields(mode), root))
	}
	os.Exit(0)
}

func TestCitationCLI(t *testing.T) {
	root, err := filepath.Abs("../../testdata/citation-project")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"--json", "--human", "--markdown", "--human --verbose", "--markdown --verbose"} {
		t.Run(mode, func(t *testing.T) {
			out := citationCLI(t, root, mode)
			if mode == "--json" {
				var r brief.Report
				if err := json.Unmarshal(out, &r); err != nil {
					t.Fatal(err)
				}
				if r.Resources == nil || r.Resources.Citation == nil || r.Resources.Citation.ValidationStatus != "valid" || r.Resources.Citation.PreferredCitation.DOI != "10.1234/paper" {
					t.Fatalf("citation output: %s", out)
				}
				return
			}
			for _, want := range []string{"Example Research Software", "Alex van Example III", "Research Team", "10.1234/software", "10.1234/archive", "10.1234/paper", "The accompanying paper", "1.20", "2024-02-29"} {
				if !strings.Contains(string(out), want) {
					t.Errorf("missing %q in output: %s", want, out)
				}
			}
			if strings.Contains(mode, "verbose") && !strings.Contains(string(out), "Simulates example systems for research.") {
				t.Fatalf("missing verbose metadata: %s", out)
			}
		})
	}
}

func TestCitationCLISchema(t *testing.T) {
	root := t.TempDir()
	var schema struct {
		Defs map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(citationCLI(t, root, "schema"), &schema); err != nil {
		t.Fatal(err)
	}
	for name, fields := range map[string][]string{
		"resourceinfo": {"citation"}, "citationinfo": {"parse_status", "preferred_citation", "diagnostics"},
		"citationdiagnostic": {"code", "line", "column"},
	} {
		for _, field := range fields {
			if _, ok := schema.Defs[name].Properties[field]; !ok {
				t.Errorf("schema missing %s.%s", name, field)
			}
		}
	}
}

func TestCitationCLIDiff(t *testing.T) {
	root := t.TempDir()
	const content = "cff-version: 1.2.0\ntitle: First\nmessage: Cite\nauthors: [{name: Team}]\n"
	writeScanFixture(t, root, "CITATION.cff", content)
	runGitFixture(t, root, "init", "-q")
	runGitFixture(t, root, "add", "CITATION.cff")
	runGitFixture(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "initial")
	writeScanFixture(t, root, "CITATION.cff", strings.Replace(content, "First", "Second", 1))
	var r brief.Report
	if err := json.Unmarshal(citationCLI(t, root, "diff"), &r); err != nil {
		t.Fatal(err)
	}
	if r.Resources == nil || r.Resources.Citation == nil || r.Resources.Citation.Title != "Second" {
		t.Fatalf("changed citation missing: %+v", r.Resources)
	}
}

func citationCLI(t *testing.T, root, mode string) []byte {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCitationCLIHelper$")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "BRIEF_CITATION_ROOT="+root, "BRIEF_CITATION_MODE="+mode)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("citation CLI %s: %v\n%s", mode, err, out)
	}
	return out
}
