package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/detect"
	"github.com/git-pkgs/brief/kb"
	"github.com/git-pkgs/brief/remote"
	"github.com/git-pkgs/brief/report"

	"golang.org/x/term"
)

func main() {
	// Disable GC for the duration of this short-lived CLI process.
	// Detection typically completes in under 100ms and allocates modestly,
	// so skipping collection avoids ~10% overhead. The enrich subcommand
	// may allocate more due to network I/O, but still finishes quickly.
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
		case "diff":
			cmdDiff(os.Args[2:])
			return
		case "missing":
			cmdMissing(os.Args[2:])
			return
		case "threat-model":
			cmdThreatModel(os.Args[2:])
			return
		case "sinks":
			cmdSinks(os.Args[2:])
			return
		case "outline":
			cmdOutline(os.Args[2:])
			return
		}
	}

	cmdScan(os.Args[1:])
}

func cmdScan(args []string) {
	fs := flag.NewFlagSet("brief", flag.ExitOnError)
	jsonFlag := fs.Bool("json", false, "Force JSON output")
	humanFlag := fs.Bool("human", false, "Force human-readable output")
	markdownFlag := fs.Bool("markdown", false, "Force markdown output")
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

	code := runScan(src.Dir, *scanDepth, *skip, *category, *jsonFlag, *humanFlag, *markdownFlag, *verbose)
	src.Cleanup()
	os.Exit(code)
}

func runScan(dir string, scanDepth int, skip, category string, jsonFlag, humanFlag, markdownFlag, verbose bool) int {
	knowledgeBase, err := kb.Load(brief.KnowledgeFS)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error loading knowledge base: %v\n", err)
		return 1
	}

	engine := detect.New(knowledgeBase, dir)
	engine.ScanDepth = scanDepth
	if skip != "" {
		engine.SkipDirs = strings.Split(skip, ",")
	}
	r, err := engine.Run()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if category != "" {
		r = filterCategory(r, category)
	}

	switch {
	case markdownFlag:
		report.Markdown(os.Stdout, r, verbose)
	case jsonFlag || (!humanFlag && !isTTY()):
		if err := report.JSON(os.Stdout, r); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
			return 1
		}
	default:
		report.Human(os.Stdout, r, verbose)
	}
	return 0
}

func cmdList(args []string) {
	fs := flag.NewFlagSet("brief list", flag.ExitOnError)
	readmeFlag := fs.Bool("readme", false, "Output markdown for README")
	_ = fs.Parse(args)

	if fs.NArg() == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: brief list <tools|ecosystems>")
		os.Exit(1)
	}

	knowledgeBase, err := kb.Load(brief.KnowledgeFS)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error loading knowledge base: %v\n", err)
		os.Exit(1)
	}

	switch fs.Arg(0) {
	case "tools":
		if *readmeFlag {
			listToolsReadme(knowledgeBase)
		} else {
			listTools(knowledgeBase)
		}
	case "ecosystems":
		listEcosystems(knowledgeBase)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "unknown list type: %s\nusage: brief list <tools|ecosystems>\n", fs.Arg(0))
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
		if err := enc.Encode(tools); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
			os.Exit(1)
		}
	}
}

func listToolsReadme(knowledgeBase *kb.KnowledgeBase) {
	// Group tools by category, deduplicating names.
	seen := make(map[string]map[string]bool)
	byCategory := make(map[string][]string)
	for _, t := range knowledgeBase.Tools {
		if t.Tool.Name == "" {
			continue
		}
		cat := t.Tool.Category
		if seen[cat] == nil {
			seen[cat] = make(map[string]bool)
		}
		if seen[cat][t.Tool.Name] {
			continue
		}
		seen[cat][t.Tool.Name] = true
		byCategory[cat] = append(byCategory[cat], t.Tool.Name)
	}
	for _, names := range byCategory {
		sort.Strings(names)
	}

	languages := byCategory["language"]

	ecosystems := knowledgeBase.AllEcosystems()
	totalTools := 0
	for _, names := range byCategory {
		totalTools += len(names)
	}

	_, _ = fmt.Fprintf(os.Stdout, "## What it detects\n\n")
	_, _ = fmt.Fprintf(os.Stdout, "%d language ecosystems with %d tool definitions across %d categories.\n\n",
		len(ecosystems), totalTools, len(byCategory))

	// Languages
	if len(languages) > 0 {
		_, _ = fmt.Fprintf(os.Stdout, "**Languages:** %s.\n\n", strings.Join(languages, ", "))
	}

	// Package managers
	if pms := byCategory["package_manager"]; len(pms) > 0 {
		_, _ = fmt.Fprintf(os.Stdout, "**Package Managers:** %s.\n\n", strings.Join(pms, ", "))
	}

	// Tool categories in display order.
	for _, cat := range report.CategoryOrder {
		names := byCategory[cat]
		if len(names) == 0 {
			continue
		}
		label := report.CategoryLabels[cat]
		if label == "" {
			label = cat
		}
		_, _ = fmt.Fprintf(os.Stdout, "**%s:** %s.\n\n", label, strings.Join(names, ", "))
	}

	_, _ = fmt.Fprintf(os.Stdout, "Run `brief list tools` for the full list.\n")
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
		if err := enc.Encode(entries); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
			os.Exit(1)
		}
	}
}

func cmdSchema() {
	defs := make(map[string]any)
	root := schemaForType(reflect.TypeFor[brief.Report](), defs)
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["title"] = "brief report"
	root["description"] = "Output of brief project detection"
	if len(defs) > 0 {
		root["$defs"] = defs
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(root); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error writing schema: %v\n", err)
		os.Exit(1)
	}
}

// schemaForType generates a JSON Schema object for a Go type using reflection.
// Named struct types are emitted as $ref pointers into the $defs map.
func schemaForType(t reflect.Type, defs map[string]any) map[string]any {
	// Unwrap pointers.
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Skip special types that don't appear in JSON output.
	if t == reflect.TypeFor[time.Duration]() {
		return map[string]any{"type": "string"}
	}

	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice:
		return map[string]any{
			"type":  "array",
			"items": schemaForType(t.Elem(), defs),
		}
	case reflect.Map:
		return map[string]any{
			"type":                 "object",
			"additionalProperties": schemaForType(t.Elem(), defs),
		}
	case reflect.Struct:
		return schemaForStruct(t, defs)
	default:
		return map[string]any{}
	}
}

func schemaForStruct(t reflect.Type, defs map[string]any) map[string]any {
	// For the root type (Report), inline it. For other named types, use $ref.
	if t != reflect.TypeFor[brief.Report]() && t.Name() != "" {
		name := schemaDefName(t)
		if _, exists := defs[name]; !exists {
			// Insert placeholder to break cycles, then fill it in.
			defs[name] = map[string]any{}
			defs[name] = buildStructSchema(t, defs)
		}
		return map[string]any{"$ref": "#/$defs/" + name}
	}
	return buildStructSchema(t, defs)
}

func buildStructSchema(t reflect.Type, defs map[string]any) map[string]any {
	props := make(map[string]any)
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = f.Name
		}
		props[name] = schemaForType(f.Type, defs)
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
	}
}

// schemaDefName returns a lowercase name for use in $defs.
func schemaDefName(t reflect.Type) string {
	return strings.ToLower(t.Name())
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
