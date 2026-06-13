package report

import (
	"fmt"
	"hash/fnv"
	"html/template"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/git-pkgs/brief"
)

// HTML writes the report as a self-contained HTML page.
func HTML(w io.Writer, r *brief.Report) error {
	return htmlTmpl.Execute(w, newHTMLData(r))
}

type htmlData struct {
	R          *brief.Report
	Title      string
	RepoURL    string
	Langs      []langSlice
	ToolGroups []toolGroup
	Direct     []brief.DepInfo
	Transitive []brief.DepInfo
	DepSummary string
	Resources  []resourceLink
	Facts      []fact
}

type langSlice struct {
	Name    string
	Lines   int
	Percent float64
	Color   string
}

type toolGroup struct {
	Key   string
	Label string
	Icon  string
	Tools []brief.Detection
}

type resourceLink struct {
	Label string
	Path  string
	Note  string
	Icon  string
}

type fact struct {
	Label string
	Value string
}

func newHTMLData(r *brief.Report) *htmlData {
	d := &htmlData{R: r, Title: projectTitle(r), RepoURL: repoWebURL(r)}
	d.Langs = languageSlices(r.Lines)
	d.ToolGroups = orderedToolGroups(r)
	d.Direct, d.Transitive = splitDeps(r.Dependencies)
	d.DepSummary = depSummary(r.Dependencies)
	d.Resources = resourceLinks(r.Resources)
	d.Facts = facts(r)
	return d
}

func projectTitle(r *brief.Report) string {
	if r.Git != nil {
		if u := r.Git.Remotes["origin"]; u != "" {
			s := strings.TrimSuffix(u, ".git")
			s = strings.TrimSuffix(s, "/")
			if i := strings.LastIndexAny(s, "/:"); i >= 0 && i < len(s)-1 {
				return s[i+1:]
			}
		}
	}
	if r.Path != "" && r.Path != "." {
		return filepath.Base(r.Path)
	}
	return "project"
}

func repoWebURL(r *brief.Report) string {
	if r.Git == nil {
		return ""
	}
	u := r.Git.Remotes["origin"]
	if u == "" {
		return ""
	}
	u = strings.TrimSuffix(u, ".git")
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if rest, ok := strings.CutPrefix(u, "git@"); ok {
		return "https://" + strings.Replace(rest, ":", "/", 1)
	}
	return ""
}

//nolint:mnd // 100 is percent, 360 is hue degrees
func languageSlices(lc *brief.LineCount) []langSlice {
	if lc == nil || len(lc.ByLanguage) == 0 {
		return nil
	}
	skip := map[string]bool{"License": true, "Plain Text": true, "gitignore": true}
	type kv struct {
		k string
		v int
	}
	var pairs []kv
	total := 0
	for k, v := range lc.ByLanguage {
		if skip[k] || v == 0 {
			continue
		}
		pairs = append(pairs, kv{k, v})
		total += v
	}
	if total == 0 {
		return nil
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })

	const maxSlices = 8
	var out []langSlice
	other := 0
	for i, p := range pairs {
		if i >= maxSlices {
			other += p.v
			continue
		}
		out = append(out, langSlice{
			Name:    p.k,
			Lines:   p.v,
			Percent: float64(p.v) / float64(total) * 100,
			Color:   hashColor(p.k),
		})
	}
	if other > 0 {
		out = append(out, langSlice{
			Name:    "Other",
			Lines:   other,
			Percent: float64(other) / float64(total) * 100,
			Color:   "#6e7681",
		})
	}
	return out
}

func hashColor(s string) string {
	const hueRange = 360
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("hsl(%d 65%% 55%%)", h.Sum32()%hueRange)
}

//nolint:goconst // category keys repeat CategoryOrder/CategoryLabels by design
var categoryIcons = map[string]string{
	"language":        "code",
	"package_manager": "package",
	"test":            "flask-conical",
	"lint":            "search-check",
	"format":          "align-left",
	"typecheck":       "check-check",
	"docs":            "book-open",
	"build":           "hammer",
	"library":         "library",
	"codegen":         "wand",
	"database":        "database",
	"security":        "shield",
	"ci":              "rocket",
	"container":       "container",
	"infrastructure":  "server",
	"monorepo":        "layers",
	"environment":     "leaf",
	"i18n":            "languages",
	"release":         "tag",
	"coverage":        "gauge",
	"dependency_bot":  "bot",
}

