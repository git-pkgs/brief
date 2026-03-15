package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

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

	// Enrich the report with external data
	ctx := context.Background()
	r.Enrichment = enrich(ctx, r)

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

func enrich(ctx context.Context, r *brief.Report) *brief.EnrichmentInfo {
	info := &brief.EnrichmentInfo{
		Dependencies: make(map[string]*brief.DepEnrichment),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Enrich dependencies via ecosyste.ms
	if len(r.Dependencies) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			enrichDeps(ctx, r, info, &mu)
		}()
	}

	// Enrich runtime EOL
	if r.Platforms != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			enrichEOL(ctx, r, info, &mu)
		}()
	}

	// Enrich repo scorecard
	if r.Git != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			enrichScorecard(ctx, r, info, &mu)
		}()
	}

	wg.Wait()

	if info.Repo == nil && len(info.RuntimeEOL) == 0 && len(info.Dependencies) == 0 {
		return nil
	}
	return info
}

func enrichDeps(ctx context.Context, r *brief.Report, info *brief.EnrichmentInfo, mu *sync.Mutex) {
	// Deduplicate PURLs, strip versions for package-level lookup
	seen := make(map[string]bool)
	var purls []string
	for _, dep := range r.Dependencies {
		// Strip version from PURL for package-level lookup
		p := dep.PURL
		if idx := strings.Index(p, "@"); idx > 0 {
			p = p[:idx]
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		purls = append(purls, p)
	}

	if len(purls) == 0 {
		return
	}

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
		dep := &brief.DepEnrichment{
			LatestVersion:          pkg.LatestVersion,
			License:                pkg.License,
			Downloads:              pkg.Downloads,
			DownloadsPeriod:        pkg.DownloadsPeriod,
			DependentPackagesCount: pkg.DependentPackagesCount,
			DependentReposCount:    pkg.DependentReposCount,
			Repository:             pkg.Repository,
			RegistryURL:            pkg.RegistryURL,
			Description:            pkg.Description,
		}
		for _, adv := range pkg.Advisories {
			dep.Advisories = append(dep.Advisories, brief.AdvisoryInfo{
				Title:       adv.Title,
				Severity:    adv.Severity,
				CVSSScore:   adv.CVSSScore,
				URL:         adv.URL,
				Identifiers: adv.Identifiers,
			})
		}
		info.Dependencies[purl] = dep
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

	// Check runtime version files (e.g. .ruby-version = "3.4.2")
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

	// Check CI matrix versions
	for name, versions := range r.Platforms.CIMatrixVersions {
		product, ok := runtimeProducts[capitalize(name)]
		if !ok {
			// Try the name directly
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

	// Find a GitHub/GitLab remote URL for scorecard
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

// productFromFile maps runtime version file names to endoflife.date product names.
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

// majorMinor extracts "3.4" from "3.4.2" or "3.4".
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
	if !d.Date.Time.IsZero() {
		return d.Date.Time.Format("2006-01-02")
	}
	return ""
}
