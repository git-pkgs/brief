package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/detect"
	"github.com/git-pkgs/brief/kb"
	"github.com/git-pkgs/brief/remote"
	"github.com/git-pkgs/brief/report"
	"github.com/git-pkgs/enrichment"
	"github.com/git-pkgs/enrichment/endoflife"
	"github.com/git-pkgs/enrichment/scorecard"
)

func cmdEnrich(args []string) {
	fs := flag.NewFlagSet("brief enrich", flag.ExitOnError)
	jsonFlag := fs.Bool("json", false, "Force JSON output")
	humanFlag := fs.Bool("human", false, "Force human-readable output")
	verbose := fs.Bool("verbose", false, "Include breadcrumb/reference information")
	keep := fs.Bool("keep", false, "Keep downloaded remote source")
	depth := fs.Int("depth", -1, "Git clone depth (0 = full clone, default shallow)")
	dir := fs.String("dir", "", "Directory to clone remote source into")
	scanDepth := fs.Int("scan-depth", 0, "Max directory depth for language detection (default 4)")
	skip := fs.String("skip", "", "Additional directories to skip, comma-separated")
	_ = fs.Parse(args)

	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

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

	ctx := context.Background()
	r.Enrichment = enrich(ctx, r, src.Dir)

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

func enrich(ctx context.Context, r *brief.Report, root string) *brief.EnrichmentInfo {
	info := &brief.EnrichmentInfo{}

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Published packages
	purls := detectPublishedPURLs(root, r)
	if len(purls) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			enrichPublishedPackages(ctx, purls, info, &mu)
		}()
	}

	// Runtime EOL
	if r.Platforms != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			enrichEOL(ctx, r, info, &mu)
		}()
	}

	// Repo scorecard
	if r.Git != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			enrichScorecard(ctx, r, info, &mu)
		}()
	}

	wg.Wait()

	if info.Repo == nil && len(info.Packages) == 0 && len(info.RuntimeEOL) == 0 {
		return nil
	}
	return info
}

// detectPublishedPURLs figures out what packages this repo publishes by
// reading the project's own identity from manifest files.
func detectPublishedPURLs(root string, r *brief.Report) []string {
	var purls []string

	// Go module
	if p := goModulePURL(root); p != "" {
		purls = append(purls, p)
	}

	// npm package
	if p := npmPackagePURL(root); p != "" {
		purls = append(purls, p)
	}

	// Python package
	if p := pythonPackagePURL(root); p != "" {
		purls = append(purls, p)
	}

	// Ruby gem
	if p := gemPURL(root); p != "" {
		purls = append(purls, p)
	}

	// Rust crate
	if p := cratePURL(root); p != "" {
		purls = append(purls, p)
	}

	return purls
}

func goModulePURL(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			mod := strings.TrimPrefix(line, "module ")
			mod = strings.TrimSpace(mod)
			return "pkg:golang/" + mod
		}
	}
	return ""
}

func npmPackagePURL(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Name    string `json:"name"`
		Private bool   `json:"private"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil || pkg.Name == "" || pkg.Private {
		return ""
	}
	name := strings.ReplaceAll(pkg.Name, "@", "%40")
	return "pkg:npm/" + name
}

func pythonPackagePURL(root string) string {
	// Try pyproject.toml [project] name
	data, err := os.ReadFile(filepath.Join(root, "pyproject.toml"))
	if err == nil {
		var pyproject struct {
			Project struct {
				Name string `toml:"name"`
			} `toml:"project"`
		}
		if _, err := toml.Decode(string(data), &pyproject); err == nil && pyproject.Project.Name != "" {
			return "pkg:pypi/" + pyproject.Project.Name
		}
	}

	// Try setup.cfg [metadata] name
	data, err = os.ReadFile(filepath.Join(root, "setup.cfg"))
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "name") && strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				name := strings.TrimSpace(parts[1])
				if name != "" {
					return "pkg:pypi/" + name
				}
			}
		}
	}
	return ""
}

func gemPURL(root string) string {
	// Look for *.gemspec
	matches, _ := filepath.Glob(filepath.Join(root, "*.gemspec"))
	if len(matches) > 0 {
		data, err := os.ReadFile(matches[0])
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				// Match: spec.name = "foo" or s.name = "foo"
				if strings.Contains(line, ".name") && strings.Contains(line, "=") {
					for _, q := range []string{`"`, `'`} {
						if idx := strings.Index(line, q); idx >= 0 {
							end := strings.Index(line[idx+1:], q)
							if end >= 0 {
								return "pkg:gem/" + line[idx+1:idx+1+end]
							}
						}
					}
				}
			}
		}
	}
	return ""
}