func categoryIcon(cat string) string {
	if i, ok := categoryIcons[cat]; ok {
		return i
	}
	return "circle-dot"
}

const languageLineThreshold = 10.0

func majorLanguages(langs []brief.Detection, lc *brief.LineCount) []brief.Detection {
	if lc == nil || len(lc.ByLanguage) == 0 {
		return langs
	}
	total := 0
	for _, n := range lc.ByLanguage {
		total += n
	}
	if total == 0 {
		return langs
	}
	var out []brief.Detection
	for _, l := range langs {
		n, ok := lc.ByLanguage[l.Name]
		if !ok {
			out = append(out, l)
			continue
		}
		if float64(n)/float64(total)*100 >= languageLineThreshold {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return langs
	}
	return out
}

func orderedToolGroups(r *brief.Report) []toolGroup {
	var groups []toolGroup
	add := func(key, label string, dets []brief.Detection) {
		if len(dets) == 0 {
			return
		}
		groups = append(groups, toolGroup{Key: key, Label: label, Icon: categoryIcon(key), Tools: dets})
	}
	add("language", "Language", majorLanguages(r.Languages, r.Lines))
	add("package_manager", "Package Manager", r.PackageManagers)
	seen := map[string]bool{}
	for _, cat := range CategoryOrder {
		if dets := r.Tools[cat]; len(dets) > 0 {
			label := CategoryLabels[cat]
			if label == "" {
				label = cat
			}
			add(cat, label, dets)
			seen[cat] = true
		}
	}
	extras := make([]string, 0)
	for cat := range r.Tools {
		if !seen[cat] {
			extras = append(extras, cat)
		}
	}
	sort.Strings(extras)
	for _, cat := range extras {
		add(cat, cat, r.Tools[cat])
	}
	return groups
}

func splitDeps(deps []brief.DepInfo) (direct, transitive []brief.DepInfo) {
	for _, d := range deps {
		if d.Direct {
			direct = append(direct, d)
		} else {
			transitive = append(transitive, d)
		}
	}
	return direct, transitive
}

func resourceLinks(res *brief.ResourceInfo) []resourceLink {
	if res == nil {
		return nil
	}
	var out []resourceLink
	add := func(label, path, note, icon string) {
		if path != "" {
			out = append(out, resourceLink{Label: label, Path: path, Note: note, Icon: icon})
		}
	}
	add("Readme", res.Readme, "", "book-open")
	add("Changelog", res.Changelog, "", "history")
	add("Roadmap", res.Roadmap, "", "map")
	add("License", res.License, res.LicenseType, "scale")
	for _, k := range sortedKeys(res.Community) {
		add("Community", res.Community[k], k, "users")
	}
	for _, k := range sortedKeys(res.Security) {
		add("Security", res.Security[k], k, "shield")
	}
	for _, k := range sortedKeys(res.Legal) {
		add("Legal", res.Legal[k], k, "gavel")
	}
	for _, k := range sortedKeys(res.Metadata) {
		add("Metadata", res.Metadata[k], k, "file-text")
	}
	for _, k := range sortedKeys(res.Agents) {
		add("Agents", res.Agents[k], k, "bot")
	}
	return out
}

func facts(r *brief.Report) []fact {
	var out []fact
	if r.Resources != nil && r.Resources.LicenseType != "" {
		out = append(out, fact{"License", r.Resources.LicenseType})
	}
	if r.Style != nil && r.Style.Indentation != "" {
		out = append(out, fact{"Indent", r.Style.Indentation})
	}
	if r.Style != nil && r.Style.LineEnding != "" {
		out = append(out, fact{"Line ending", r.Style.LineEnding})
	}
	if r.Layout != nil && len(r.Layout.SourceDirs) > 0 {
		out = append(out, fact{"Source", joinDirs(r.Layout.SourceDirs)})
	}
	if r.Layout != nil && len(r.Layout.TestDirs) > 0 {
		out = append(out, fact{"Tests", joinDirs(r.Layout.TestDirs)})
	}
	if r.Platforms != nil && len(r.Platforms.CIMatrixOS) > 0 {
		out = append(out, fact{"CI OS", strings.Join(r.Platforms.CIMatrixOS, ", ")})
	}
	if r.Git != nil && r.Git.DefaultBranch != "" {
		out = append(out, fact{"Default branch", r.Git.DefaultBranch})
	}
	return out
}

//nolint:mnd // small formatting helpers
var htmlFuncs = template.FuncMap{
	"comma": func(n int) string {
		s := fmt.Sprintf("%d", n)
		if len(s) <= 3 {
			return s
		}
		var b strings.Builder
		pre := len(s) % 3
		if pre > 0 {
			b.WriteString(s[:pre])
			if len(s) > pre {
				b.WriteByte(',')
			}
		}
		for i := pre; i < len(s); i += 3 {
			if i > pre {
				b.WriteByte(',')
			}
			b.WriteString(s[i : i+3])
		}
		return b.String()
	},
	"pct": func(f float64) string {
		if f > 0 && f < 0.1 {
			return "<0.1"
		}
		return fmt.Sprintf("%.1f", f)
	},
	"short": func(s string) string {
		const n = 12
		if len(s) > n {
			return s[:n]
		}
		return s
	},
	"safeCSS": func(s string) template.CSS { return template.CSS(s) },
	"dict": func(kv ...any) map[string]any {
		m := make(map[string]any, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			if k, ok := kv[i].(string); ok {
				m[k] = kv[i+1]
			}
		}
		return m
	},
}

var htmlTmpl = template.Must(template.New("report").Funcs(htmlFuncs).Parse(htmlPage))

const htmlPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}} · brief</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/basecoat-css@0.3.11/dist/basecoat.cdn.min.css">
<script src="https://cdn.jsdelivr.net/npm/basecoat-css@0.3.11/dist/js/all.min.js" defer></script>
<script src="https://cdn.jsdelivr.net/npm/lucide@latest/dist/umd/lucide.min.js" defer></script>
<style>
  body { max-width: 1100px; margin: 0 auto; padding: 2rem 1.5rem 4rem; }
  .lucide { width: 1em; height: 1em; flex-shrink: 0; }
  .muted { color: var(--color-muted-foreground); }
  .mono { font-family: var(--font-mono); }
  .truncate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }

  .b-hdr { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between;
           gap: 1rem; margin-bottom: 2rem; }
  .b-hdr h1 { font-size: 1.75rem; font-weight: 600; letter-spacing: -0.01em; margin: 0; }
  .b-hdr p { font-size: 0.8rem; margin: 0.25rem 0 0; }

  .b-section { margin-bottom: 2.5rem; }
  .b-h2 { font-size: 0.7rem; font-weight: 600; text-transform: uppercase;
          letter-spacing: 0.08em; color: var(--color-muted-foreground); margin: 0 0 0.75rem; }

  .b-stats { display: flex; flex-wrap: wrap; gap: 0.75rem; }
  .b-stat.card { flex-direction: row; align-items: center; gap: 0.9rem;
                 padding: 0.75rem 1rem; min-width: 8.5rem; flex: 1 1 0; }
  .b-stat .lucide { width: 1.25rem; height: 1.25rem; color: var(--color-muted-foreground); }
  .b-stat-v { font-size: 1.15rem; font-weight: 600; line-height: 1.1; white-space: nowrap; }
  .b-stat-l { font-size: 0.65rem; text-transform: uppercase; letter-spacing: 0.06em;
              color: var(--color-muted-foreground); }

  .langbar { display: flex; height: 10px; border-radius: 9999px; overflow: hidden;
             border: 1px solid var(--color-border); margin-bottom: 0.75rem; }
  .langbar > span { display: block; height: 100%; }
  .langlegend { display: flex; flex-wrap: wrap; gap: 0.4rem 1.25rem; padding: 0; margin: 0;
                list-style: none; font-size: 0.85rem; }
  .langlegend li { display: flex; align-items: center; gap: 0.4rem; }
  .langlegend .pc { font-size: 0.75rem; color: var(--color-muted-foreground); }
  .dot { width: 9px; height: 9px; border-radius: 9999px; display: inline-block; flex-shrink: 0; }

  .b-groups { display: flex; flex-direction: column; gap: 1rem; }
  .b-group.card { padding: 0; gap: 0; }
  .b-group.card > header { padding: 0.9rem 1.1rem; border-bottom: 1px solid var(--color-border); }
  .b-group.card > header h2 { display: flex; align-items: center; gap: 0.55rem; margin: 0; }
  .b-group.card > header .lucide { width: 1.05rem; height: 1.05rem; color: var(--color-muted-foreground); }
  .b-group.card > header .n { margin-left: auto; font-weight: 400; font-size: 0.8rem;
                              color: var(--color-muted-foreground); }
  .b-tools { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
             gap: 1.25rem 1.75rem; padding: 1rem 1.1rem 1.1rem; }
  .b-tool { display: flex; flex-direction: column; gap: 0.5rem; min-width: 0; }
  .b-tool-h { display: flex; align-items: baseline; flex-wrap: wrap; gap: 0.2rem 0.6rem; min-width: 0; }
  .b-tool-name { font-weight: 600; font-size: 0.92rem; color: inherit; text-decoration: none; }
  a.b-tool-name:hover { text-decoration: underline; }
  .b-tool-desc { font-size: 0.8rem; color: var(--color-muted-foreground); }
  .b-cmd { display: flex; align-items: center; gap: 0.25rem; }
  .b-cmd code { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
                border: 1px solid var(--color-border); background: var(--color-muted);
                border-radius: var(--radius-md); padding: 0.4rem 0.6rem;
                font-family: var(--font-mono); font-size: 0.75rem; }
  .b-tool-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 0.25rem 0.4rem;
                 margin-left: -0.5rem; }
  .b-files { font-family: var(--font-mono); font-size: 0.7rem; color: var(--color-muted-foreground);
             overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
             margin-left: 0.5rem; flex-basis: 100%; }

  .b-summary { display: flex; align-items: center; gap: 0.6rem; }
  .b-summary .lucide { color: var(--color-muted-foreground); }
  .b-summary .sub { font-size: 0.8rem; font-weight: 400; color: var(--color-muted-foreground); }
  .accordion details > section { padding-top: 0.75rem; }
  .tabs { width: 100%; }
  .tabs > nav { width: 100%; }
  .tabs [role=tab] .n { color: var(--color-muted-foreground); margin-left: 0.35rem; }
  .b-scroll { overflow-x: auto; }
  .table td.dep-n { font-family: var(--font-mono); }
  .table td.dep-n .badge-outline { margin-left: 0.5rem; }
  .table td.dep-v { font-family: var(--font-mono); text-align: right;
                    color: var(--color-muted-foreground); white-space: nowrap; }

  .b-pills { display: flex; flex-wrap: wrap; gap: 0.5rem; margin-bottom: 0.75rem; }
  .b-pills .badge-outline { gap: 0.4rem; }
  .b-pills .note { color: var(--color-muted-foreground); }

  .b-facts { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
             gap: 1.25rem 2rem; }
  .b-facts .k { font-size: 0.65rem; text-transform: uppercase; letter-spacing: 0.06em;
                color: var(--color-muted-foreground); }
  .b-facts .v { font-size: 0.85rem; margin-top: 0.2rem; word-break: break-word; }

  .b-foot { display: flex; flex-wrap: wrap; gap: 0.25rem 1rem; font-size: 0.75rem;
            color: var(--color-muted-foreground); border-top: 1px solid var(--color-border);
            padding-top: 1rem; margin-top: 3rem; }
