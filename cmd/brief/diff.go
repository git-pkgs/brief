package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/detect"
	"github.com/git-pkgs/brief/kb"
	"github.com/git-pkgs/brief/report"
)

func cmdDiff(args []string) {
	fs := flag.NewFlagSet("brief diff", flag.ExitOnError)
	jsonFlag := fs.Bool("json", false, "Force JSON output")
	humanFlag := fs.Bool("human", false, "Force human-readable output")
	verbose := fs.Bool("verbose", false, "Include breadcrumb/reference information")
	category := fs.String("category", "", "Only report on specific category")
	_ = fs.Parse(args)

	// Determine the project root (git toplevel).
	root, err := gitToplevel()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: not a git repository\n")
		os.Exit(1)
	}

	// Determine diff refs.
	// No args: diff from default branch to working tree (including uncommitted).
	// One arg: diff from that ref to working tree.
	// Two args: diff between two refs.
	var ref1, ref2 string
	var diffRef string
	includeUncommitted := false

	switch fs.NArg() {
	case 0:
		// Default: compare against the default branch + uncommitted changes.
		ref1 = detectDefaultBranch(root)
		includeUncommitted = true
		diffRef = ref1 + "..HEAD (+ uncommitted)"
	case 1:
		ref1 = fs.Arg(0)
		includeUncommitted = true
		diffRef = ref1 + " (+ uncommitted)"
	case 2:
		ref1 = fs.Arg(0)
		ref2 = fs.Arg(1)
		diffRef = ref1 + ".." + ref2
	default:
		_, _ = fmt.Fprintf(os.Stderr, "usage: brief diff [ref1] [ref2]\n")
		os.Exit(1)
	}

	changedFiles, err := gitChangedFiles(context.Background(), root, ref1, ref2, includeUncommitted)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(changedFiles) == 0 {
		_, _ = fmt.Fprintf(os.Stderr, "no changed files\n")
		os.Exit(0)
	}

	knowledgeBase, err := kb.Load(brief.KnowledgeFS)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error loading knowledge base: %v\n", err)
		os.Exit(1)
	}

	engine := detect.New(knowledgeBase, root)
	r, err := engine.Run()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	r.DiffRef = diffRef
	r.ChangedFiles = changedFiles

	r = detect.FilterByChangedFiles(r, knowledgeBase, changedFiles)

	if *category != "" {
		r = filterCategory(r, *category)
	}

	useJSON := *jsonFlag || (!*humanFlag && !isTTY())

	if useJSON {
		if err := report.JSON(os.Stdout, r); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
			os.Exit(1)
		}
	} else {
		report.Human(os.Stdout, r, *verbose)
	}
}

// gitToplevel returns the root of the git repository.
func gitToplevel() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// detectDefaultBranch tries to find the default branch name (main or master).
func detectDefaultBranch(root string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "origin/HEAD")
	cmd.Dir = root
	if out, err := cmd.Output(); err == nil {
		ref := strings.TrimSpace(string(out))
		if after, ok := strings.CutPrefix(ref, "origin/"); ok && after != "" {
			return after
		}
	}

	// Fall back to checking if main or master exists.
	for _, branch := range []string{"main", "master"} {
		cmd := exec.Command("git", "rev-parse", "--verify", branch)
		cmd.Dir = root
		if err := cmd.Run(); err == nil {
			return branch
		}
	}

	return "main"
}

// gitChangedFiles returns the list of files that differ between refs.
// If includeUncommitted is true, also includes staged and unstaged changes.
func gitChangedFiles(ctx context.Context, root, ref1, ref2 string, includeUncommitted bool) ([]string, error) {
	seen := make(map[string]bool)
	var files []string

	addFiles := func(output string) {
		for _, f := range strings.Split(strings.TrimSpace(output), "\n") {
			f = strings.TrimSpace(f)
			if f != "" && !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}

	if ref2 != "" {
		// Two-ref diff: just diff between the two.
		cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", ref1+"..."+ref2)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			// Try two-dot diff if three-dot fails.
			cmd = exec.CommandContext(ctx, "git", "diff", "--name-only", ref1, ref2)
			cmd.Dir = root
			out, err = cmd.Output()
			if err != nil {
				return nil, fmt.Errorf("git diff %s %s: %w", ref1, ref2, err)
			}
		}
		addFiles(string(out))
	} else {
		// Compare ref1 to HEAD.
		cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", ref1+"...HEAD")
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			cmd = exec.CommandContext(ctx, "git", "diff", "--name-only", ref1, "HEAD")
			cmd.Dir = root
			out, err = cmd.Output()
			if err != nil {
				return nil, fmt.Errorf("git diff %s: %w", ref1, err)
			}
		}
		addFiles(string(out))

		if includeUncommitted {
			// Staged changes.
			cmd = exec.CommandContext(ctx, "git", "diff", "--name-only", "--cached")
			cmd.Dir = root
			if out, err := cmd.Output(); err == nil {
				addFiles(string(out))
			}

			// Unstaged changes.
			cmd = exec.CommandContext(ctx, "git", "diff", "--name-only")
			cmd.Dir = root
			if out, err := cmd.Output(); err == nil {
				addFiles(string(out))
			}

			// Untracked files.
			cmd = exec.CommandContext(ctx, "git", "ls-files", "--others", "--exclude-standard")
			cmd.Dir = root
			if out, err := cmd.Output(); err == nil {
				addFiles(string(out))
			}
		}
	}

	// Make paths relative to root.
	for i, f := range files {
		if filepath.IsAbs(f) {
			if rel, err := filepath.Rel(root, f); err == nil {
				files[i] = rel
			}
		}
	}

	return files, nil
}
