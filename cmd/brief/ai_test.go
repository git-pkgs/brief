package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chaoss/ai-detection-action/detection"
	"github.com/chaoss/ai-detection-action/scan"
)

func TestParseConfidence(t *testing.T) {
	tests := []struct {
		input string
		want  detection.Confidence
		err   bool
	}{
		{"low", detection.ConfidenceLow, false},
		{"medium", detection.ConfidenceMedium, false},
		{"high", detection.ConfidenceHigh, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		got, err := parseConfidence(tt.input)
		if tt.err && err == nil {
			t.Errorf("parseConfidence(%q) expected error", tt.input)
		}
		if !tt.err && err != nil {
			t.Errorf("parseConfidence(%q) unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("parseConfidence(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestFilterByConfidence(t *testing.T) {
	report := scan.Report{
		Commits: []scan.CommitResult{
			{
				Hash: "abc123",
				Findings: []detection.Finding{
					{Detector: "toolmention", Tool: "Claude", Confidence: detection.ConfidenceLow, Detail: "mentions Claude"},
					{Detector: "coauthor", Tool: "Claude Code", Confidence: detection.ConfidenceHigh, Detail: "co-author trailer"},
				},
			},
			{
				Hash: "def456",
				Findings: []detection.Finding{
					{Detector: "message", Tool: "Aider", Confidence: detection.ConfidenceMedium, Detail: "aider: prefix"},
				},
			},
			{
				Hash:     "ghi789",
				Findings: nil,
			},
		},
		Summary: scan.Summary{
			TotalCommits: 3,
			AICommits:    2,
			ToolCounts:   map[string]int{"Claude": 1, "Claude Code": 1, "Aider": 1},
			ByConfidence: map[string]int{"low": 1, "medium": 1, "high": 1},
		},
	}

	t.Run("filter low keeps commits with findings", func(t *testing.T) {
		filtered := filterByConfidence(report, detection.ConfidenceLow)
		if filtered.Summary.AICommits != 2 {
			t.Errorf("expected 2 AI commits, got %d", filtered.Summary.AICommits)
		}
		if len(filtered.Commits) != 2 {
			t.Errorf("expected 2 commits (empty-finding commit excluded), got %d", len(filtered.Commits))
		}
		if len(filtered.Commits[0].Findings) != 2 {
			t.Errorf("expected 2 findings in first commit, got %d", len(filtered.Commits[0].Findings))
		}
	})

	t.Run("filter medium drops low", func(t *testing.T) {
		filtered := filterByConfidence(report, detection.ConfidenceMedium)
		if filtered.Summary.AICommits != 2 {
			t.Errorf("expected 2 AI commits, got %d", filtered.Summary.AICommits)
		}
		if len(filtered.Commits[0].Findings) != 1 {
			t.Errorf("expected 1 finding in first commit, got %d", len(filtered.Commits[0].Findings))
		}
		if filtered.Commits[0].Findings[0].Tool != "Claude Code" {
			t.Errorf("expected Claude Code finding, got %s", filtered.Commits[0].Findings[0].Tool)
		}
	})

	t.Run("filter high drops commits with no remaining findings", func(t *testing.T) {
		filtered := filterByConfidence(report, detection.ConfidenceHigh)
		if filtered.Summary.AICommits != 1 {
			t.Errorf("expected 1 AI commit, got %d", filtered.Summary.AICommits)
		}
		if len(filtered.Commits) != 1 {
			t.Errorf("expected 1 commit, got %d", len(filtered.Commits))
		}
		if filtered.Commits[0].Hash != "abc123" {
			t.Errorf("expected abc123, got %s", filtered.Commits[0].Hash)
		}
	})

	t.Run("total commits preserved", func(t *testing.T) {
		filtered := filterByConfidence(report, detection.ConfidenceHigh)
		if filtered.Summary.TotalCommits != 3 {
			t.Errorf("expected 3 total commits, got %d", filtered.Summary.TotalCommits)
		}
	})
}

func TestAIHuman(t *testing.T) {
	r := scan.Report{
		Commits: []scan.CommitResult{
			{
				Hash: "abc123def456abc123def456abc123def456abc1",
				Findings: []detection.Finding{
					{Detector: "coauthor", Tool: "Claude Code", Confidence: detection.ConfidenceHigh, Detail: "co-author trailer"},
				},
			},
		},
		Summary: scan.Summary{
			TotalCommits: 5,
			AICommits:    1,
			ToolCounts:   map[string]int{"Claude Code": 1},
			ByConfidence: map[string]int{"high": 1},
		},
	}

	var buf bytes.Buffer
	aiHuman(&buf, r)
	out := buf.String()

	checks := []string{
		"5 commits scanned, 1 with AI signals",
		"Claude Code",
		"abc123def456",
		"[high]",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestAIHumanNoFindings(t *testing.T) {
	r := scan.Report{
		Summary: scan.Summary{
			TotalCommits: 3,
			AICommits:    0,
			ToolCounts:   map[string]int{},
			ByConfidence: map[string]int{},
		},
	}

	var buf bytes.Buffer
	aiHuman(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "0 with AI signals") {
		t.Errorf("expected no signals message\ngot:\n%s", out)
	}
}

func TestAIMarkdown(t *testing.T) {
	r := scan.Report{
		Commits: []scan.CommitResult{
			{
				Hash: "abc123def456abc123def456abc123def456abc1",
				Findings: []detection.Finding{
					{Detector: "coauthor", Tool: "Claude Code", Confidence: detection.ConfidenceHigh, Detail: "co-author trailer"},
				},
			},
		},
		Summary: scan.Summary{
			TotalCommits: 5,
			AICommits:    1,
			ToolCounts:   map[string]int{"Claude Code": 1},
			ByConfidence: map[string]int{"high": 1},
		},
	}

	var buf bytes.Buffer
	aiMarkdown(&buf, r)
	out := buf.String()

	checks := []string{
		"## AI Detection",
		"5 commits scanned, 1 with AI signals",
		"| Tool | Commits |",
		"| Claude Code | 1 |",
		"### Findings",
		"**abc123def456**",
		"- [high] Claude Code: co-author trailer",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestAIMarkdownNoFindings(t *testing.T) {
	r := scan.Report{
		Summary: scan.Summary{
			TotalCommits: 3,
			AICommits:    0,
			ToolCounts:   map[string]int{},
			ByConfidence: map[string]int{},
		},
	}

	var buf bytes.Buffer
	aiMarkdown(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "0 with AI signals") {
		t.Errorf("expected no signals message\ngot:\n%s", out)
	}
	if strings.Contains(out, "### Findings") {
		t.Errorf("should not have findings section\ngot:\n%s", out)
	}
}