</style>
</head>
<body>

<header class="b-hdr">
  <div>
    <h1>{{.Title}}</h1>
    <p class="muted mono">{{.R.Path}}</p>
  </div>
  {{if .RepoURL}}
  <a href="{{.RepoURL}}" class="btn-outline"><i data-lucide="git-branch"></i> Repository</a>
  {{end}}
</header>

<section class="b-section b-stats">
  {{with .R.Lines}}
  {{template "stat" dict "v" (comma .TotalLines) "l" "lines" "i" "file-code"}}
  {{template "stat" dict "v" (comma .TotalFiles) "l" "files" "i" "files"}}
  {{end}}
  {{if .R.Dependencies}}
  {{template "stat" dict "v" (printf "%d" (len .Direct)) "l" "direct deps" "i" "package"}}
  {{end}}
  {{template "stat" dict "v" (printf "%d" .R.Stats.ToolsMatched) "l" "tools" "i" "wrench"}}
  {{with .R.Git}}{{if .CommitCount}}
  {{template "stat" dict "v" (comma .CommitCount) "l" "commits" "i" "git-commit-horizontal"}}
  {{end}}{{end}}
  {{with .R.Resources}}{{if .LicenseType}}
  {{template "stat" dict "v" .LicenseType "l" "license" "i" "scale"}}
  {{end}}{{end}}