func cratePURL(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "Cargo.toml"))
	if err != nil {
		return ""
	}
	var cargo struct {
		Package struct {
			Name    string `toml:"name"`
			Publish *bool  `toml:"publish"`
		} `toml:"package"`
	}
	if _, err := toml.Decode(string(data), &cargo); err != nil || cargo.Package.Name == "" {
		return ""
	}
	if cargo.Package.Publish != nil && !*cargo.Package.Publish {
		return ""
	}
	return "pkg:cargo/" + cargo.Package.Name
}

func enrichPublishedPackages(ctx context.Context, purls []string, info *brief.EnrichmentInfo, mu *sync.Mutex) {
	client, err := enrichment.NewClient(enrichment.WithUserAgent("brief/" + brief.Version))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: enrichment client: %v\n", err)
		return
	}

	packages, err := client.BulkLookup(ctx, purls)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: enrichment lookup: %v\n", err)
		return
	}

	mu.Lock()
	defer mu.Unlock()
	for purl, pkg := range packages {
		if pkg == nil {
			continue
		}
		info.Packages = append(info.Packages, brief.PublishedPackage{
			Name:                   pkg.Name,
			Ecosystem:              pkg.Ecosystem,
			PURL:                   purl,
			LatestVersion:          pkg.LatestVersion,
			License:                pkg.License,
			Description:            pkg.Description,
			Downloads:              pkg.Downloads,
			DownloadsPeriod:        pkg.DownloadsPeriod,
			DependentPackagesCount: pkg.DependentPackagesCount,
			DependentReposCount:    pkg.DependentReposCount,
			RegistryURL:            pkg.RegistryURL,
		})
	}
}

// runtimeProducts maps runtime names from _runtimes.toml to endoflife.date product names.
var runtimeProducts = map[string]string{
	"Ruby":   "ruby",
	"Node":   "nodejs",
	"Python": "python",
	"Go":     "go",
}

func enrichEOL(ctx context.Context, r *brief.Report, info *brief.EnrichmentInfo, mu *sync.Mutex) {
	if r.Platforms == nil {
		return
	}

	eolClient := endoflife.New("brief/" + brief.Version)
	eolMap := make(map[string]*brief.RuntimeEOL)

	for file, version := range r.Platforms.RuntimeVersionFiles {
		product := productFromFile(file)
		if product == "" {
			continue
		}
		cycle := majorMinor(version)
		if cycle == "" {
			continue
		}

		c, err := eolClient.GetCycle(ctx, product, cycle)
		if err != nil {
			continue
		}

		key := product + " " + cycle
		eolMap[key] = &brief.RuntimeEOL{
			EOL:       formatDateOrBool(c.EOL),
			Supported: c.IsSupported(),
			LTS:       c.IsLTS(),
			Latest:    c.Latest,
		}
	}

	for name, versions := range r.Platforms.CIMatrixVersions {
		product, ok := runtimeProducts[capitalize(name)]
		if !ok {
			product = name
		}

		for _, version := range versions {
			cycle := majorMinor(version)
			if cycle == "" {
				continue
			}
			key := product + " " + cycle
			if eolMap[key] != nil {
				continue
			}

			c, err := eolClient.GetCycle(ctx, product, cycle)
			if err != nil {
				continue
			}
			eolMap[key] = &brief.RuntimeEOL{
				EOL:       formatDateOrBool(c.EOL),
				Supported: c.IsSupported(),
				LTS:       c.IsLTS(),
				Latest:    c.Latest,
			}
		}
	}

	if len(eolMap) == 0 {
		return
	}

	mu.Lock()
	info.RuntimeEOL = eolMap
	mu.Unlock()
}

func enrichScorecard(ctx context.Context, r *brief.Report, info *brief.EnrichmentInfo, mu *sync.Mutex) {
	if r.Git == nil {
		return
	}

	var repoURL string
	for _, url := range r.Git.Remotes {
		if strings.Contains(url, "github.com") || strings.Contains(url, "gitlab.com") {
			repoURL = url
			break
		}
	}
	if repoURL == "" {
		return
	}

	sc := scorecard.New("brief/" + brief.Version)
	result, err := sc.GetScore(ctx, repoURL)
	if err != nil {
		return
	}

	mu.Lock()
	info.Repo = &brief.RepoEnrichment{
		Scorecard:     result.Score,
		ScorecardDate: result.Date,
	}
	mu.Unlock()
}

func productFromFile(file string) string {
	switch file {
	case ".ruby-version":
		return "ruby"
	case ".node-version", ".nvmrc":
		return "nodejs"
	case ".python-version":
		return "python"
	case ".go-version":
		return "go"
	case "rust-toolchain.toml", "rust-toolchain":
		return "rust"
	}
	return ""
}

func majorMinor(version string) string {
	version = strings.TrimSpace(version)
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func formatDateOrBool(d endoflife.DateOrBool) string {
	if d.Bool != nil {
		if *d.Bool {
			return "true"
		}
		return "false"
	}
	if !d.Date.IsZero() {
		return d.Date.Format("2006-01-02")
	}
	return ""
}
