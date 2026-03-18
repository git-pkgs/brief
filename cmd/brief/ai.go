package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/chaoss/ai-detection-action/detection"
	"github.com/chaoss/ai-detection-action/detection/coauthor"
	"github.com/chaoss/ai-detection-action/detection/committer"
	"github.com/chaoss/ai-detection-action/detection/message"
	"github.com/chaoss/ai-detection-action/detection/toolmention"
	"github.com/chaoss/ai-detection-action/scan"
)

func cmdAI(args []string) {
	fs := flag.NewFlagSet("brief ai", flag.ExitOnError)
	jsonFlag := fs.Bool("json", false, "Force JSON output")
	humanFlag := fs.Bool("human", false, "Force human-readable output")
	markdownFlag := fs.Bool("markdown", false, "Force markdown output")
	commitRange := fs.String("range", "", "Commit range to scan (e.g. base..head)")
	minConfidence := fs.String("min-confidence", "low", "Minimum confidence level: low, medium, or high")
	_ = fs.Parse(args)

	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	minConf, err := parseConfidence(*minConfidence)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	detectors := []detection.Detector{
		&committer.Detector{},
		&coauthor.Detector{},
		&message.Detector{},
		&toolmention.Detector{},
	}

	report, err := scan.ScanCommitRange(path, *commitRange, detectors)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	report = filterByConfidence(report, minConf)

	switch {
	case *markdownFlag:
		aiMarkdown(os.Stdout, report)
	case *jsonFlag || (!*humanFlag && !isTTY()):
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
			os.Exit(1)
		}
	default:
		aiHuman(os.Stdout, report)
	}

	if report.Summary.AICommits > 0 {
		os.Exit(1)
	}
}

func parseConfidence(s string) (detection.Confidence, error) {
	switch s {
	case "low":
		return detection.ConfidenceLow, nil
	case "medium":
		return detection.ConfidenceMedium, nil
	case "high":
		return detection.ConfidenceHigh, nil
	default:
		return 0, fmt.Errorf("invalid confidence level %q, expected low, medium, or high", s)
	}
}

func filterByConfidence(r scan.Report, minConf detection.Confidence) scan.Report {
	var filtered []scan.CommitResult
	for _, cr := range r.Commits {
		var findings []detection.Finding
		for _, f := range cr.Findings {
			if f.Confidence >= minConf {
				findings = append(findings, f)
			}
		}
		filtered = append(filtered, scan.CommitResult{
			Hash:     cr.Hash,
			Findings: findings,
		})
	}

	// Rebuild summary from filtered results.
	summary := scan.Summary{
		TotalCommits: len(filtered),
		ToolCounts:   map[string]int{},
		ByConfidence: map[string]int{},
	}
	for _, cr := range filtered {
		if len(cr.Findings) > 0 {
			summary.AICommits++
		}
		for _, f := range cr.Findings {
			summary.ToolCounts[f.Tool]++
			summary.ByConfidence[f.Confidence.String()]++
		}
	}

	return scan.Report{
		Commits: filtered,
		Summary: summary,
	}
}

const shortHashLen = 12

func aiHuman(w io.Writer, r scan.Report) {
	_, _ = fmt.Fprintf(w, "%d commits scanned, %d with AI signals\n", r.Summary.TotalCommits, r.Summary.AICommits)

	if r.Summary.AICommits == 0 {
		return
	}

	_, _ = fmt.Fprintln(w)
	for tool, count := range r.Summary.ToolCounts {
		_, _ = fmt.Fprintf(w, "  %-25s %d commits\n", tool, count)
	}

	_, _ = fmt.Fprintln(w)
	for _, cr := range r.Commits {
		if len(cr.Findings) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(w, "%s\n", cr.Hash[:min(len(cr.Hash), shortHashLen)])
		for _, f := range cr.Findings {
			_, _ = fmt.Fprintf(w, "  [%s] %s: %s\n", f.Confidence, f.Tool, f.Detail)
		}
	}
}

func aiMarkdown(w io.Writer, r scan.Report) {
	_, _ = fmt.Fprintf(w, "## AI Detection\n\n")
	_, _ = fmt.Fprintf(w, "%d commits scanned, %d with AI signals\n\n", r.Summary.TotalCommits, r.Summary.AICommits)

	if r.Summary.AICommits == 0 {
		return
	}

	_, _ = fmt.Fprintln(w, "| Tool | Commits |")
	_, _ = fmt.Fprintln(w, "|------|---------|")
	for tool, count := range r.Summary.ToolCounts {
		_, _ = fmt.Fprintf(w, "| %s | %d |\n", tool, count)
	}

	_, _ = fmt.Fprintf(w, "\n### Findings\n\n")
	for _, cr := range r.Commits {
		if len(cr.Findings) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(w, "**%s**\n\n", cr.Hash[:min(len(cr.Hash), shortHashLen)])
		for _, f := range cr.Findings {
			_, _ = fmt.Fprintf(w, "- [%s] %s: %s\n", f.Confidence, f.Tool, f.Detail)
		}
		_, _ = fmt.Fprintln(w)
	}
}