</section>

{{if .Langs}}
<section class="b-section">
  <h2 class="b-h2">Languages</h2>
  <div class="langbar">
    {{range .Langs}}<span style="width:{{.Percent}}%;background:{{safeCSS .Color}}" title="{{.Name}} {{pct .Percent}}%"></span>{{end}}
  </div>
  <ul class="langlegend">
    {{range .Langs}}
    <li>
      <span class="dot" style="background:{{safeCSS .Color}}"></span>
      <span>{{.Name}}</span>
      <span class="pc">{{pct .Percent}}%</span>
    </li>
    {{end}}
  </ul>
</section>
{{end}}

{{if .ToolGroups}}
<section class="b-section">
  <h2 class="b-h2">Toolchain</h2>
  <div class="b-groups">
    {{range .ToolGroups}}
    <div class="card b-group">
      <header>
        <h2>
          <i data-lucide="{{.Icon}}"></i>{{.Label}}
          {{if gt (len .Tools) 1}}<span class="n">{{len .Tools}}</span>{{end}}
        </h2>
      </header>
      <div class="b-tools">
      {{range .Tools}}
      <div class="b-tool">
        <div class="b-tool-h">
          {{if .Homepage}}<a href="{{.Homepage}}" class="b-tool-name truncate">{{.Name}}</a>{{else}}<span class="b-tool-name truncate">{{.Name}}</span>{{end}}
          {{if .Description}}<span class="b-tool-desc">{{.Description}}</span>{{end}}
        </div>
        {{with .Command}}
        <div class="b-cmd">
          <code>{{.Run}}</code>
          <button type="button" class="btn-sm-icon-ghost" data-copy="{{.Run}}" aria-label="Copy command">
            <i data-lucide="copy"></i>
          </button>
        </div>
        {{end}}
        {{if or .Docs .Repo .Lockfile .ConfigFiles}}
        <div class="b-tool-meta">
          {{if .Docs}}<a href="{{.Docs}}" class="btn-sm-ghost"><i data-lucide="book-open"></i> Docs</a>{{end}}
          {{if .Repo}}<a href="{{.Repo}}" class="btn-sm-ghost"><i data-lucide="github"></i> Source</a>{{end}}
          {{if .Lockfile}}<span class="badge-outline"><i data-lucide="lock"></i> {{.Lockfile}}</span>{{end}}
          {{if .ConfigFiles}}<span class="b-files">{{range $i, $f := .ConfigFiles}}{{if $i}} · {{end}}{{$f}}{{end}}</span>{{end}}
        </div>
        {{end}}
      </div>
      {{end}}
      </div>
    </div>
    {{end}}
  </div>
