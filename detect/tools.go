package detect

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/kb"
)

// ToolScanOptions scopes tool detection for an existing project snapshot.
type ToolScanOptions struct {
	// Full evaluates every non-language definition.
	Full bool
	// ChangedFiles limits a partial scan to definitions affected by these paths.
	ChangedFiles []string
	// Languages supplies language names already detected by the caller.
	Languages []string
	// LanguagesChanged reevaluates definitions scoped to a language ecosystem.
	LanguagesChanged bool
}

// ToolRef identifies a tool definition evaluated by a tool scan.
type ToolRef struct {
	Name     string
	Category string
}

// ToolScan contains matches and every definition evaluated by a tool scan.
type ToolScan struct {
	Tools   map[string][]brief.Detection
	Checked []ToolRef
	Stats   brief.Stats
}

// RunTools detects tools affected by a project snapshot or its changed files.
func (e *Engine) RunTools(options ToolScanOptions) (*ToolScan, error) {
	started := time.Now()
	if err := e.validateOptions(); err != nil {
		return nil, err
	}
	abs, err := e.reportPath()
	if err != nil {
		return nil, err
	}
	info, err := fs.Stat(e.projectFS(), ".")
	if err != nil {
		return nil, fmt.Errorf("path does not exist: %s", abs)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", abs)
	}
	if e.TrackedOnly {
		if err := e.loadTracked(abs); err != nil {
			return nil, err
		}
	}

	e.detectedEcosystems = ecosystemsForLanguages(e.KB, options.Languages)
	result := &ToolScan{Tools: make(map[string][]brief.Detection)}
	var scriptsByName map[string]brief.Script
	var changes *filterContext
	if !options.Full {
		changes = newFilterContext(e.KB, options.ChangedFiles)
	}
	for _, category := range e.KB.Categories() {
		if category == categoryLanguage {
			continue
		}
		definitions := e.toolScanDefinitions(category, options.Full, changes, options.LanguagesChanged)
		for _, tool := range definitions {
			result.Checked = append(result.Checked, ToolRef{Name: tool.Tool.Name, Category: category})
		}
		if detections := e.detectDefinitions(definitions); len(detections) > 0 {
			scriptsByName = e.linkToolScanScripts(detections, category, scriptsByName)
			result.Tools[category] = detections
		}
	}
	sort.Slice(result.Checked, func(i, j int) bool {
		if result.Checked[i].Category != result.Checked[j].Category {
			return result.Checked[i].Category < result.Checked[j].Category
		}
		return result.Checked[i].Name < result.Checked[j].Name
	})
	elapsed := time.Since(started)
	result.Stats = brief.Stats{
		Duration:      elapsed,
		DurationMS:    float64(elapsed.Microseconds()) / microsPerMS,
		FilesChecked:  e.filesChecked,
		ToolsMatched:  e.toolsMatched,
		ToolsChecked:  e.toolsChecked,
		ScanEntries:   e.scanEntries,
		ScanTruncated: e.scanTruncated || e.scanDepthTruncated,
	}
	return result, nil
}

func (e *Engine) toolScanDefinitions(category string, full bool, changes *filterContext, languagesChanged bool) []*kb.ToolDef {
	definitions := e.KB.ToolsForCategory(category)
	if full {
		return definitions
	}
	var affected []*kb.ToolDef
	for _, tool := range definitions {
		if toolAffected(tool, changes, languagesChanged) {
			affected = append(affected, tool)
		}
	}
	if len(affected) > 0 {
		if _, ok := categoryScriptNames[category]; ok {
			return definitions
		}
	}
	return affected
}

func (e *Engine) linkToolScanScripts(detections []brief.Detection, category string, scriptsByName map[string]brief.Script) map[string]brief.Script {
	if _, ok := categoryScriptNames[category]; !ok {
		return scriptsByName
	}
	if scriptsByName == nil {
		scriptsByName = make(map[string]brief.Script)
		for _, script := range e.detectScripts() {
			scriptsByName[script.Name] = script
		}
	}
	linkScriptToTool(detections, category, scriptsByName)
	return scriptsByName
}

func ecosystemsForLanguages(knowledge *kb.KnowledgeBase, languages []string) map[string]bool {
	ecosystems := make(map[string]bool)
	definitions := knowledge.ToolsForCategory(categoryLanguage)
	for _, language := range languages {
		matched := false
		for _, tool := range definitions {
			if strings.EqualFold(language, tool.Tool.Name) {
				addToolEcosystems(ecosystems, tool)
				matched = true
			}
		}
		if matched {
			continue
		}
		key := languageKey(language)
		for _, tool := range definitions {
			if key == languageKey(tool.Tool.Name) {
				addToolEcosystems(ecosystems, tool)
			}
		}
	}
	return ecosystems
}

func addToolEcosystems(ecosystems map[string]bool, tool *kb.ToolDef) {
	for _, ecosystem := range tool.Detect.Ecosystems {
		ecosystems[ecosystem] = true
	}
}

func languageKey(name string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsLetter(char) || unicode.IsNumber(char) {
			return unicode.ToLower(char)
		}
		return -1
	}, name)
}

func toolAffected(tool *kb.ToolDef, changes *filterContext, languagesChanged bool) bool {
	if scriptSourceChanged(tool.Tool.Category, changes) {
		return true
	}
	if languagesChanged && len(tool.Detect.Ecosystems) > 0 {
		return true
	}
	if toolMatchesChangedFiles(tool, changes.changed, changes.changedExts) {
		return true
	}
	if changes.manifestChanged && tool.Tool.Category == "package_manager" {
		return true
	}
	return changes.manifestChanged && (len(tool.Detect.Dependencies) > 0 || len(tool.Detect.DevDependencies) > 0)
}

func scriptSourceChanged(category string, changes *filterContext) bool {
	if _, ok := categoryScriptNames[category]; !ok {
		return false
	}
	for _, source := range changes.kb.ScriptSources {
		if changes.changed[source.Source.File] {
			return true
		}
	}
	return false
}
