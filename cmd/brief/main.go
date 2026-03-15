package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/detect"
	"github.com/git-pkgs/brief/kb"
	"github.com/git-pkgs/brief/remote"
	"github.com/git-pkgs/brief/report"

	"golang.org/x/term"
)

func main() {
	debug.SetGCPercent(-1)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "list":
			cmdList(os.Args[2:])
			return
		case "schema":
			cmdSchema()
			return
		case "enrich":
			cmdEnrich(os.Args[2:])
			return
		}
	}

	cmdScan(os.Args[1:])
}

func cmdScan(args []string) {
	fs := flag.NewFlagSet("brief", flag.ExitOnError)
	jsonFlag := fs.Bool("json", false, "Force JSON output")
	humanFlag := fs.Bool("human", false, "Force human-readable output")
	verbose := fs.Bool("verbose", false, "Include breadcrumb/reference information")
	category := fs.String("category", "", "Only report on specific category")
	keep := fs.Bool("keep", false, "Keep downloaded remote source")
	depth := fs.Int("depth", -1, "Git clone depth (0 = full clone, default shallow)")
	dir := fs.String("dir", "", "Directory to clone remote source into")
	scanDepth := fs.Int("scan-depth", 0, "Max directory depth for language detection (default 4)")
	skip := fs.String("skip", "", "Additional directories to skip, comma-separated")
	version := fs.Bool("version", false, "Print version and exit")
	_ = fs.Parse(args)

	if *version {
		_, _ = fmt.Println("brief", brief.Version)
		os.Exit(0)
	}

	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	// Resolve remote sources
	src, err := remote.Resolve(context.Background(), path, remote.Options{
		Keep:  *keep,
		Depth: *depth,
		Dir:   *dir,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer src.Cleanup()

	knowledgeBase, err := kb.Load(brief.KnowledgeFS)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error loading knowledge base: %v\n", err)
		os.Exit(1)
	}

	engine := detect.New(knowledgeBase, src.Dir)
	engine.ScanDepth = *scanDepth
	if *skip != "" {
		engine.SkipDirs = strings.Split(*skip, ",")
	}
	r, err := engine.Run()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

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

func cmdList(args []string) {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: brief list <tools|ecosystems>")
		os.Exit(1)
	}

	knowledgeBase, err := kb.Load(brief.KnowledgeFS)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error loading knowledge base: %v\n", err)
		os.Exit(1)
	}

	switch args[0] {
	case "tools":
		listTools(knowledgeBase)
	case "ecosystems":
		listEcosystems(knowledgeBase)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "unknown list type: %s\nusage: brief list <tools|ecosystems>\n", args[0])
		os.Exit(1)
	}
}

func listTools(knowledgeBase *kb.KnowledgeBase) {
	type toolEntry struct {
		Name        string `json:"name"`
		Category    string `json:"category"`
		Description string `json:"description,omitempty"`
	}

	var tools []toolEntry
	for _, t := range knowledgeBase.Tools {
		tools = append(tools, toolEntry{
			Name:        t.Tool.Name,
			Category:    t.Tool.Category,
			Description: t.Tool.Description,
		})
	}
	sort.Slice(tools, func(i, j int) bool {
		if tools[i].Category != tools[j].Category {
			return tools[i].Category < tools[j].Category
		}
		return tools[i].Name < tools[j].Name
	})

	if isTTY() {
		currentCat := ""
		for _, t := range tools {
			if t.Category != currentCat {
				if currentCat != "" {
					fmt.Println()
				}
				currentCat = t.Category
				_, _ = fmt.Printf("%s:\n", currentCat)
			}
			_, _ = fmt.Printf("  %-25s %s\n", t.Name, t.Description)
		}
	} else {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(tools)
	}
}

func listEcosystems(knowledgeBase *kb.KnowledgeBase) {
	type ecoEntry struct {
		Name  string `json:"name"`
		Tools int    `json:"tools"`
	}

	ecos := knowledgeBase.AllEcosystems()
	sort.Strings(ecos)

	var entries []ecoEntry
	for _, e := range ecos {
		entries = append(entries, ecoEntry{
			Name:  e,
			Tools: len(knowledgeBase.ToolsForEcosystem(e)),
		})
	}

	if isTTY() {
		for _, e := range entries {
			_, _ = fmt.Printf("%-15s %d tools\n", e.Name, e.Tools)
		}
	} else {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(entries)
	}
}