</section>
{{end}}

{{if .R.Dependencies}}
<section class="b-section">
  <h2 class="b-h2">Dependencies</h2>
  <section class="accordion">
    <details class="group">
      <summary>
        <h2 class="b-summary">
          <i data-lucide="boxes"></i>
          Packages
          <span class="sub">{{if .DepSummary}}{{.DepSummary}}{{else}}{{len .R.Dependencies}}{{end}}</span>
        </h2>
      </summary>
      <section>
        <div class="tabs" id="dep-tabs">
          <nav role="tablist" aria-orientation="horizontal">
            <button type="button" role="tab" id="dep-tab-direct" aria-controls="dep-panel-direct" aria-selected="true" tabindex="0">
              Direct <span class="n">{{len .Direct}}</span>
            </button>
            {{if .Transitive}}
            <button type="button" role="tab" id="dep-tab-trans" aria-controls="dep-panel-trans" aria-selected="false" tabindex="-1">
              Transitive <span class="n">{{len .Transitive}}</span>
            </button>
            {{end}}
          </nav>
          <div role="tabpanel" id="dep-panel-direct" aria-labelledby="dep-tab-direct" tabindex="-1" aria-selected="true">
            {{template "deptable" .Direct}}
          </div>
          {{if .Transitive}}
          <div role="tabpanel" id="dep-panel-trans" aria-labelledby="dep-tab-trans" tabindex="-1" aria-selected="false" hidden>
            {{template "deptable" .Transitive}}
          </div>
          {{end}}
        </div>
      </section>
    </details>
  </section>
</section>
{{end}}

{{if or .Resources .R.Scripts}}
<section class="b-section">
  <h2 class="b-h2">Project</h2>
  {{if .Resources}}
  <div class="b-pills">
    {{range .Resources}}
    <a href="{{.Path}}" class="badge-outline">
      <i data-lucide="{{.Icon}}"></i>
      {{.Label}}{{if .Note}}<span class="note">{{.Note}}</span>{{end}}
    </a>
    {{end}}
  </div>
  {{end}}
  {{if .R.Scripts}}
  <section class="accordion">
    <details class="group">
      <summary>
        <h2 class="b-summary">
          <i data-lucide="terminal"></i>
          Scripts <span class="sub">{{len .R.Scripts}}</span>
        </h2>
      </summary>
      <section>
        <div class="b-scroll">
          <table class="table">
            <tbody>
              {{range .R.Scripts}}
              <tr>
                <td class="dep-n">{{.Name}}</td>
                <td class="mono muted">{{.Run}}</td>
                <td class="dep-v">{{.Source}}</td>
              </tr>
              {{end}}
            </tbody>
          </table>
        </div>
      </section>
    </details>
  </section>
  {{end}}
</section>
{{end}}

{{if .Facts}}
<section class="b-section">
  <h2 class="b-h2">Conventions</h2>
  <div class="card">
    <section class="b-facts">
      {{range .Facts}}
      <div><div class="k">{{.Label}}</div><div class="v">{{.Value}}</div></div>
      {{end}}
    </section>
  </div>
</section>
{{end}}

<footer class="b-foot">
  <span>brief {{.R.Version}}</span>
  <span>{{printf "%.1f" .R.Stats.DurationMS}}ms</span>
  <span>{{.R.Stats.FilesChecked}} files scanned</span>
  <span>{{.R.Stats.ToolsMatched}}/{{.R.Stats.ToolsChecked}} tools matched</span>
</footer>

<script>
window.addEventListener('DOMContentLoaded',function(){
  if(window.lucide)lucide.createIcons();
  document.querySelectorAll('[data-copy]').forEach(function(b){
    b.addEventListener('click',function(){
      navigator.clipboard.writeText(b.dataset.copy).then(function(){
        var i=b.querySelector('svg');if(!i)return;
        var o=i.outerHTML;i.outerHTML='<i data-lucide="check"></i>';lucide.createIcons();
        setTimeout(function(){b.querySelector('svg').outerHTML=o},1200);
      });
    });
  });
});
</script>
</body>
</html>

{{define "stat"}}
<div class="card b-stat">
  <i data-lucide="{{.i}}"></i>
  <div>
    <div class="b-stat-v">{{.v}}</div>
    <div class="b-stat-l">{{.l}}</div>
  </div>
</div>
{{end}}

{{define "deptable"}}
<div class="b-scroll">
  <table class="table">
    <tbody>
      {{range .}}
      <tr>
        <td class="dep-n">{{.Name}}{{if and .Scope (ne .Scope "runtime")}}<span class="badge-outline">{{.Scope}}</span>{{end}}</td>
        <td class="dep-v" title="{{.Version}}">{{short .Version}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}
`