func cmdSchema() {
	schema := map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"title":       "brief report",
		"description": "Output of brief project detection",
		"type":        "object",
		"properties": map[string]any{
			"version":          map[string]any{"type": "string"},
			"path":             map[string]any{"type": "string"},
			"languages":        map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/detection"}},
			"package_managers": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/detection"}},
			"scripts":          map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/script"}},
			"tools":            map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/detection"}}},
			"style":            map[string]any{"$ref": "#/$defs/style"},
			"layout":           map[string]any{"$ref": "#/$defs/layout"},
			"platforms":        map[string]any{"$ref": "#/$defs/platforms"},
			"resources":        map[string]any{"$ref": "#/$defs/resources"},
			"git":              map[string]any{"$ref": "#/$defs/git"},
			"lines":            map[string]any{"$ref": "#/$defs/lines"},
			"stats":            map[string]any{"$ref": "#/$defs/stats"},
		},
		"$defs": map[string]any{
			"detection": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":         map[string]any{"type": "string"},
					"category":     map[string]any{"type": "string"},
					"confidence":   map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
					"command":      map[string]any{"$ref": "#/$defs/command"},
					"config_files": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"lockfile":     map[string]any{"type": "string"},
					"homepage":     map[string]any{"type": "string"},
					"docs":         map[string]any{"type": "string"},
					"repo":         map[string]any{"type": "string"},
					"description":  map[string]any{"type": "string"},
				},
			},
			"command": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"run":           map[string]any{"type": "string"},
					"alternatives":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"source":        map[string]any{"type": "string", "enum": []string{"project_script", "knowledge_base", "config_file"}},
					"inferred_tool": map[string]any{"type": "string"},
				},
			},
			"script": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":   map[string]any{"type": "string"},
					"run":    map[string]any{"type": "string"},
					"source": map[string]any{"type": "string"},
				},
			},
			"style": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"indentation":      map[string]any{"type": "string"},
					"indent_source":    map[string]any{"type": "string"},
					"line_ending":      map[string]any{"type": "string"},
					"trailing_newline": map[string]any{"type": "boolean"},
				},
			},
			"layout": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source_dirs":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"test_dirs":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"mirrors_source": map[string]any{"type": "boolean"},
				},
			},
			"platforms": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"runtime_version_files": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
					"ci_matrix_versions":    map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
					"ci_matrix_os":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
			"resources": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"readme":       map[string]any{"type": "string"},
					"contributing": map[string]any{"type": "string"},
					"changelog":    map[string]any{"type": "string"},
					"license":      map[string]any{"type": "string"},
					"license_type": map[string]any{"type": "string"},
					"security":     map[string]any{"type": "string"},
				},
			},
			"git": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"branch":         map[string]any{"type": "string"},
					"default_branch": map[string]any{"type": "string"},
					"remotes":        map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
					"commit_count":   map[string]any{"type": "integer"},
				},
			},
			"lines": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"total_files": map[string]any{"type": "integer"},
					"total_lines": map[string]any{"type": "integer"},
					"by_language": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}},
					"source":      map[string]any{"type": "string"},
				},
			},
			"stats": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"duration_ms":   map[string]any{"type": "number"},
					"files_checked": map[string]any{"type": "integer"},
					"tools_matched": map[string]any{"type": "integer"},
					"tools_checked": map[string]any{"type": "integer"},
				},
			},
		},
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(schema)
}

func filterCategory(r *brief.Report, category string) *brief.Report {
	filtered := *r
	filtered.Languages = nil
	filtered.PackageManagers = nil
	filtered.Scripts = nil
	filtered.Tools = make(map[string][]brief.Detection)

	switch category {
	case "language":
		filtered.Languages = r.Languages
	case "package_manager":
		filtered.PackageManagers = r.PackageManagers
	case "scripts":
		filtered.Scripts = r.Scripts
	default:
		if tools, ok := r.Tools[category]; ok {
			filtered.Tools[category] = tools
		}
	}

	return &filtered
}

func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
